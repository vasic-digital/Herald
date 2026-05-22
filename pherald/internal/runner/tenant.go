package runner

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// TenantResolver is Stage 3 — binds the tenant UUID into a context.Context
// the downstream stages will pass to PG queries (where RLS policies key
// off `app.tenant_id` GUC, which commons_storage.WithTenantContext sets).
//
// Note: in Wave 3b the actual GUC-setting happens inside commons_storage
// when SubscriberResolver / OutcomeRecorder issue queries; this stage's
// job is to ensure the ctx carries the resolved tenant so downstream
// don't need to reach back into RunCtx.
type TenantResolver struct{}

// tenantCtxKey is the context key for the resolved tenant UUID. Stage
// 5+6+7 read it via ctx.Value(tenantCtxKey{}).
type tenantCtxKey struct{}

func (r *TenantResolver) Process(ctx context.Context, rc *RunCtx) error {
	if rc.TenantID == uuid.Nil {
		return fmt.Errorf("tenant_resolver: TenantID is uuid.Nil (claim extraction failed?)")
	}
	rc.TenantPGCtx = context.WithValue(ctx, tenantCtxKey{}, rc.TenantID)
	return nil
}

// TenantFromCtx is exported so downstream stages can extract the tenant
// without circular references. Returns uuid.Nil if not set.
func TenantFromCtx(ctx context.Context) uuid.UUID {
	v, _ := ctx.Value(tenantCtxKey{}).(uuid.UUID)
	return v
}
