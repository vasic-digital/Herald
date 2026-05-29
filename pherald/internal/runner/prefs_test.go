package runner

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/vasic-digital/herald/commons"
)

// evWithCategory builds a CloudEvent envelope with a given workflow Type
// and heraldcategory extension for HRD-154 filter tests.
func evWithCategory(typ, category string) commons.CloudEventEnvelope {
	return commons.CloudEventEnvelope{
		SpecVersion: "1.0",
		ID:          "01890000-0000-7000-8000-000000000000",
		Source:      "//test/source",
		Type:        typ,
		Extensions:  map[string]string{categoryExtension: category},
	}
}

func subWithPrefs(name, tz string, p *commons.PreferenceSet, aliases ...commons.SubscriberAlias) commons.Subscriber {
	return commons.Subscriber{
		ID:          uuid.New(),
		DisplayName: name,
		Timezone:    tz,
		Preferences: p,
		Aliases:     aliases,
	}
}

func recipChannels(rs []commons.Recipient) map[string]bool {
	m := map[string]bool{}
	for _, r := range rs {
		m[r.Channel] = true
	}
	return m
}

// ── Opt-in by category ────────────────────────────────────────────────

func TestFilterByPreferences_CategoryOptIn(t *testing.T) {
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC) // noon, not quiet
	sub := subWithPrefs("alice", "UTC", &commons.PreferenceSet{
		Categories: map[string]commons.CategoryPref{
			"deploys": {Channels: []commons.ChannelID{commons.ChannelTelegram}},
		},
	}, commons.SubscriberAlias{Channel: "tgram", ChannelUserID: "100"})

	// Event in opted-in category "deploys" → tgram included.
	got := filterByPreferences([]commons.Subscriber{sub}, evWithCategory("x.deploy.finished", "deploys"), now)
	if len(got) != 1 || got[0].Channel != "tgram" {
		t.Fatalf("opted-in category: got %v, want one tgram recipient", got)
	}

	// Event in a DIFFERENT category they didn't opt into → excluded.
	got = filterByPreferences([]commons.Subscriber{sub}, evWithCategory("x.incident.opened", "incidents"), now)
	if len(got) != 0 {
		t.Fatalf("non-opted category: got %v, want zero recipients", got)
	}
}

// ── Workflow opt-in overrides category ────────────────────────────────

func TestFilterByPreferences_WorkflowOptIn(t *testing.T) {
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	sub := subWithPrefs("alice", "UTC", &commons.PreferenceSet{
		Workflows: map[string]commons.WorkflowPref{
			"x.deploy.finished": {Channels: []commons.ChannelID{commons.ChannelSlack}},
		},
	},
		commons.SubscriberAlias{Channel: "tgram", ChannelUserID: "100"},
		commons.SubscriberAlias{Channel: "slack", ChannelUserID: "U1"},
	)
	got := filterByPreferences([]commons.Subscriber{sub}, evWithCategory("x.deploy.finished", "deploys"), now)
	ch := recipChannels(got)
	if !ch["slack"] || ch["tgram"] {
		t.Fatalf("workflow opt-in: got channels %v, want slack-only", ch)
	}
}

// ── Muted ─────────────────────────────────────────────────────────────

func TestFilterByPreferences_Muted(t *testing.T) {
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)

	// Category-muted subscriber → excluded entirely.
	catMuted := subWithPrefs("alice", "UTC", &commons.PreferenceSet{
		Categories: map[string]commons.CategoryPref{
			"deploys": {Channels: []commons.ChannelID{commons.ChannelTelegram}, Muted: true},
		},
	}, commons.SubscriberAlias{Channel: "tgram", ChannelUserID: "100"})
	if got := filterByPreferences([]commons.Subscriber{catMuted}, evWithCategory("x.deploy.finished", "deploys"), now); len(got) != 0 {
		t.Fatalf("category-muted: got %v, want zero", got)
	}

	// Workflow-muted subscriber → excluded even though category opts in.
	wfMuted := subWithPrefs("bob", "UTC", &commons.PreferenceSet{
		Categories: map[string]commons.CategoryPref{
			"deploys": {Channels: []commons.ChannelID{commons.ChannelTelegram}},
		},
		Workflows: map[string]commons.WorkflowPref{
			"x.deploy.finished": {Muted: true},
		},
	}, commons.SubscriberAlias{Channel: "tgram", ChannelUserID: "200"})
	if got := filterByPreferences([]commons.Subscriber{wfMuted}, evWithCategory("x.deploy.finished", "deploys"), now); len(got) != 0 {
		t.Fatalf("workflow-muted: got %v, want zero", got)
	}
}

