-- Created at: 2026-10-06T00:00:00Z
--
-- The opaque callback token NSW sends with each inject. It names the one NSW
-- workflow step the application answers, and the review outcome or feedback is
-- sent back on /api/v1/callbacks/{callback_token}. A re-inject of the same task
-- (e.g. after an amendment) replaces it. Rows from before this column have no
-- token and cannot be called back; re-inject them.

-- @UP
ALTER TABLE applications ADD COLUMN callback_token TEXT NOT NULL DEFAULT '';

-- @DOWN
ALTER TABLE applications DROP COLUMN callback_token;
