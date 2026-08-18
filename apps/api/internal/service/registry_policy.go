package service

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"api/internal/biz"

	"github.com/bizjs/kratoscarf/response"
	"github.com/bizjs/kratoscarf/router"
)

type RegistryPolicyService struct {
	policy *biz.RegistryPolicyBiz
	audit  *biz.AuditUsecase
}

func NewRegistryPolicyService(policy *biz.RegistryPolicyBiz, audit *biz.AuditUsecase) *RegistryPolicyService {
	return &RegistryPolicyService{policy: policy, audit: audit}
}

type RegistryPolicyView struct {
	PreventTagOverwrite bool     `json:"prevent_tag_overwrite"`
	OverwriteExclusions []string `json:"overwrite_exclusions"`
	Version             int64    `json:"version"`
	UpdatedAt           int64    `json:"updated_at"`
	UpdatedBy           string   `json:"updated_by"`
}

type UpdateRegistryPolicyRequest struct {
	PreventTagOverwrite *bool     `json:"prevent_tag_overwrite" validate:"required"`
	OverwriteExclusions *[]string `json:"overwrite_exclusions" validate:"omitempty,max=128,dive,max=128"`
	Version             *int64    `json:"version" validate:"required,min=0"`
}

func (s *RegistryPolicyService) Get(ctx *router.Context) error {
	p, err := s.policy.Get()
	if err != nil {
		return response.ErrInternal.WithCause(err)
	}
	return ctx.Success(toRegistryPolicyView(p))
}

func (s *RegistryPolicyService) Update(ctx *router.Context) error {
	var req UpdateRegistryPolicyRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}

	before, err := s.policy.Get()
	if err != nil {
		return response.ErrInternal.WithCause(err)
	}
	actor := sessionUsername(ctx)
	overwriteExclusions := before.OverwriteExclusions
	if req.OverwriteExclusions != nil {
		overwriteExclusions = *req.OverwriteExclusions
	}
	updated, err := s.policy.Update(
		ctx.Context(),
		*req.Version,
		*req.PreventTagOverwrite,
		overwriteExclusions,
		actor,
	)
	if err != nil {
		switch {
		case errors.Is(err, biz.ErrRegistryPolicyConflict):
			return response.NewBizError(http.StatusConflict, 40902, "registry policy was changed by another administrator")
		case errors.Is(err, biz.ErrRegistryPolicyInvalid):
			return response.NewBizError(http.StatusUnprocessableEntity, 42202, err.Error())
		case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
			return response.NewBizError(http.StatusServiceUnavailable, 50301, "registry policy switch timed out")
		default:
			return response.ErrInternal.WithCause(err)
		}
	}

	if before.PreventTagOverwrite != updated.PreventTagOverwrite ||
		!slices.Equal(before.OverwriteExclusions, updated.OverwriteExclusions) {
		s.audit.Write(ctx.Context(), biz.AuditEntry{
			Actor:    actor,
			Action:   biz.ActionRegistryPolicyUpdated,
			Target:   "tag-overwrite-protection",
			ClientIP: ctx.ClientIP(),
			Success:  true,
			Detail: map[string]any{
				"before": map[string]any{
					"prevent_tag_overwrite": before.PreventTagOverwrite,
					"overwrite_exclusions":  before.OverwriteExclusions,
				},
				"after": map[string]any{
					"prevent_tag_overwrite": updated.PreventTagOverwrite,
					"overwrite_exclusions":  updated.OverwriteExclusions,
				},
				"version": updated.Version,
			},
		})
	}
	return ctx.Success(toRegistryPolicyView(updated))
}

func toRegistryPolicyView(p *biz.RegistryPolicy) RegistryPolicyView {
	updatedAt := int64(0)
	if !p.UpdatedAt.Equal(time.Time{}) {
		updatedAt = p.UpdatedAt.Unix()
	}
	return RegistryPolicyView{
		PreventTagOverwrite: p.PreventTagOverwrite,
		OverwriteExclusions: p.OverwriteExclusions,
		Version:             p.Version,
		UpdatedAt:           updatedAt,
		UpdatedBy:           p.UpdatedBy,
	}
}
