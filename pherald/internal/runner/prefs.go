package runner

import (
	"time"

	"github.com/vasic-digital/herald/commons"
)

// HRD-154 (ATMOSphere integration WS-3) — per-subscriber preference +
// quiet-hours routing for Stage 5 (SubscriberResolver).
//
// Before HRD-154, Stage 5 resolved EVERY alias of EVERY subscriber in a
// tenant (tenant-scoped fan-out), ignoring the PreferenceSet/QuietHours
// the data model already carried. filterByPreferences is the pure,
// PG-free core that decides, per (subscriber, alias), whether that
// concrete delivery endpoint should receive a given event.
//
// ── Precedence (highest wins; first rule that fires decides) ──────────
//
//	1. Muted          — if the subscriber is muted for the event's
//	                     workflow (CloudEvents Type) OR its category, the
//	                     channel is EXCLUDED. Muted is the strongest
//	                     negative signal (spec §7.2 line 862: "if muted
//	                     for the workflow/category, drop").
//	2. QuietHours     — if `now` (evaluated in the subscriber's timezone)
//	                     falls inside the configured quiet window, the
//	                     channel is EXCLUDED — UNLESS the event's category
//	                     is in QuietHours.ExemptCategories (spec §7.3 +
//	                     §44 "Quiet hours override only for
//	                     severity=critical + category=incidents"). A
//	                     quiet-hours suppression DROPS the notification
//	                     for that channel for this event (see caveat
//	                     below — v1 is drop, not defer).
//	3. Workflow opt-in — if the PreferenceSet has a WorkflowPref for the
//	                     event's Type, ONLY the channels it lists are
//	                     included (empty list ⇒ fall through to the
//	                     category default, per the struct comment on
//	                     WorkflowPref.Channels).
//	4. Category opt-in — else if the PreferenceSet has a CategoryPref for
//	                     the event's category, ONLY the channels it lists
//	                     are included (empty list ⇒ no opt-in ⇒ EXCLUDED).
//	5. Default        — a subscriber with NO PreferenceSet (Preferences ==
//	                     nil) preserves the pre-HRD-154 behaviour: ALL of
//	                     that subscriber's aliases are resolved. This keeps
//	                     existing single-tenant→single-chat callers from
//	                     regressing. A subscriber WITH a PreferenceSet but
//	                     no matching workflow/category pref is treated as
//	                     "no opt-in" ⇒ EXCLUDED (explicit opt-in model).
//
// ── Category source ───────────────────────────────────────────────────
//
// The event's category is carried as the `heraldcategory` CloudEvent
// extension (spec §42.1.1 three-axis envelope precedent — extensions
// carry routing axes). The event's Type is the CloudEvents reverse-DNS
// type and keys directly into PreferenceSet.Workflows.
const categoryExtension = "heraldcategory"

// eventCategory extracts the routing category from a CloudEvent envelope.
// Empty string when no `heraldcategory` extension is present.
func eventCategory(ev commons.CloudEventEnvelope) string {
	if ev.Extensions == nil {
		return ""
	}
	return ev.Extensions[categoryExtension]
}

// filterByPreferences is the pure (PG-free, clock-injected) core of
// HRD-154. Given the tenant's subscribers, the event, and the current
// instant `now`, it returns the recipients that should actually receive
// the event after preference + mute + quiet-hours filtering.
//
// `now` is supplied by the caller (Stage 5 passes commons.Clock.Now())
// so tests inject a deterministic instant. The function never calls
// time.Now() itself.
func filterByPreferences(subs []commons.Subscriber, ev commons.CloudEventEnvelope, now time.Time) []commons.Recipient {
	category := eventCategory(ev)
	var out []commons.Recipient
	for _, sub := range subs {
		// Rule 5 (default): no PreferenceSet ⇒ resolve all aliases.
		if sub.Preferences == nil {
			for _, a := range sub.Aliases {
				out = append(out, commons.Recipient{
					Channel:       a.Channel,
					ChannelUserID: a.ChannelUserID,
					DisplayName:   sub.DisplayName,
				})
			}
			continue
		}
		for _, a := range sub.Aliases {
			if channelAllowed(sub, ev.Type, category, commons.ChannelID(a.Channel), now) {
				out = append(out, commons.Recipient{
					Channel:       a.Channel,
					ChannelUserID: a.ChannelUserID,
					DisplayName:   sub.DisplayName,
				})
			}
		}
	}
	return out
}

