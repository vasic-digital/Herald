-- 000015_subscriber_alias_username.down.sql — reverse of the up migration.
DROP INDEX IF EXISTS subscriber_aliases_channel_username_idx;
ALTER TABLE subscriber_aliases DROP COLUMN IF EXISTS username;
