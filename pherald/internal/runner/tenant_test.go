package runner

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestTenantResolver_SetsTenantPGCtx(t *testing.T) {
	tenantID := mustParse("22222222-2222-2222-2222-222222222222")
	r := &TenantResolver{}
	rc := &RunCtx{TenantID: tenantID}
	if err := r.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if rc.TenantPGCtx == nil {
		t.Fatal("TenantPGCtx is nil after Process")
	}
	got, ok := rc.TenantPGCtx.Value(tenantCtxKey{}).(uuid.UUID)
	if !ok {
		t.Fatal("TenantPGCtx missing tenantCtxKey")
	}
	if got != tenantID {
		t.Errorf("ctx tenantID = %s, want %s", got, tenantID)
	}
}

func TestTenantResolver_NilTenantID_Errors(t *testing.T) {
	r := &TenantResolver{}
	rc := &RunCtx{} // TenantID = uuid.Nil
	if err := r.Process(context.Background(), rc); err == nil {
		t.Fatal("expected error for nil TenantID")
	}
}
