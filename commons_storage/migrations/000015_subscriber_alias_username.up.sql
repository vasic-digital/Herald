-- 000015_subscriber_alias_username.up.sql — docs/design/PARTICIPANT_ATTRIBUTION.md §4d.
-- Adds the per-channel @handle used for notification @-tagging. Distinct from
-- channel_user_id (the chat/user id): username is the "@handle" resolved by
-- IdentityResolver.UsernameFor when building outbound mention lists. Nullable —
-- backfilled where known; participants with no username on a channel cannot be
-- tagged there.
ALTER TABLE subscriber_aliases ADD COLUMN IF NOT EXISTS username TEXT;

-- Lookup path: resolve a canonical handle's @username on a channel (outbound
-- tagging) and match an inbound sender by (channel, username).
CREATE INDEX IF NOT EXISTS subscriber_aliases_channel_username_idx
    ON subscriber_aliases (channel, username)
    WHERE username IS NOT NULL;

-- subscriber_aliases inherits RLS via its subscriber_id FK to subscribers
-- (which is tenant-isolated + FORCE RLS per 000003 / 000008). No new policy is
-- required: the column is additive and carries no tenant axis of its own.
