// Notification-tagging matrix per docs/design/PARTICIPANT_ATTRIBUTION.md §3.
package commons

// MentionsFor returns the canonical handles to @-mention for a workable-item
// event dispatched to `channel`, implementing the §3 matrix exactly:
//
//	mentions = {}
//	if assigned_to is a human handle AND assigned_to != Operator:   mentions += assigned_to
//	if created_by  is a human handle AND created_by  != Operator AND created_by != "Claude":
//	                                                                mentions += created_by
//	# "Claude" is NEVER tagged (it is the system).
//	# Operator is NEVER tagged (no self-ping).
//	# de-dup; resolve UsernameFor(handle, channel) — skip if not on that channel.
//
// The returned slice contains the canonical handles (NOT the resolved
// @usernames) that have a valid alias on `channel`, in a stable order
// (assigned_to before created_by), de-duplicated. A handle with no alias on
// `channel` is skipped — you cannot tag someone who is not on that messenger.
//
// operatorHandle is passed explicitly (rather than read from r) so callers that
// already know the operator for a non-primary channel can override; pass
// r.OperatorHandle() for the default.
func MentionsFor(createdBy, assignedTo, operatorHandle, channel string, r IdentityResolver) []string {
	mentions := make([]string, 0, 2)
	seen := make(map[string]struct{}, 2)

	add := func(handle string) {
		// SystemAgentHandle ("Claude") is never a human handle — never tagged.
		if handle == "" || handle == SystemAgentHandle {
			return
		}
		// Operator is never tagged (no self-ping).
		if handle == operatorHandle {
			return
		}
		if _, dup := seen[handle]; dup {
			return
		}
		// Must have an alias on this channel to be taggable here.
		if r == nil {
			return
		}
		if _, ok := r.UsernameFor(handle, channel); !ok {
			return
		}
		seen[handle] = struct{}{}
		mentions = append(mentions, handle)
	}

	// Order per §3: assigned_to first, then created_by.
	add(assignedTo)
	add(createdBy)

	return mentions
}
