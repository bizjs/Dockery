package data

import (
	"context"
	"time"

	"api/internal/biz"
	"api/internal/data/ent"
	"api/internal/data/ent/auditlog"

	"github.com/go-kratos/kratos/v2/log"
)

type auditRepo struct {
	data *Data
	log  *log.Helper
}

// NewAuditRepo adapts ent's AuditLog client to biz.AuditRepo.
func NewAuditRepo(d *Data, logger log.Logger) biz.AuditRepo {
	return &auditRepo{data: d, log: log.NewHelper(log.With(logger, "module", "data/audit"))}
}

func (r *auditRepo) Create(ctx context.Context, e *biz.AuditEntry) error {
	creator := r.data.DB().AuditLog.Create().
		SetActor(nonEmpty(e.Actor, "anonymous")).
		SetAction(e.Action).
		SetSuccess(e.Success)
	if e.Ts > 0 {
		creator = creator.SetTs(time.Unix(e.Ts, 0))
	}
	if e.Target != "" {
		creator = creator.SetTarget(e.Target)
	}
	if e.Scope != "" {
		creator = creator.SetScope(e.Scope)
	}
	if e.ClientIP != "" {
		creator = creator.SetClientIP(e.ClientIP)
	}
	if len(e.Detail) > 0 {
		creator = creator.SetDetail(e.Detail)
	}
	_, err := creator.Save(ctx)
	return err
}

func (r *auditRepo) Query(ctx context.Context, f biz.AuditFilter) ([]*biz.AuditEntry, int, error) {
	q := r.data.DB().AuditLog.Query()
	if f.Actor != "" {
		q = q.Where(auditlog.ActorContainsFold(f.Actor))
	}
	if f.Action != "" {
		q = q.Where(auditlog.ActionEQ(f.Action))
	}
	if f.Since > 0 {
		q = q.Where(auditlog.TsGTE(time.Unix(f.Since, 0)))
	}
	if f.Until > 0 {
		q = q.Where(auditlog.TsLTE(time.Unix(f.Until, 0)))
	}

	// Count first so callers can render pagination; do it before the
	// Limit/Offset application.
	total, err := q.Clone().Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	rows, err := q.
		Order(ent.Desc(auditlog.FieldTs), ent.Desc(auditlog.FieldID)).
		Limit(f.Limit).
		Offset(f.Offset).
		All(ctx)
	if err != nil {
		return nil, 0, err
	}

	out := make([]*biz.AuditEntry, 0, len(rows))
	for _, row := range rows {
		out = append(out, &biz.AuditEntry{
			ID:       int64(row.ID),
			Ts:       row.Ts.Unix(),
			Actor:    row.Actor,
			Action:   row.Action,
			Target:   row.Target,
			Scope:    row.Scope,
			ClientIP: row.ClientIP,
			Success:  row.Success,
			Detail:   row.Detail,
		})
	}
	return out, total, nil
}

func nonEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

var _ biz.AuditRepo = (*auditRepo)(nil)
