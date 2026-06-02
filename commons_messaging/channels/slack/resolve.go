package slack

import "context"

// userHandleCache memoizes Slack user-id → display-handle resolutions so the
// inbound hot path (every thread-context message + every inbound sender) does
// not re-dial users.info per event. It mirrors the BotSelfIdentity caching
// pattern (selfMu/selfID in slack.go) but is keyed by user-id because a thread
// can carry many distinct authors.
//
// The zero value is NOT usable; resolveUserHandle lazily initializes the map
// under userMu so callers never need an explicit constructor step.
//
// Negative resolutions (empty/unknown id, or a users.info error) are NOT
// cached — they fall back deterministically to the raw id, and a transient
// API error should not poison the cache for the lifetime of the adapter.

// resolveUserHandle maps a Slack user-id (e.g. "U0123ABC") to a human-readable
// "@handle" for display/attribution. Resolution order, most→least preferred:
//
//	@<name>                — Slack's account handle (UserProfile/User.Name); the
//	                         canonical messenger @username for §11.4.104 tagging.
//	@<display_name>        — the workspace display name when Name is empty.
//	@<real_name>           — the full name as a last resort before the raw id.
//	<userID> (unchanged)   — deterministic fallback when users.info yields
//	                         nothing usable (or errors, or the id is empty).
//
// It is best-effort and NON-FATAL by design: the inbound path must never drop a
// message because a handle could not be resolved, so a users.info error simply
// yields the raw id (the previous behaviour) rather than propagating.
func (a *Adapter) resolveUserHandle(ctx context.Context, userID string) string {
	if userID == "" {
		return ""
	}
	a.userMu.Lock()
	if a.userHandles != nil {
		if cached, ok := a.userHandles[userID]; ok {
			a.userMu.Unlock()
			return cached
		}
	}
	a.userMu.Unlock()

	u, err := a.api.GetUserInfoContext(ctx, userID)
	if err != nil || u == nil {
		// Deterministic fallback — never drop the sender, never cache the miss.
		return userID
	}
	handle := bestUserHandle(u.Name, u.Profile.DisplayName, u.RealName, userID)

	a.userMu.Lock()
	if a.userHandles == nil {
		a.userHandles = make(map[string]string)
	}
	a.userHandles[userID] = handle
	a.userMu.Unlock()
	return handle
}

// bestUserHandle picks the most human-readable, "@"-prefixed handle from the
// fields a Slack users.info response exposes, falling back to the raw user-id
// (UNPREFIXED) when none is available. Splitting this out keeps the resolution
// policy unit-testable without a wire round-trip and makes the §107 paired
// assertion (drop the resolver → fall back to the raw id) explicit.
func bestUserHandle(name, displayName, realName, userID string) string {
	switch {
	case name != "":
		return "@" + name
	case displayName != "":
		return "@" + displayName
	case realName != "":
		return "@" + realName
	default:
		return userID
	}
}
