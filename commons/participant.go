// Participant identity model per docs/design/PARTICIPANT_ATTRIBUTION.md §1.
//
// A Participant is one logical person/agent (a Subscriber/User) who may carry a
// DIFFERENT @username on every messenger channel. Workable items store the
// canonical, messenger-neutral handle in their created_by / assigned_to fields;
// the IdentityResolver bridges between that canonical world and the per-channel
// runtime (inbound sender resolution + outbound @username lookup for tagging).
//
// This is the contract surface other streams (inbound, workflow, channel
// adapters, storage) code against — the Go signatures here are load-bearing and
// match the §1/§3 contract exactly.
package commons

import (
	"os"
	"strings"
)

// SystemAgentHandle is the reserved created_by/assigned_to sentinel for the
// system agent (Claude). It is NEVER @-tagged in any notification (it is the
// system, not a human participant). See §1 + §3.
const SystemAgentHandle = "Claude"

// Participant is a logical Subscriber/User: one person/agent with potentially a
// different username on every messenger. See §1.
type Participant struct {
	// Handle is the canonical, messenger-neutral handle stored in items'
	// created_by / assigned_to (e.g. "@milos85vasic" or "Claude").
	Handle string
	// DisplayName is the human-readable name.
	DisplayName string
	// Kind is "human" | "agent" | "service" (spec §7.5).
	Kind string
	// Usernames maps a channel name ("tgram", "slack", …) to the participant's
	// "@username" on that channel. A participant with no entry for a channel
	// cannot be @-tagged there.
	Usernames map[string]string
}

// IdentityResolver bridges the per-channel runtime world to canonical handles.
// See §1 contract.
type IdentityResolver interface {
	// ResolveSender maps a received message's sender to a canonical handle.
	// Unknown senders resolve to their raw "@username" (so first-contact users
	// are still attributable).
	ResolveSender(channel, channelUserID, username string) (handle string)
	// UsernameFor returns the @username for a canonical handle on a target
	// channel. ok=false if the participant has no alias on that channel —
	// you cannot tag someone who is not on that messenger.
	UsernameFor(handle, channel string) (username string, ok bool)
	// OperatorHandle returns the canonical operator handle (sourced from
	// HERALD_<CHANNEL>_OPERATOR_USERNAME).
	OperatorHandle() string
}

// MemoryResolver is a concrete in-memory IdentityResolver built from a
// participant list plus an operator handle. It is the resolver used by tests
// and by runtime paths that have loaded the participant roster into memory.
type MemoryResolver struct {
	operatorHandle string
	// byHandle indexes participants by canonical handle.
	byHandle map[string]Participant
	// bySenderKey indexes by "channel\x00channelUserID" and "channel\x00@username"
	// for fast inbound sender resolution.
	bySenderKey map[string]string
}

// memSep separates the composite-key fields. NUL is never part of a channel
// name, channel user id, or username, so it is a collision-free separator.
const memSep = "\x00"

// NewMemoryResolver builds a MemoryResolver from the given participant roster
// and operator handle. The operator handle is messenger-neutral (the operator
// is a normal Participant whose handle equals the operator env value); pass it
// from OperatorHandleFromEnv for the primary channel.
func NewMemoryResolver(operatorHandle string, participants []Participant) *MemoryResolver {
	r := &MemoryResolver{
		operatorHandle: operatorHandle,
		byHandle:       make(map[string]Participant, len(participants)),
		bySenderKey:    make(map[string]string),
	}
	for _, p := range participants {
		r.byHandle[p.Handle] = p
		for channel, username := range p.Usernames {
			// Index by (channel, @username) for username-based inbound match.
			if username != "" {
				r.bySenderKey[channel+memSep+normalizeUsername(username)] = p.Handle
			}
		}
	}
	return r
}

// AddSenderIndex registers a (channel, channelUserID) -> canonical handle
// mapping so a sender identified by chat/user id (not just @username) resolves.
// channelUserID is the chat/user id (subscriber_aliases.channel_user_id), which
// is distinct from the @username. Safe to call repeatedly.
func (r *MemoryResolver) AddSenderIndex(channel, channelUserID, handle string) {
	if channel == "" || channelUserID == "" || handle == "" {
		return
	}
	r.bySenderKey[channel+memSep+channelUserID] = handle
}

// ResolveSender implements IdentityResolver. It matches by (channel,
// channelUserID) first, then by (channel, @username). An unknown sender returns
// the raw "@username" (normalized to a single leading "@") so attribution still
// works for first-contact users.
func (r *MemoryResolver) ResolveSender(channel, channelUserID, username string) string {
	if channelUserID != "" {
		if h, ok := r.bySenderKey[channel+memSep+channelUserID]; ok {
			return h
		}
	}
	if username != "" {
		if h, ok := r.bySenderKey[channel+memSep+normalizeUsername(username)]; ok {
			return h
		}
		return normalizeUsername(username)
	}
	// No username and no known id: fall back to the raw channelUserID so the
	// event is still attributable to something stable.
	return channelUserID
}

// UsernameFor implements IdentityResolver. It returns the participant's
// @username on the target channel; ok=false if the handle is unknown or the
// participant has no alias on that channel.
func (r *MemoryResolver) UsernameFor(handle, channel string) (string, bool) {
	p, ok := r.byHandle[handle]
	if !ok {
		return "", false
	}
	username, ok := p.Usernames[channel]
	if !ok || username == "" {
		return "", false
	}
	return username, true
}

// OperatorHandle implements IdentityResolver.
func (r *MemoryResolver) OperatorHandle() string { return r.operatorHandle }

// OperatorHandleFromEnv reads HERALD_<CHANNEL>_OPERATOR_USERNAME for the given
// channel (e.g. channel="tgram" -> HERALD_TGRAM_OPERATOR_USERNAME) and returns
// the operator's canonical handle. The channel is upper-cased; the value is
// returned verbatim (trimmed) — an empty string means "no operator configured
// for this channel". Per §1 the Telegram operator username is the operator's
// canonical handle, so callers building the roster typically pass channel
// "tgram".
func OperatorHandleFromEnv(channel string) string {
	if channel == "" {
		return ""
	}
	key := "HERALD_" + strings.ToUpper(channel) + "_OPERATOR_USERNAME"
	return strings.TrimSpace(os.Getenv(key))
}

// normalizeUsername guarantees exactly one leading "@" on a non-empty username
// and trims surrounding whitespace. An empty input returns "".
func normalizeUsername(u string) string {
	u = strings.TrimSpace(u)
	if u == "" {
		return ""
	}
	return "@" + strings.TrimLeft(u, "@")
}

// Compile-time assertion that MemoryResolver satisfies IdentityResolver.
var _ IdentityResolver = (*MemoryResolver)(nil)