// ── Quiet hours (non-UTC timezone to prove TZ handling) ───────────────

func TestFilterByPreferences_QuietHours_NonUTC(t *testing.T) {
	// Subscriber in Asia/Kolkata (UTC+05:30, no DST). Quiet window
	// 22:00–07:00 local. We inject UTC instants and assert the local
	// conversion is what gates suppression.
	sub := subWithPrefs("ravi", "Asia/Kolkata", &commons.PreferenceSet{
		Categories: map[string]commons.CategoryPref{
			"deploys": {Channels: []commons.ChannelID{commons.ChannelTelegram}},
		},
		QuietHours: &commons.QuietHours{Start: "22:00", End: "07:00"},
	}, commons.SubscriberAlias{Channel: "tgram", ChannelUserID: "100"})

	ev := evWithCategory("x.deploy.finished", "deploys")

	// 18:00 UTC == 23:30 IST → INSIDE quiet window → excluded.
	inside := time.Date(2026, 5, 20, 18, 0, 0, 0, time.UTC)
	if got := filterByPreferences([]commons.Subscriber{sub}, ev, inside); len(got) != 0 {
		t.Fatalf("inside quiet window (23:30 IST): got %v, want zero", got)
	}

	// 06:00 UTC == 11:30 IST → OUTSIDE quiet window → included.
	outside := time.Date(2026, 5, 20, 6, 0, 0, 0, time.UTC)
	if got := filterByPreferences([]commons.Subscriber{sub}, ev, outside); len(got) != 1 {
		t.Fatalf("outside quiet window (11:30 IST): got %v, want one", got)
	}

	// Prove TZ matters: the SAME UTC instant that is OUTSIDE the IST
	// window would be INSIDE a UTC window. 06:00 UTC is 06:00 local in
	// UTC, which IS within 22:00–07:00.
	utcSub := subWithPrefs("zoe", "UTC", &commons.PreferenceSet{
		Categories: map[string]commons.CategoryPref{
			"deploys": {Channels: []commons.ChannelID{commons.ChannelTelegram}},
		},
		QuietHours: &commons.QuietHours{Start: "22:00", End: "07:00"},
	}, commons.SubscriberAlias{Channel: "tgram", ChannelUserID: "200"})
	if got := filterByPreferences([]commons.Subscriber{utcSub}, ev, outside); len(got) != 0 {
		t.Fatalf("06:00 UTC sub should be inside UTC quiet window: got %v, want zero", got)
	}
}

// ── Quiet-hours exempt category overrides suppression ─────────────────

func TestFilterByPreferences_QuietHoursExemptCategory(t *testing.T) {
	sub := subWithPrefs("ops", "UTC", &commons.PreferenceSet{
		Categories: map[string]commons.CategoryPref{
			"incidents": {Channels: []commons.ChannelID{commons.ChannelTelegram}},
		},
		QuietHours: &commons.QuietHours{Start: "22:00", End: "07:00", ExemptCategories: []string{"incidents"}},
	}, commons.SubscriberAlias{Channel: "tgram", ChannelUserID: "100"})

	// 23:00 UTC → inside quiet window, but "incidents" is exempt → included.
	night := time.Date(2026, 5, 20, 23, 0, 0, 0, time.UTC)
	if got := filterByPreferences([]commons.Subscriber{sub}, evWithCategory("x.incident.opened", "incidents"), night); len(got) != 1 {
		t.Fatalf("exempt category during quiet hours: got %v, want one", got)
	}
}

// ── No PreferenceSet → all-aliases regression ─────────────────────────

