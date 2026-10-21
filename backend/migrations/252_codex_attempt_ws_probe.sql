ALTER TABLE codex_ticket_attempts
    ADD COLUMN IF NOT EXISTS safety_buffering_enabled BOOLEAN,
    ADD COLUMN IF NOT EXISTS safety_buffering_faster_model TEXT,
    ADD COLUMN IF NOT EXISTS turn_state_len INTEGER,
    ADD COLUMN IF NOT EXISTS transport TEXT;
