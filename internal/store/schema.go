package store

const schemaSQL = `
CREATE TABLE IF NOT EXISTS participants (
  id TEXT PRIMARY KEY,
  twitch_user_id TEXT UNIQUE NOT NULL,
  twitch_login TEXT NOT NULL,
  twitch_display_name TEXT,
  twitch_profile_image_url TEXT,
  minecraft_nick TEXT,
  avatar_url TEXT,
  best_time_ms INT,
  status TEXT NOT NULL DEFAULT 'invited',
  stream_online BOOLEAN NOT NULL DEFAULT FALSE,
  last_login_at TIMESTAMPTZ,
  last_stream_status_sync_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS runs (
  id TEXT PRIMARY KEY,
  participant_id TEXT NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
  time_ms INT NOT NULL,
  nether_split_ms INT,
  nether_exit_split_ms INT,
  end_split_ms INT,
  started_at TIMESTAMPTZ NOT NULL,
  finished_at TIMESTAMPTZ NOT NULL,
  proof_url TEXT,
  source TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'approved',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE runs
  ADD COLUMN IF NOT EXISTS tournament_id TEXT;

ALTER TABLE runs
  ADD COLUMN IF NOT EXISTS phase TEXT;

ALTER TABLE runs
  ADD COLUMN IF NOT EXISTS match_id TEXT;

ALTER TABLE runs
  ADD COLUMN IF NOT EXISTS nether_split_ms INT;

ALTER TABLE runs
  ADD COLUMN IF NOT EXISTS nether_exit_split_ms INT;

ALTER TABLE runs
  ADD COLUMN IF NOT EXISTS end_split_ms INT;

ALTER TABLE participants
  ADD COLUMN IF NOT EXISTS show_in_now_playing BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE participants
  ADD COLUMN IF NOT EXISTS now_playing_authorized BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE participants
  ADD COLUMN IF NOT EXISTS stream_title TEXT;

ALTER TABLE participants
  ADD COLUMN IF NOT EXISTS stream_game_name TEXT;

CREATE TABLE IF NOT EXISTS tournament_periods (
  id TEXT PRIMARY KEY,
  starts_at TIMESTAMPTZ NOT NULL,
  qualification_ends_at TIMESTAMPTZ,
  playoff_ends_at TIMESTAMPTZ,
  ends_at TIMESTAMPTZ NOT NULL,
  state TEXT NOT NULL,
  playoff_slots INT NOT NULL DEFAULT 8,
  bracket_generated_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE tournament_periods
  ADD COLUMN IF NOT EXISTS qualification_ends_at TIMESTAMPTZ;

ALTER TABLE tournament_periods
  ADD COLUMN IF NOT EXISTS playoff_ends_at TIMESTAMPTZ;

ALTER TABLE tournament_periods
  ADD COLUMN IF NOT EXISTS playoff_slots INT NOT NULL DEFAULT 8;

ALTER TABLE tournament_periods
  ADD COLUMN IF NOT EXISTS bracket_generated_at TIMESTAMPTZ;

UPDATE tournament_periods
SET qualification_ends_at = LEAST(starts_at + INTERVAL '14 days', ends_at - INTERVAL '4 days')
WHERE qualification_ends_at IS NULL;

UPDATE tournament_periods
SET qualification_ends_at = starts_at + INTERVAL '1 day'
WHERE qualification_ends_at <= starts_at;

UPDATE tournament_periods
SET playoff_ends_at = GREATEST(qualification_ends_at + INTERVAL '1 day', ends_at - INTERVAL '3 days')
WHERE playoff_ends_at IS NULL;

UPDATE tournament_periods
SET playoff_ends_at = ends_at - INTERVAL '1 hour'
WHERE playoff_ends_at >= ends_at;

CREATE TABLE IF NOT EXISTS tournament_matches (
  id TEXT PRIMARY KEY,
  tournament_id TEXT NOT NULL REFERENCES tournament_periods(id) ON DELETE CASCADE,
  round TEXT NOT NULL,
  position INT NOT NULL,
  player1_participant_id TEXT REFERENCES participants(id) ON DELETE SET NULL,
  player2_participant_id TEXT REFERENCES participants(id) ON DELETE SET NULL,
  player1_seed INT,
  player2_seed INT,
  starts_at TIMESTAMPTZ NOT NULL,
  ends_at TIMESTAMPTZ NOT NULL,
  status TEXT NOT NULL DEFAULT 'scheduled',
  winner_participant_id TEXT REFERENCES participants(id) ON DELETE SET NULL,
  best_of INT NOT NULL DEFAULT 1,
  world_seed TEXT,
  player1_connected_at TIMESTAMPTZ,
  player2_connected_at TIMESTAMPTZ,
  official_started_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (tournament_id, round, position)
);

ALTER TABLE tournament_matches
  ADD COLUMN IF NOT EXISTS world_seed TEXT;

ALTER TABLE tournament_matches
  ADD COLUMN IF NOT EXISTS player1_connected_at TIMESTAMPTZ;

ALTER TABLE tournament_matches
  ADD COLUMN IF NOT EXISTS player2_connected_at TIMESTAMPTZ;

ALTER TABLE tournament_matches
  ADD COLUMN IF NOT EXISTS official_started_at TIMESTAMPTZ;

UPDATE tournament_matches
SET world_seed = CASE WHEN random() < 0.5 THEN '-' ELSE '' END || floor(random() * 9000000000000000000)::bigint::text
WHERE world_seed IS NULL OR world_seed = '';

CREATE TABLE IF NOT EXISTS admins (
  id TEXT PRIMARY KEY,
  twitch_user_id TEXT UNIQUE NOT NULL,
  role TEXT NOT NULL DEFAULT 'admin',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS app_settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS tournament_applications (
  id TEXT PRIMARY KEY,
  application_number BIGINT,
  twitch_user_id TEXT UNIQUE NOT NULL,
  twitch_login TEXT NOT NULL,
  twitch_display_name TEXT,
  twitch_profile_image_url TEXT,
  twitch_channel_url TEXT NOT NULL,
  discord_username TEXT NOT NULL,
  timezone TEXT NOT NULL,
  referral TEXT,
  understands_stream_required BOOLEAN NOT NULL DEFAULT TRUE,
  status TEXT NOT NULL DEFAULT 'pending',
  participant_id TEXT REFERENCES participants(id) ON DELETE SET NULL,
  reviewed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE SEQUENCE IF NOT EXISTS discord_applications_application_number_seq;

CREATE TABLE IF NOT EXISTS discord_applications (
  id TEXT PRIMARY KEY,
  application_number BIGINT NOT NULL DEFAULT nextval('discord_applications_application_number_seq'),
  guild_id TEXT NOT NULL,
  discord_user_id TEXT NOT NULL,
  discord_username TEXT NOT NULL,
  project_key TEXT NOT NULL DEFAULT 'chaos',
  game_nick TEXT NOT NULL,
  timezone TEXT NOT NULL,
  experience TEXT NOT NULL,
  motivation TEXT NOT NULL,
  links TEXT,
  status TEXT NOT NULL DEFAULT 'pending',
  reviewer_discord_id TEXT,
  review_reason TEXT,
  log_channel_id TEXT,
  log_message_id TEXT,
  reviewed_at TIMESTAMPTZ,
  role_granted_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT discord_applications_status_check
    CHECK (status IN ('pending', 'needs_info', 'approved', 'rejected', 'cancelled'))
);

CREATE UNIQUE INDEX IF NOT EXISTS discord_applications_number_idx
  ON discord_applications (application_number);

CREATE UNIQUE INDEX IF NOT EXISTS discord_applications_active_user_idx
  ON discord_applications (guild_id, discord_user_id, project_key)
  WHERE status IN ('pending', 'needs_info', 'approved');

CREATE INDEX IF NOT EXISTS discord_applications_status_created_idx
  ON discord_applications (status, created_at DESC);

CREATE TABLE IF NOT EXISTS discord_application_audit (
  id TEXT PRIMARY KEY,
  application_id TEXT NOT NULL REFERENCES discord_applications(id) ON DELETE CASCADE,
  actor_discord_id TEXT NOT NULL,
  action TEXT NOT NULL,
  details TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS discord_application_audit_application_idx
  ON discord_application_audit (application_id, created_at);

CREATE TABLE IF NOT EXISTS mod_nonces (
  nonce TEXT PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS mod_auth_tokens (
  id TEXT PRIMARY KEY,
  participant_id TEXT NOT NULL UNIQUE REFERENCES participants(id) ON DELETE CASCADE,
  token_preview TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  last_used_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS mod_run_sessions (
  id TEXT PRIMARY KEY,
  participant_id TEXT NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
  world_fingerprint TEXT NOT NULL,
  seed_hash TEXT NOT NULL,
  tournament_id TEXT,
  phase TEXT,
  match_id TEXT,
  status TEXT NOT NULL DEFAULT 'active',
  started_at TIMESTAMPTZ NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  completed_run_id TEXT REFERENCES runs(id) ON DELETE SET NULL,
  completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS leaderboard_snapshots (
  id TEXT PRIMARY KEY,
  participant_id TEXT NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
  run_id TEXT REFERENCES runs(id) ON DELETE SET NULL,
  rank INT NOT NULL,
  best_time_ms INT NOT NULL,
  delta INT NOT NULL DEFAULT 0,
  reason TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS participant_achievements (
  participant_id TEXT NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
  achievement_slug TEXT NOT NULL,
  unlocked_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  source_run_id TEXT REFERENCES runs(id) ON DELETE SET NULL,
  PRIMARY KEY (participant_id, achievement_slug)
);

CREATE TABLE IF NOT EXISTS telegram_support_tickets (
  id TEXT PRIMARY KEY,
  ticket_number BIGINT,
  user_chat_id BIGINT NOT NULL,
  user_telegram_id BIGINT,
  user_username TEXT,
  question TEXT NOT NULL,
  answer TEXT,
  status TEXT NOT NULL DEFAULT 'open',
  answered_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS telegram_chaos_application_drafts (
  chat_id BIGINT PRIMARY KEY,
  step INT NOT NULL,
  game_nick TEXT NOT NULL DEFAULT '',
  timezone TEXT NOT NULL DEFAULT '',
  experience TEXT NOT NULL DEFAULT '',
  discord_id TEXT NOT NULL DEFAULT '',
  discord_username TEXT NOT NULL DEFAULT '',
  motivation TEXT NOT NULL DEFAULT '',
  links TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE telegram_chaos_application_drafts
  ADD COLUMN IF NOT EXISTS discord_username TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS telegram_news_posts (
  id TEXT PRIMARY KEY,
  telegram_chat_id BIGINT NOT NULL,
  telegram_message_id INT NOT NULL,
  source_url TEXT,
  text TEXT NOT NULL,
  image_url TEXT,
  published_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (telegram_chat_id, telegram_message_id)
);

CREATE TABLE IF NOT EXISTS seen_modrinth_versions (
  id TEXT PRIMARY KEY,
  project_id TEXT NOT NULL,
  version_number TEXT NOT NULL,
  published_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);


CREATE INDEX IF NOT EXISTS participants_status_best_time_idx
  ON participants (status, best_time_ms);

CREATE INDEX IF NOT EXISTS runs_participant_created_idx
  ON runs (participant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS tournament_periods_state_window_idx
  ON tournament_periods (state, starts_at DESC, ends_at DESC);

CREATE INDEX IF NOT EXISTS tournament_matches_tournament_round_idx
  ON tournament_matches (tournament_id, round, position);

CREATE INDEX IF NOT EXISTS tournament_matches_window_idx
  ON tournament_matches (starts_at, ends_at);

CREATE INDEX IF NOT EXISTS leaderboard_snapshots_participant_created_idx
  ON leaderboard_snapshots (participant_id, created_at DESC);

CREATE INDEX IF NOT EXISTS participant_achievements_unlocked_idx
  ON participant_achievements (participant_id, unlocked_at DESC);

CREATE INDEX IF NOT EXISTS mod_run_sessions_participant_status_idx
  ON mod_run_sessions (participant_id, status, expires_at DESC);

CREATE INDEX IF NOT EXISTS mod_run_sessions_world_idx
  ON mod_run_sessions (world_fingerprint, seed_hash);

CREATE SEQUENCE IF NOT EXISTS telegram_support_tickets_ticket_number_seq;

UPDATE telegram_support_tickets
SET ticket_number = nextval('telegram_support_tickets_ticket_number_seq')
WHERE ticket_number IS NULL;

SELECT setval(
  'telegram_support_tickets_ticket_number_seq',
  COALESCE((SELECT MAX(ticket_number) FROM telegram_support_tickets), 0) + 1,
  false
);

ALTER TABLE telegram_support_tickets
  ALTER COLUMN ticket_number SET DEFAULT nextval('telegram_support_tickets_ticket_number_seq');

ALTER TABLE telegram_support_tickets
  ALTER COLUMN ticket_number SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS telegram_support_tickets_number_idx
  ON telegram_support_tickets (ticket_number);

CREATE INDEX IF NOT EXISTS telegram_support_tickets_status_created_idx
  ON telegram_support_tickets (status, created_at DESC);

CREATE INDEX IF NOT EXISTS telegram_news_posts_published_idx
  ON telegram_news_posts (published_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS telegram_news_posts_source_url_idx
  ON telegram_news_posts (source_url)
  WHERE source_url IS NOT NULL;

CREATE INDEX IF NOT EXISTS tournament_applications_status_created_idx
  ON tournament_applications (status, created_at DESC);

ALTER TABLE tournament_applications
  ADD COLUMN IF NOT EXISTS application_number BIGINT;

CREATE SEQUENCE IF NOT EXISTS tournament_applications_application_number_seq;

UPDATE tournament_applications
SET application_number = nextval('tournament_applications_application_number_seq')
WHERE application_number IS NULL;

SELECT setval(
  'tournament_applications_application_number_seq',
  COALESCE((SELECT MAX(application_number) FROM tournament_applications), 0) + 1,
  false
);

ALTER TABLE tournament_applications
  ALTER COLUMN application_number SET DEFAULT nextval('tournament_applications_application_number_seq');

ALTER TABLE tournament_applications
  ALTER COLUMN application_number SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS tournament_applications_number_idx
  ON tournament_applications (application_number);

CREATE INDEX IF NOT EXISTS tournament_applications_referral_idx
  ON tournament_applications (referral);

CREATE INDEX IF NOT EXISTS mod_auth_tokens_token_hash_idx
  ON mod_auth_tokens (token_hash);

ALTER TABLE runs
  DROP CONSTRAINT IF EXISTS runs_phase_check;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'participants_status_check'
  ) THEN
    ALTER TABLE participants
    ADD CONSTRAINT participants_status_check
    CHECK (status IN ('invited', 'active', 'blocked'));
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'participants_best_time_ms_check'
  ) THEN
    ALTER TABLE participants
    ADD CONSTRAINT participants_best_time_ms_check
    CHECK (best_time_ms IS NULL OR best_time_ms > 0);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'runs_time_ms_check'
  ) THEN
    ALTER TABLE runs
    ADD CONSTRAINT runs_time_ms_check
    CHECK (time_ms > 0);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'runs_time_window_check'
  ) THEN
    ALTER TABLE runs
    ADD CONSTRAINT runs_time_window_check
    CHECK (finished_at > started_at);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'runs_source_check'
  ) THEN
    ALTER TABLE runs
    ADD CONSTRAINT runs_source_check
    CHECK (source IN ('dashboard', 'mod', 'admin'));
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'runs_status_check'
  ) THEN
    ALTER TABLE runs
    ADD CONSTRAINT runs_status_check
    CHECK (status IN ('pending', 'approved', 'rejected'));
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'runs_phase_check'
  ) THEN
    ALTER TABLE runs
    ADD CONSTRAINT runs_phase_check
    CHECK (phase IS NULL OR phase IN ('qualification', 'playoff', 'final', 'test'));
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'runs_split_order_check'
  ) THEN
    ALTER TABLE runs
    ADD CONSTRAINT runs_split_order_check
    CHECK (
      (nether_split_ms IS NULL OR (nether_split_ms > 0 AND nether_split_ms < time_ms))
      AND (nether_exit_split_ms IS NULL OR (nether_exit_split_ms > 0 AND nether_exit_split_ms < time_ms))
      AND (end_split_ms IS NULL OR (end_split_ms > 0 AND end_split_ms < time_ms))
      AND (nether_split_ms IS NULL OR nether_exit_split_ms IS NULL OR nether_exit_split_ms > nether_split_ms)
      AND (nether_exit_split_ms IS NULL OR end_split_ms IS NULL OR end_split_ms > nether_exit_split_ms)
      AND (nether_split_ms IS NULL OR end_split_ms IS NULL OR end_split_ms > nether_split_ms)
    );
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'tournament_periods_state_check'
  ) THEN
    ALTER TABLE tournament_periods
    ADD CONSTRAINT tournament_periods_state_check
    CHECK (state IN ('scheduled', 'running', 'finished'));
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'tournament_periods_window_check'
  ) THEN
    ALTER TABLE tournament_periods
    ADD CONSTRAINT tournament_periods_window_check
    CHECK (ends_at > starts_at);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'tournament_periods_phase_window_check'
  ) THEN
    ALTER TABLE tournament_periods
    ADD CONSTRAINT tournament_periods_phase_window_check
    CHECK (
      qualification_ends_at > starts_at
      AND playoff_ends_at > qualification_ends_at
      AND ends_at > playoff_ends_at
    );
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'tournament_periods_playoff_slots_check'
  ) THEN
    ALTER TABLE tournament_periods
    ADD CONSTRAINT tournament_periods_playoff_slots_check
    CHECK (playoff_slots >= 2 AND playoff_slots <= 16);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'tournament_matches_round_check'
  ) THEN
    ALTER TABLE tournament_matches
    ADD CONSTRAINT tournament_matches_round_check
    CHECK (round IN ('quarterfinal', 'semifinal', 'final', 'third_place'));
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'tournament_matches_status_check'
  ) THEN
    ALTER TABLE tournament_matches
    ADD CONSTRAINT tournament_matches_status_check
    CHECK (status IN ('scheduled', 'running', 'finished'));
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'tournament_matches_window_check'
  ) THEN
    ALTER TABLE tournament_matches
    ADD CONSTRAINT tournament_matches_window_check
    CHECK (ends_at > starts_at);
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'tournament_matches_best_of_check'
  ) THEN
    ALTER TABLE tournament_matches
    ADD CONSTRAINT tournament_matches_best_of_check
    CHECK (best_of IN (1, 3, 5));
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'admins_role_check'
  ) THEN
    ALTER TABLE admins
    ADD CONSTRAINT admins_role_check
    CHECK (role IN ('admin', 'superadmin'));
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'tournament_applications_status_check'
  ) THEN
    ALTER TABLE tournament_applications
    ADD CONSTRAINT tournament_applications_status_check
    CHECK (status IN ('pending', 'approved', 'rejected'));
  END IF;
END $$;
DELETE FROM mod_nonces
WHERE created_at < NOW() - INTERVAL '2 hours';

CREATE TABLE IF NOT EXISTS match_predictions (
  id SERIAL PRIMARY KEY,
  twitch_user_id TEXT NOT NULL,
  twitch_login TEXT NOT NULL,
  match_id TEXT NOT NULL REFERENCES tournament_matches(id) ON DELETE CASCADE,
  predicted_winner_id TEXT NOT NULL REFERENCES participants(id) ON DELETE CASCADE,
  points_awarded INT DEFAULT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (twitch_user_id, match_id)
);

CREATE INDEX IF NOT EXISTS idx_predictions_match ON match_predictions(match_id);
CREATE INDEX IF NOT EXISTS idx_predictions_user ON match_predictions(twitch_user_id);

CREATE TABLE IF NOT EXISTS pickem_submissions (
  twitch_user_id TEXT NOT NULL,
  tournament_id TEXT NOT NULL REFERENCES tournament_periods(id) ON DELETE CASCADE,
  twitch_login TEXT NOT NULL,
  submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (twitch_user_id, tournament_id)
);
`
