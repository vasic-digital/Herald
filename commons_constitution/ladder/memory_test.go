package ladder

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/vasic-digital/herald/commons_constitution"
)

func TestMemory_DefaultIsEnforce(t *testing.T) {
	m := NewMemory()
	tenant := uuid.New()
	got, err := m.Get(context.Background(), tenant, "§11.4.10")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != constitution.ModeEnforce {
		t.Errorf("default for unbound rule = %v; want ModeEnforce (safe default)", got)
	}
}

func TestMemory_SetThenGetRoundtrips(t *testing.T) {
	m := NewMemory()
	tenant := uuid.New()
	ctx := context.Background()

	if err := m.Set(ctx, tenant, "§A", constitution.ModeWarn, "ops@example"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := m.Get(ctx, tenant, "§A")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != constitution.ModeWarn {
		t.Errorf("after Set(warn), Get = %v; want ModeWarn", got)
	}
}

func TestMemory_TenantIsolation(t *testing.T) {
	m := NewMemory()
	a, b := uuid.New(), uuid.New()
	ctx := context.Background()

	_ = m.Set(ctx, a, "§A", constitution.ModeAllow, "ops")

	got, _ := m.Get(ctx, b, "§A")
	if got != constitution.ModeEnforce {
		t.Errorf("tenant B saw tenant A's binding: got %v; want ModeEnforce (default)", got)
	}
}

func TestMemory_List(t *testing.T) {
	m := NewMemory()
	tenant := uuid.New()
	ctx := context.Background()

	_ = m.Set(ctx, tenant, "§A", constitution.ModeAllow, "ops")
	_ = m.Set(ctx, tenant, "§B", constitution.ModeWarn, "ops")

	list, err := m.List(ctx, tenant)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List returned %d entries; want 2", len(list))
	}
	if list["§A"] != constitution.ModeAllow || list["§B"] != constitution.ModeWarn {
		t.Errorf("List returned wrong bindings: %v", list)
	}
}

func TestMemory_MutationsAudit(t *testing.T) {
	m := NewMemory()
	tenant := uuid.New()
	ctx := context.Background()

	_ = m.Set(ctx, tenant, "§A", constitution.ModeAllow, "ops")
	_ = m.Set(ctx, tenant, "§B", constitution.ModeWarn, "sre")
	_ = m.Set(ctx, tenant, "§A", constitution.ModeEnforce, "ops") // overwrite

	muts := m.Mutations()
	if len(muts) != 3 {
		t.Fatalf("Mutations() returned %d; want 3 (including overwrite)", len(muts))
	}
	if muts[0].By != "ops" || muts[1].By != "sre" || muts[2].By != "ops" {
		t.Errorf("By attribution not preserved: %v", muts)
	}
	if muts[2].Mode != constitution.ModeEnforce {
		t.Errorf("audit overwrite Mode = %v; want ModeEnforce", muts[2].Mode)
	}
}

func TestMemory_ListReturnsCopy(t *testing.T) {
	m := NewMemory()
	tenant := uuid.New()
	ctx := context.Background()
	_ = m.Set(ctx, tenant, "§A", constitution.ModeAllow, "ops")

	list, _ := m.List(ctx, tenant)
	list["§Z"] = constitution.ModeEnforce // mutate the returned map

	got, _ := m.Get(ctx, tenant, "§Z")
	if got != constitution.ModeEnforce {
		// Wait — default is ModeEnforce. The right check: §Z must NOT be a real binding.
		// Use List again to confirm.
	}
	freshList, _ := m.List(ctx, tenant)
	if _, ok := freshList["§Z"]; ok {
		t.Errorf("List didn't return a copy — caller mutation leaked back")
	}
}