func TestFilterByPreferences_NoPreferenceSet_AllAliases(t *testing.T) {
	now := time.Date(2026, 5, 20, 2, 0, 0, 0, time.UTC) // would be quiet IF prefs existed
	sub := commons.Subscriber{
		DisplayName: "legacy",
		Preferences: nil, // no prefs at all
		Aliases: []commons.SubscriberAlias{
			{Channel: "tgram", ChannelUserID: "100"},
			{Channel: "slack", ChannelUserID: "U1"},
		},
	}
	got := filterByPreferences([]commons.Subscriber{sub}, evWithCategory("x.any.thing", "whatever"), now)
	if len(got) != 2 {
		t.Fatalf("no-PreferenceSet: got %d recipients, want 2 (all aliases)", len(got))
	}
}

// ── PreferenceSet present but no matching pref → excluded ─────────────

func TestFilterByPreferences_PrefsButNoMatch_Excluded(t *testing.T) {
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	sub := subWithPrefs("alice", "UTC", &commons.PreferenceSet{
		Categories: map[string]commons.CategoryPref{
			"deploys": {Channels: []commons.ChannelID{commons.ChannelTelegram}},
		},
	}, commons.SubscriberAlias{Channel: "tgram", ChannelUserID: "100"})

	// Event category "unknown" has no pref → opt-in model → excluded.
	if got := filterByPreferences([]commons.Subscriber{sub}, evWithCategory("x.other", "unknown"), now); len(got) != 0 {
		t.Fatalf("prefs-but-no-match: got %v, want zero", got)
	}
}

// ── Malformed quiet-hours window fails open ───────────────────────────

func TestInQuietHours_MalformedFailsOpen(t *testing.T) {
	now := time.Date(2026, 5, 20, 23, 0, 0, 0, time.UTC)
	// Bad HH:MM → not suppressed (fail-open).
	if inQuietHours(&commons.QuietHours{Start: "2500", End: "07:00"}, "UTC", now) {
		t.Fatal("malformed Start should fail open (not quiet)")
	}
	// Bad TZ → not suppressed (fail-open).
	if inQuietHours(&commons.QuietHours{TZ: "Mars/Olympus", Start: "22:00", End: "07:00"}, "UTC", now) {
		t.Fatal("unknown TZ should fail open (not quiet)")
	}
}

// ── Process-level wiring: injected FakeClock drives quiet-hours ───────

func TestSubscriberResolver_Process_QuietHoursFiltersAlias(t *testing.T) {
	tenantID := mustParse("33333333-3333-3333-3333-333333333333")
	store := newFakeSubscribersStore()
	store.Add(tenantID, subscriberRow{
		ID: uuid.New(), Handle: "ravi", DisplayName: "Ravi",
		Timezone: "Asia/Kolkata",
		Preferences: &commons.PreferenceSet{
			Categories: map[string]commons.CategoryPref{
				"deploys": {Channels: []commons.ChannelID{commons.ChannelTelegram}},
			},
			QuietHours: &commons.QuietHours{Start: "22:00", End: "07:00"},
		},
		Aliases: []subscriberAliasRow{{Channel: "tgram", ChannelUserID: "100"}},
	})

	// FakeClock anchored at 2026-05-20 12:00 UTC == 17:30 IST → NOT quiet.
	clk := commons.NewFakeClock()
	r := &SubscriberResolver{Subscribers: store, Clock: clk}

	rc := &RunCtx{TenantID: tenantID, Event: evWithCategory("x.deploy.finished", "deploys")}
	rc.TenantPGCtx = withTenantCtx(context.Background(), tenantID)
	if err := r.Process(context.Background(), rc); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(rc.Recipients) != 1 {
		t.Fatalf("12:00 UTC (17:30 IST, not quiet): got %d recipients, want 1", len(rc.Recipients))
	}

	// Advance to 18:00 UTC == 23:30 IST → INSIDE quiet window → excluded.
	clk.Advance(6 * time.Hour)
	rc2 := &RunCtx{TenantID: tenantID, Event: evWithCategory("x.deploy.finished", "deploys")}
	rc2.TenantPGCtx = withTenantCtx(context.Background(), tenantID)
	if err := r.Process(context.Background(), rc2); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(rc2.Recipients) != 0 {
		t.Fatalf("18:00 UTC (23:30 IST, quiet): got %d recipients, want 0", len(rc2.Recipients))
	}
}
