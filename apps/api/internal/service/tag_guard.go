package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"api/internal/biz"

	"github.com/bizjs/kratoscarf/router"
)

const maxManifestBytes int64 = 16 << 20

var digestReferencePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_+.-]*:[A-Za-z0-9=_-]+$`)

var hopByHopHeaders = map[string]struct{}{
	"Connection":          {},
	"Proxy-Connection":    {},
	"Keep-Alive":          {},
	"Te":                  {},
	"Trailer":             {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
}

type TagGuardService struct {
	policy   *biz.RegistryPolicyUsecase
	tokens   *biz.TokenIssuer
	audit    *biz.AuditUsecase
	upstream string
	realm    string
	client   *http.Client
	locks    *tagLocker
}

func NewTagGuardService(
	policy *biz.RegistryPolicyUsecase,
	tokens *biz.TokenIssuer,
	audit *biz.AuditUsecase,
	upstream biz.RegistryUpstreamURL,
	realm biz.RegistryAuthRealm,
) *TagGuardService {
	return &TagGuardService{
		policy:   policy,
		tokens:   tokens,
		audit:    audit,
		upstream: strings.TrimRight(string(upstream), "/"),
		realm:    string(realm),
		client:   &http.Client{Timeout: 30 * time.Second},
		locks:    newTagLocker(),
	}
}

// PutManifest is a raw OCI Distribution endpoint. It must never use the
// kratoscarf response envelope because Docker clients expect registry-native
// status codes, headers and error bodies.
func (s *TagGuardService) PutManifest(ctx *router.Context) error {
	policy, leave, err := s.policy.EnterManifestPut(ctx.Context())
	if err != nil {
		return s.writeRegistryError(ctx, http.StatusServiceUnavailable, "DENIED",
			"registry policy is temporarily unavailable", nil)
	}
	defer leave()

	if !policy.PreventTagOverwrite {
		return s.proxyPut(ctx, ctx.Request().Body, ctx.Request().ContentLength)
	}

	repo := ctx.Param("name")
	reference := ctx.Param("reference")
	if repo == "" || reference == "" {
		return s.writeRegistryError(ctx, http.StatusBadRequest, "NAME_INVALID",
			"invalid repository or manifest reference", nil)
	}

	rawToken, ok := biz.ParseBearerToken(ctx.Header("Authorization"))
	if !ok {
		return s.writeAuthError(ctx, repo, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
	}
	actor, err := s.tokens.VerifyRegistryAccess(rawToken, repo, "push")
	if err != nil {
		if errors.Is(err, biz.ErrRegistryAccessDenied) {
			return s.writeAuthError(ctx, repo, http.StatusForbidden, "DENIED", "requested access to the resource is denied")
		}
		return s.writeAuthError(ctx, repo, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
	}

	body, err := io.ReadAll(io.LimitReader(ctx.Request().Body, maxManifestBytes+1))
	if err != nil {
		return s.writeRegistryError(ctx, http.StatusBadRequest, "MANIFEST_INVALID", "cannot read manifest", nil)
	}
	if int64(len(body)) > maxManifestBytes {
		return s.writeRegistryError(ctx, http.StatusRequestEntityTooLarge, "MANIFEST_INVALID", "manifest exceeds 16 MiB limit", nil)
	}

	attemptedDigest := reference
	targetTags := append([]string(nil), ctx.Request().URL.Query()["tag"]...)
	if !digestReferencePattern.MatchString(reference) {
		sum := sha256.Sum256(body)
		attemptedDigest = "sha256:" + hex.EncodeToString(sum[:])
		targetTags = append(targetTags, reference)
	}
	for _, tag := range targetTags {
		if tag == "" || strings.Contains(tag, "/") {
			return s.writeRegistryError(ctx, http.StatusBadRequest, "TAG_INVALID", "invalid manifest tag", nil)
		}
	}
	if len(targetTags) == 0 {
		return s.proxyPut(ctx, io.NopCloser(bytes.NewReader(body)), int64(len(body)))
	}

	lockKeys := make([]string, 0, len(targetTags))
	for _, tag := range targetTags {
		lockKeys = append(lockKeys, repo+"\x00"+tag)
	}
	unlock := s.locks.lockMany(lockKeys)
	defer unlock()

	for _, tag := range uniqueSorted(targetTags) {
		currentDigest, status, retryAfter, err := s.currentTagDigest(ctx, repo, tag)
		if err != nil {
			if retryAfter != "" {
				ctx.SetHeader("Retry-After", retryAfter)
			}
			return s.writeRegistryError(ctx, http.StatusServiceUnavailable, "DENIED",
				"cannot verify current tag state; overwrite protection failed closed", nil)
		}
		if status == http.StatusNotFound {
			continue
		}
		if currentDigest != attemptedDigest {
			s.audit.Write(ctx.Context(), biz.AuditEntry{
				Actor:    actor,
				Action:   biz.ActionTagOverwriteDenied,
				Target:   repo + ":" + tag,
				Scope:    "repository:" + repo + ":push",
				ClientIP: ctx.ClientIP(),
				Success:  false,
				Detail: map[string]any{
					"current_digest":   currentDigest,
					"attempted_digest": attemptedDigest,
					"media_type":       ctx.Header("Content-Type"),
				},
			})
			return s.writeRegistryError(ctx, http.StatusConflict, "DENIED",
				"tag is immutable and cannot be overwritten", map[string]any{
					"name":             repo,
					"tag":              tag,
					"current_digest":   currentDigest,
					"attempted_digest": attemptedDigest,
				})
		}
	}

	return s.proxyPut(ctx, io.NopCloser(bytes.NewReader(body)), int64(len(body)))
}

func (s *TagGuardService) currentTagDigest(ctx *router.Context, repo, tag string) (string, int, string, error) {
	token, err := s.tokens.IssueRegistryToken("dockery-tag-guard", []biz.RegistryAccess{{
		Type: "repository", Name: repo, Actions: []string{"pull"},
	}})
	if err != nil {
		return "", 0, "", err
	}
	endpoint := s.upstream + "/v2/" + repo + "/manifests/" + url.PathEscape(tag)
	req, err := http.NewRequestWithContext(ctx.Context(), http.MethodHead, endpoint, nil)
	if err != nil {
		return "", 0, "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", strings.Join([]string{
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.oci.image.index.v1+json",
		"application/vnd.docker.distribution.manifest.v2+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
	}, ","))
	resp, err := s.client.Do(req)
	if err != nil {
		return "", 0, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", resp.StatusCode, "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", resp.StatusCode, resp.Header.Get("Retry-After"), fmt.Errorf("tag HEAD status %d", resp.StatusCode)
	}
	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" || !digestReferencePattern.MatchString(digest) {
		return "", resp.StatusCode, "", fmt.Errorf("tag HEAD returned invalid digest")
	}
	return digest, resp.StatusCode, "", nil
}

func (s *TagGuardService) proxyPut(ctx *router.Context, body io.ReadCloser, contentLength int64) error {
	endpoint := s.upstream + ctx.Request().URL.RequestURI()
	req, err := http.NewRequestWithContext(ctx.Context(), http.MethodPut, endpoint, body)
	if err != nil {
		return s.writeRegistryError(ctx, http.StatusBadGateway, "DENIED", "cannot proxy manifest", nil)
	}
	req.ContentLength = contentLength
	req.Host = ctx.Request().Host
	copyHTTPHeaders(req.Header, ctx.Request().Header)

	resp, err := s.client.Do(req)
	if err != nil {
		return s.writeRegistryError(ctx, http.StatusBadGateway, "DENIED", "upstream registry unavailable", nil)
	}
	defer resp.Body.Close()
	copyHTTPHeaders(ctx.Response().Header(), resp.Header)
	ctx.Response().Header().Set("Docker-Distribution-Api-Version", "registry/2.0")
	ctx.Response().WriteHeader(resp.StatusCode)
	_, copyErr := io.Copy(ctx.Response(), resp.Body)
	return copyErr
}

type registryErrorResponse struct {
	Errors []registryErrorItem `json:"errors"`
}

type registryErrorItem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  any    `json:"detail,omitempty"`
}

func (s *TagGuardService) writeRegistryError(
	ctx *router.Context,
	status int,
	code, message string,
	detail any,
) error {
	ctx.SetHeader("Docker-Distribution-Api-Version", "registry/2.0")
	return ctx.JSON(status, registryErrorResponse{Errors: []registryErrorItem{{
		Code: code, Message: message, Detail: detail,
	}}})
}

func (s *TagGuardService) writeAuthError(ctx *router.Context, repo string, status int, code, message string) error {
	if status == http.StatusUnauthorized {
		realm := s.realm
		if realm == "" {
			proto := ctx.Header("X-Forwarded-Proto")
			if proto == "" {
				proto = "http"
			}
			realm = proto + "://" + ctx.Request().Host + "/token"
		}
		ctx.SetHeader("WWW-Authenticate", fmt.Sprintf(
			`Bearer realm=%q,service=%q,scope=%q`, realm, "dockery", "repository:"+repo+":pull,push"))
	}
	return s.writeRegistryError(ctx, status, code, message, nil)
}

func copyHTTPHeaders(dst, src http.Header) {
	for key := range dst {
		dst.Del(key)
	}
	for key, values := range src {
		if _, skip := hopByHopHeaders[http.CanonicalHeaderKey(key)]; skip {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}