// channelAllowed applies the precedence rules to a single
// (subscriber, channel) pair. The subscriber is guaranteed to have a
// non-nil PreferenceSet by the caller.
func channelAllowed(sub commons.Subscriber, workflowType, category string, ch commons.ChannelID, now time.Time) bool {
	prefs := sub.Preferences

	wf, hasWF := prefs.Workflows[workflowType]
	cat, hasCat := prefs.Categories[category]

	// Rule 1 — Muted (workflow OR category). Strongest signal.
	if (hasWF && wf.Muted) || (hasCat && cat.Muted) {
		return false
	}

	// Rule 2 — QuietHours (unless the category is exempt).
	if prefs.QuietHours != nil && !categoryExempt(prefs.QuietHours, category) {
		if inQuietHours(prefs.QuietHours, sub.Timezone, now) {
			return false
		}
	}

	// Rule 3 — Workflow opt-in (non-empty channel list is authoritative).
	if hasWF && len(wf.Channels) > 0 {
		return channelInList(ch, wf.Channels)
	}

	// Rule 4 — Category opt-in.
	if hasCat {
		// Empty channel list ⇒ no concrete opt-in ⇒ excluded.
		return channelInList(ch, cat.Channels)
	}

	// Rule 5 (within a PreferenceSet) — no matching workflow/category
	// pref ⇒ explicit opt-in model ⇒ excluded.
	return false
}

func channelInList(ch commons.ChannelID, list []commons.ChannelID) bool {
	for _, c := range list {
		if c == ch {
			return true
		}
	}
	return false
}

func categoryExempt(qh *commons.QuietHours, category string) bool {
	if category == "" {
		return false
	}
	for _, c := range qh.ExemptCategories {
		if c == category {
			return true
		}
	}
	return false
}

// inQuietHours reports whether `now`, evaluated in the subscriber's
// timezone (QuietHours.TZ takes precedence, then the subscriber's
// Timezone, then UTC), falls within the [Start, End) window.
//
// Windows that wrap past midnight (Start > End, e.g. 22:00→07:00) are
// handled: the in-window test becomes (t >= Start || t < End). A
// malformed TZ or HH:MM string fails CLOSED (returns false — i.e. NOT
// quiet hours, so the notification is NOT suppressed) so a config typo
// never silently swallows alerts.
func inQuietHours(qh *commons.QuietHours, subTZ string, now time.Time) bool {
	tzName := qh.TZ
	if tzName == "" {
		tzName = subTZ
	}
	if tzName == "" {
		tzName = "UTC"
	}
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return false // fail-open: unknown TZ ⇒ do not suppress
	}
	local := now.In(loc)
	cur := local.Hour()*60 + local.Minute()

	start, ok1 := parseHHMM(qh.Start)
	end, ok2 := parseHHMM(qh.End)
	if !ok1 || !ok2 {
		return false // fail-open: malformed window ⇒ do not suppress
	}
	if start == end {
		return false // zero-width window ⇒ never quiet
	}
	if start < end {
		return cur >= start && cur < end
	}
	// Wrap-past-midnight window (e.g. 22:00 → 07:00).
	return cur >= start || cur < end
}

// parseHHMM parses "HH:MM" 24h into minutes-since-midnight. Returns
// (_, false) on any malformed input.
func parseHHMM(s string) (int, bool) {
	if len(s) != 5 || s[2] != ':' {
		return 0, false
	}
	h := int(s[0]-'0')*10 + int(s[1]-'0')
	m := int(s[3]-'0')*10 + int(s[4]-'0')
	if s[0] < '0' || s[0] > '9' || s[1] < '0' || s[1] > '9' ||
		s[3] < '0' || s[3] > '9' || s[4] < '0' || s[4] > '9' {
		return 0, false
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}
