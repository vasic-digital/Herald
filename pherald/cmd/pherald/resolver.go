// resolver.go — GAP G1/G2 (§11.4.104 participant-identity wiring for
// `pherald listen`).
//
// Prior to this, `loadListenConfigFromEnv` built the inbound.Dispatcher WITHOUT
// a Resolver, so created_by/assigned_to attribution (PARTICIPANT_ATTRIBUTION
// §2) and the §110 Tier-3 clarify-tag (`@username`) resolution were silently
// skipped for ALL channels — Slack AND Telegram. This file constructs the
// production resolver consumed by `cfg.Resolver`.
//
// Resolver choice — ENV-ONLY (not PG-backed) — and why:
//
// The `pherald listen` path does NOT build a Postgres pool. The workable-items
// store it opens (commons_workable.Open) is the SQLite SSoT, not the PG
// `subscribers`/`subscriber_aliases` roster that a full PG-backed resolver
// would query. Standing up a PG pool here purely for alias lookup would be a
// large, network-dependent change well outside this gap's scope (and the §107
// fail-loud contract would force `listen` to refuse to boot when PG is
// unreachable — a regression for the Telegram-only operator who has no PG at
// all). Per §11.4.74 catalogue-first we therefore reuse the existing
// commons.MemoryResolver + commons.OperatorHandleFromEnv primitives and wire an
// env-only resolver that:
//
//   - resolves the operator's canonical handle per channel from
//     HERALD_<CHANNEL>_OPERATOR_USERNAME (HERALD_TGRAM_OPERATOR_USERNAME,
//     HERALD_SLACK_OPERATOR_USERNAME, …);
//   - lets the operator be @-tagged on every enabled channel (UsernameFor);
//   - resolves first-contact senders to their raw @username (MemoryResolver's
//     documented fallback) so attribution still works without a roster.
//
// TODO(HRD-PG-ALIAS): a full PG-backed IdentityResolver that loads the
// `subscribers` + `subscriber_aliases` roster (so a non-operator participant
// with a known cross-channel alias is tagged on the correct channel) is
// deferred until the listen path grows a PG pool (tracked alongside the
// HRD-156 PG subscriber-resolution e2e). Until then non-operator senders are
// attributed/tagged by their raw per-channel @username, which is correct for
// single-channel use and a safe degradation cross-channel (a participant is
// simply tagged by the handle Herald actually saw).
package main

import "github.com/vasic-digital/herald/commons"

// buildResolver constructs the env-only participant IdentityResolver for the
// given enabled channel set (GAP G1/G2).
//
// The operator's canonical handle is taken from the PRIMARY channel
// (Telegram when enabled, else the first enabled channel) per
// PARTICIPANT_ATTRIBUTION §1 ("defaults to their Telegram @username since
// Telegram is the primary messenger"). The operator is registered as a normal
// Participant whose Usernames map carries the per-channel
// HERALD_<CHANNEL>_OPERATOR_USERNAME value for each enabled channel, so
// UsernameFor(operatorHandle, "slack") resolves the operator's Slack handle
// too (the operator is never tagged, but the alias makes the roster complete +
// keeps OperatorHandle() per-channel-consistent).
//
// Always returns a non-nil resolver — even when no operator env var is set the
// MemoryResolver still resolves first-contact senders by their raw @username,
// which is the §2 unknown-sender behaviour. Returning non-nil unconditionally
// is what flips attribution + clarify-tagging ON for the listen path.
func buildResolver(enabled []string) commons.IdentityResolver {
	primary := primaryChannel(enabled)
	operatorHandle := commons.OperatorHandleFromEnv(primary)

	usernames := map[string]string{}
	for _, ch := range enabled {
		if u := commons.OperatorHandleFromEnv(ch); u != "" {
			usernames[ch] = u
		}
	}

	var participants []commons.Participant
	if operatorHandle != "" {
		participants = append(participants, commons.Participant{
			Handle:      operatorHandle,
			DisplayName: "Operator",
			Kind:        "human",
			Usernames:   usernames,
		})
	}
	return commons.NewMemoryResolver(operatorHandle, participants)
}

// primaryChannel picks the channel whose HERALD_<CHANNEL>_OPERATOR_USERNAME
// supplies the operator's canonical handle: Telegram when it is in the enabled
// set (the §1 primary messenger), otherwise the first enabled channel, else
// "tgram" as a last resort (matches loadEnabledChannels' default).
func primaryChannel(enabled []string) string {
	for _, ch := range enabled {
		if ch == string(commons.ChannelTelegram) {
			return ch
		}
	}
	if len(enabled) > 0 {
		return enabled[0]
	}
	return string(commons.ChannelTelegram)
}
