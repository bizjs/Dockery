package service

import (
	"context"
	"errors"
	"net/http"
	"time"

	"api/internal/biz"

	"github.com/bizjs/kratoscarf/response"
	"github.com/bizjs/kratoscarf/router"
)

type RegistryPolicyService struct {
	policy *biz.RegistryPolicyUsecase
	audit  *biz.AuditUsecase
}

func NewRegistryPolicyService(policy *biz.RegistryPolicyUsecase, audit *biz.AuditUsecase) *RegistryPolicyService {
	return &RegistryPolicyService{policy: policy, audit: audit}
}

type RegistryPolicyView struct {
	PreventTagOverwrite bool   `json:"prevent_tag_overwrite"`
	Version             int64  `json:"version"`
	UpdatedAt           int64  `json:"updated_at"`
	UpdatedBy           string `json:"updated_by"`
}

type UpdateRegistryPolicyRequest struct {
	PreventTagOverwrite *bool  `json:"prevent_tag_overwrite" validate:"required"`
	Version             *int64 `json:"version" validate:"required,min=0"`
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
	updated, err := s.policy.Update(ctx.Context(), *req.Version, *req.PreventTagOverwrite, actor)
	if err != nil {
		switch {
		case errors.Is(err, biz.ErrRegistryPolicyConflict):
			return response.NewBizError(http.StatusConflict, 40902, "registry policy was changed by another administrator")
		case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
			return response.NewBizError(http.StatusServiceUnavailable, 50301, "registry policy switch timed out")
		default:
			return response.ErrInternal.WithCause(err)
		}
	}

	if before.PreventTagOverwrite != updated.PreventTagOverwrite {
		s.audit.Write(ctx.Context(), biz.AuditEntry{
			Actor:    actor,
			Action:   biz.ActionRegistryPolicyUpdated,
			Target:   "tag-overwrite-protection",
			ClientIP: ctx.ClientIP(),
			Success:  true,
			Detail: map[string]any{
				"before":  before.PreventTagOverwrite,
				"after":   updated.PreventTagOverwrite,
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
		Version:             p.Version,
		UpdatedAt:           updatedAt,
		UpdatedBy:           p.UpdatedBy,
	}
}
