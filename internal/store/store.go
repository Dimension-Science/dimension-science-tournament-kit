package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Store struct {
	db                        *sql.DB
	leaderboardMutex          sync.RWMutex
	leaderboardCache          []LeaderboardEntry
	leaderboardCacheExpiresAt time.Time
}

const testModeUntilSettingKey = "test_mode_until"

func Open(databaseURL string) (*Store, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(10 * time.Minute)
	db.SetConnMaxIdleTime(3 * time.Minute)

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) EnsureSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, schemaSQL)
	return err
}

func (s *Store) SeedDemo(ctx context.Context) error {
	if err := s.EnsureSchema(ctx); err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO admins (id, twitch_user_id, role)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (twitch_user_id) DO NOTHING`,
		uid("adm"), "admin_1", "superadmin",
	); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO participants (
			id, twitch_user_id, twitch_login, twitch_display_name, minecraft_nick, status, best_time_ms, stream_online, last_login_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NOW())
		ON CONFLICT (twitch_user_id) DO UPDATE
		SET twitch_login = EXCLUDED.twitch_login,
		    twitch_display_name = EXCLUDED.twitch_display_name,
		    minecraft_nick = EXCLUDED.minecraft_nick,
		    status = EXCLUDED.status,
		    best_time_ms = EXCLUDED.best_time_ms,
		    stream_online = EXCLUDED.stream_online,
		    updated_at = NOW()`,
		uid("p"), "streamer_1", "microsteve", "MicroSteve", "MicroSteve", "active", 735000, true,
	); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO participants (
			id, twitch_user_id, twitch_login, twitch_display_name, minecraft_nick, status, best_time_ms, stream_online, last_login_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NOW())
		ON CONFLICT (twitch_user_id) DO UPDATE
		SET twitch_login = EXCLUDED.twitch_login,
		    twitch_display_name = EXCLUDED.twitch_display_name,
		    minecraft_nick = EXCLUDED.minecraft_nick,
		    status = EXCLUDED.status,
		    best_time_ms = EXCLUDED.best_time_ms,
		    stream_online = EXCLUDED.stream_online,
		    updated_at = NOW()`,
		uid("p"), "streamer_2", "tinyalex", "TinyAlex", "TinyAlex", "active", 820000, false,
	); err != nil {
		return err
	}

	var microSteveID string
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM participants WHERE twitch_user_id = $1`,
		"streamer_1",
	).Scan(&microSteveID); err != nil {
		return err
	}
	if err := seedDemoRun(ctx, tx, microSteveID, 735000, 265000, 421000, 603000, time.Now().Add(-6*time.Hour)); err != nil {
		return err
	}

	var tinyAlexID string
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM participants WHERE twitch_user_id = $1`,
		"streamer_2",
	).Scan(&tinyAlexID); err != nil {
		return err
	}
	if err := seedDemoRun(ctx, tx, tinyAlexID, 820000, 289000, 452000, 668000, time.Now().Add(-3*time.Hour)); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO tournament_periods (id, starts_at, qualification_ends_at, playoff_ends_at, ends_at, state, updated_at)
		 SELECT $1, NOW() - INTERVAL '1 day', NOW() + INTERVAL '7 days', NOW() + INTERVAL '11 days', NOW() + INTERVAL '13 days', 'running', NOW()
		 WHERE NOT EXISTS (
		   SELECT 1 FROM tournament_periods WHERE state = 'running'
		 )`,
		uid("tp"),
	); err != nil {
		return err
	}

	return tx.Commit()
}

func seedDemoRun(ctx context.Context, tx *sql.Tx, participantID string, timeMS, netherSplitMS, netherExitSplitMS, endSplitMS int, finishedAt time.Time) error {
	var exists bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (
		   SELECT 1 FROM runs
		   WHERE participant_id = $1
		     AND status = 'approved'
		     AND time_ms = $2
		 )`,
		participantID, timeMS,
	).Scan(&exists); err != nil {
		return err
	}
	if exists {
		_, err := tx.ExecContext(ctx,
			`UPDATE runs
			 SET nether_split_ms = COALESCE(nether_split_ms, $1),
			     nether_exit_split_ms = COALESCE(nether_exit_split_ms, $2),
			     end_split_ms = COALESCE(end_split_ms, $3)
			 WHERE participant_id = $4
			   AND status = 'approved'
			   AND time_ms = $5`,
			netherSplitMS, netherExitSplitMS, endSplitMS, participantID, timeMS,
		)
		return err
	}

	startedAt := finishedAt.Add(-time.Duration(timeMS) * time.Millisecond)
	_, err := tx.ExecContext(ctx,
		`INSERT INTO runs (
		   id, participant_id, time_ms, nether_split_ms, nether_exit_split_ms, end_split_ms, started_at, finished_at, source, status
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'mod','approved')`,
		uid("run"), participantID, timeMS, netherSplitMS, netherExitSplitMS, endSplitMS, startedAt, finishedAt,
	)
	return err
}

func (s *Store) GetCurrentTournament(ctx context.Context) (*Tournament, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, starts_at, qualification_ends_at, playoff_ends_at, ends_at, state, playoff_slots, bracket_generated_at
		 FROM tournament_periods
		 ORDER BY starts_at DESC
		 LIMIT 1`,
	)
	return scanTournament(row)
}

func (s *Store) IsTournamentRunning(ctx context.Context) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (
			SELECT 1
			FROM tournament_periods
			WHERE state = 'running'
			  AND starts_at <= NOW()
			  AND ends_at >= NOW()
		)`,
	).Scan(&exists)
	return exists, err
}

func (s *Store) GetTestModeStatus(ctx context.Context) (TestModeStatus, error) {
	var endsAt time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT value::timestamptz
		 FROM app_settings
		 WHERE key = $1`,
		testModeUntilSettingKey,
	).Scan(&endsAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TestModeStatus{}, nil
		}
		return TestModeStatus{}, err
	}

	endsAt = endsAt.UTC()
	if !endsAt.After(time.Now().UTC()) {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM app_settings WHERE key = $1`, testModeUntilSettingKey)
		return TestModeStatus{}, nil
	}

	return TestModeStatus{Enabled: true, EndsAt: &endsAt}, nil
}

func (s *Store) EnableTestMode(ctx context.Context, endsAt time.Time) (TestModeStatus, error) {
	endsAt = endsAt.UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO app_settings (key, value, updated_at)
		 VALUES ($1, $2, NOW())
		 ON CONFLICT (key) DO UPDATE
		 SET value = EXCLUDED.value,
		     updated_at = NOW()`,
		testModeUntilSettingKey,
		endsAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return TestModeStatus{}, err
	}
	return s.GetTestModeStatus(ctx)
}

func (s *Store) DisableTestMode(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM app_settings WHERE key = $1`, testModeUntilSettingKey)
	return err
}

func (s *Store) AreApplicationsClosed(ctx context.Context) (bool, error) {
	var val string
	err := s.db.QueryRowContext(ctx,
		`SELECT value
		 FROM app_settings
		 WHERE key = 'applications_closed'`,
	).Scan(&val)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return val == "true", nil
}

func (s *Store) SetApplicationsClosed(ctx context.Context, closed bool) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO app_settings (key, value, updated_at)
		 VALUES ('applications_closed', $1, NOW())
		 ON CONFLICT (key) DO UPDATE
		 SET value = EXCLUDED.value,
		     updated_at = NOW()`,
		strconv.FormatBool(closed),
	)
	return err
}

func (s *Store) GetCustomNotification(ctx context.Context) (string, string, error) {
	var title, text string
	rows, err := s.db.QueryContext(ctx,
		`SELECT key, value FROM app_settings WHERE key IN ('custom_notification_title', 'custom_notification_text')`,
	)
	if err != nil {
		return "", "", err
	}
	defer rows.Close()

	for rows.Next() {
		var key, val string
		if err := rows.Scan(&key, &val); err != nil {
			return "", "", err
		}
		if key == "custom_notification_title" {
			title = val
		} else if key == "custom_notification_text" {
			text = val
		}
	}
	return title, text, rows.Err()
}

func (s *Store) SetCustomNotification(ctx context.Context, title, text string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO app_settings (key, value, updated_at) VALUES ('custom_notification_title', $1, NOW())
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		title,
	)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO app_settings (key, value, updated_at) VALUES ('custom_notification_text', $1, NOW())
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		text,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) ClearCustomNotification(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM app_settings WHERE key IN ('custom_notification_title', 'custom_notification_text')`,
	)
	return err
}

func (s *Store) ListGlobalNotifications(ctx context.Context, limit int) ([]NotificationItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT 
			event_time,
			participant_id,
			twitch_login,
			twitch_display_name,
			avatar_url,
			event_type,
			detail_data,
			time_ms
		FROM (
			SELECT 
				pa.unlocked_at AS event_time,
				p.id AS participant_id,
				p.twitch_login AS twitch_login,
				COALESCE(NULLIF(p.twitch_display_name, ''), p.twitch_login) AS twitch_display_name,
				COALESCE(NULLIF(p.avatar_url, ''), NULLIF(p.twitch_profile_image_url, ''), '') AS avatar_url,
				'achievement' AS event_type,
				pa.achievement_slug AS detail_data,
				NULL::int AS time_ms
			FROM participant_achievements pa
			JOIN participants p ON p.id = pa.participant_id
			
			UNION ALL
			
			SELECT 
				r.finished_at AS event_time,
				p.id AS participant_id,
				p.twitch_login AS twitch_login,
				COALESCE(NULLIF(p.twitch_display_name, ''), p.twitch_login) AS twitch_display_name,
				COALESCE(NULLIF(p.avatar_url, ''), NULLIF(p.twitch_profile_image_url, ''), '') AS avatar_url,
				'run' AS event_type,
				r.id AS detail_data,
				r.time_ms AS time_ms
			FROM runs r
			JOIN participants p ON p.id = r.participant_id
			WHERE r.status = 'approved'
		) events
		ORDER BY event_time DESC
		LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []NotificationItem
	for rows.Next() {
		var item NotificationItem
		var timeMS sql.NullInt64
		if err := rows.Scan(
			&item.EventTime,
			&item.ParticipantID,
			&item.TwitchLogin,
			&item.DisplayName,
			&item.AvatarURL,
			&item.EventType,
			&item.DetailData,
			&timeMS,
		); err != nil {
			return nil, err
		}
		if timeMS.Valid {
			val := int(timeMS.Int64)
			item.TimeMS = &val
		}
		if item.EventType == "achievement" {
			if def, ok := achievementDefinitionForSlug(item.DetailData); ok {
				item.AchievementName = def.name
			} else {
				item.AchievementName = item.DetailData
			}
		}
		list = append(list, item)
	}
	return list, rows.Err()
}

func (s *Store) ListPersonalNotifications(ctx context.Context, participantID string, limit int) ([]NotificationItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT 
			event_time,
			participant_id,
			twitch_login,
			twitch_display_name,
			avatar_url,
			event_type,
			detail_data,
			time_ms,
			run_status
		FROM (
			SELECT 
				pa.unlocked_at AS event_time,
				p.id AS participant_id,
				p.twitch_login AS twitch_login,
				COALESCE(NULLIF(p.twitch_display_name, ''), p.twitch_login) AS twitch_display_name,
				COALESCE(NULLIF(p.avatar_url, ''), NULLIF(p.twitch_profile_image_url, ''), '') AS avatar_url,
				'achievement' AS event_type,
				pa.achievement_slug AS detail_data,
				NULL::int AS time_ms,
				'' AS run_status
			FROM participant_achievements pa
			JOIN participants p ON p.id = pa.participant_id
			WHERE p.id = $1
			
			UNION ALL
			
			SELECT 
				r.finished_at AS event_time,
				p.id AS participant_id,
				p.twitch_login AS twitch_login,
				COALESCE(NULLIF(p.twitch_display_name, ''), p.twitch_login) AS twitch_display_name,
				COALESCE(NULLIF(p.avatar_url, ''), NULLIF(p.twitch_profile_image_url, ''), '') AS avatar_url,
				'run' AS event_type,
				r.id AS detail_data,
				r.time_ms AS time_ms,
				r.status AS run_status
			FROM runs r
			JOIN participants p ON p.id = r.participant_id
			WHERE p.id = $1
			
			UNION ALL
			
			SELECT 
				p.created_at AS event_time,
				p.id AS participant_id,
				p.twitch_login AS twitch_login,
				COALESCE(NULLIF(p.twitch_display_name, ''), p.twitch_login) AS twitch_display_name,
				COALESCE(NULLIF(p.avatar_url, ''), NULLIF(p.twitch_profile_image_url, ''), '') AS avatar_url,
				'whitelist' AS event_type,
				'' AS detail_data,
				NULL::int AS time_ms,
				'' AS run_status
			FROM participants p
			WHERE p.id = $1
		) events
		ORDER BY event_time DESC
		LIMIT $2`,
		participantID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []NotificationItem
	for rows.Next() {
		var item NotificationItem
		var timeMS sql.NullInt64
		if err := rows.Scan(
			&item.EventTime,
			&item.ParticipantID,
			&item.TwitchLogin,
			&item.DisplayName,
			&item.AvatarURL,
			&item.EventType,
			&item.DetailData,
			&timeMS,
			&item.RunStatus,
		); err != nil {
			return nil, err
		}
		if timeMS.Valid {
			val := int(timeMS.Int64)
			item.TimeMS = &val
		}
		if item.EventType == "achievement" {
			if def, ok := achievementDefinitionForSlug(item.DetailData); ok {
				item.AchievementName = def.name
			} else {
				item.AchievementName = item.DetailData
			}
		}
		list = append(list, item)
	}
	return list, rows.Err()
}

func (s *Store) ListParticipantsForAdminRuns(ctx context.Context) ([]Participant, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, twitch_user_id, twitch_login, COALESCE(twitch_display_name, ''), 
		        COALESCE(twitch_profile_image_url, ''), COALESCE(minecraft_nick, ''), 
		        COALESCE(avatar_url, ''), best_time_ms, status, stream_online, 
		        last_stream_status_sync_at, last_login_at
		 FROM participants
		 WHERE status IN ('invited', 'active')
		 ORDER BY COALESCE(NULLIF(twitch_display_name, ''), twitch_login) ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Participant
	for rows.Next() {
		var p Participant
		var bestTime sql.NullInt64
		var twitchProfileImage sql.NullString
		var minecraftNick sql.NullString
		var avatarURL sql.NullString
		var lastSync sql.NullTime
		var lastLogin sql.NullTime

		if err := rows.Scan(
			&p.ID,
			&p.TwitchUserID,
			&p.TwitchLogin,
			&p.TwitchDisplayName,
			&twitchProfileImage,
			&minecraftNick,
			&avatarURL,
			&bestTime,
			&p.Status,
			&p.StreamOnline,
			&lastSync,
			&lastLogin,
		); err != nil {
			return nil, err
		}

		p.TwitchProfileImageURL = twitchProfileImage.String
		p.MinecraftNick = minecraftNick.String
		p.AvatarURL = avatarURL.String
		if bestTime.Valid {
			val := int(bestTime.Int64)
			p.BestTimeMS = &val
		}
		if lastSync.Valid {
			p.LastStreamStatusSyncAt = &lastSync.Time
		}
		if lastLogin.Valid {
			p.LastLoginAt = &lastLogin.Time
		}

		list = append(list, p)
	}
	return list, rows.Err()
}

func (s *Store) ListParticipantRuns(ctx context.Context, participantID string) ([]Run, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, participant_id, time_ms, nether_split_ms, nether_exit_split_ms, 
		        end_split_ms, started_at, finished_at, source, status, 
		        COALESCE(tournament_id, ''), COALESCE(phase, ''), COALESCE(match_id, ''), created_at
		 FROM runs
		 WHERE participant_id = $1
		 ORDER BY finished_at DESC`,
		participantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Run
	for rows.Next() {
		var r Run
		var netherSplit sql.NullInt64
		var netherExitSplit sql.NullInt64
		var endSplit sql.NullInt64

		if err := rows.Scan(
			&r.ID,
			&r.ParticipantID,
			&r.TimeMS,
			&netherSplit,
			&netherExitSplit,
			&endSplit,
			&r.StartedAt,
			&r.FinishedAt,
			&r.Source,
			&r.Status,
			&r.TournamentID,
			&r.Phase,
			&r.MatchID,
			&r.CreatedAt,
		); err != nil {
			return nil, err
		}

		if netherSplit.Valid {
			val := int(netherSplit.Int64)
			r.NetherSplitMS = &val
		}
		if netherExitSplit.Valid {
			val := int(netherExitSplit.Int64)
			r.NetherExitSplitMS = &val
		}
		if endSplit.Valid {
			val := int(endSplit.Int64)
			r.EndSplitMS = &val
		}

		list = append(list, r)
	}
	return list, rows.Err()
}

func (s *Store) UpdateRunStatusAndRecalculate(ctx context.Context, runID string, status string) (string, *int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback()

	// 1. Get participant ID for this run
	var participantID string
	var currentStatus string
	err = tx.QueryRowContext(ctx, `SELECT participant_id, status FROM runs WHERE id = $1`, runID).Scan(&participantID, &currentStatus)
	if err != nil {
		return "", nil, err
	}

	if currentStatus == status {
		var currentBestTime sql.NullInt64
		err = tx.QueryRowContext(ctx, `SELECT best_time_ms FROM participants WHERE id = $1`, participantID).Scan(&currentBestTime)
		if err != nil {
			return "", nil, err
		}
		var val *int
		if currentBestTime.Valid {
			v := int(currentBestTime.Int64)
			val = &v
		}
		return participantID, val, nil
	}

	// 2. Update run status
	_, err = tx.ExecContext(ctx, `UPDATE runs SET status = $1 WHERE id = $2`, status, runID)
	if err != nil {
		return "", nil, err
	}

	// 3. Recalculate participant's best_time_ms
	var bestTime sql.NullInt64
	err = tx.QueryRowContext(ctx,
		`SELECT MIN(time_ms) FROM runs WHERE participant_id = $1 AND status = 'approved'`,
		participantID,
	).Scan(&bestTime)
	if err != nil {
		return "", nil, err
	}

	var newBestTimeVal *int
	if bestTime.Valid {
		v := int(bestTime.Int64)
		newBestTimeVal = &v
		_, err = tx.ExecContext(ctx,
			`UPDATE participants SET best_time_ms = $1, updated_at = NOW() WHERE id = $2`,
			bestTime.Int64, participantID,
		)
	} else {
		_, err = tx.ExecContext(ctx,
			`UPDATE participants SET best_time_ms = NULL, updated_at = NOW() WHERE id = $2`,
			participantID,
		)
	}
	if err != nil {
		return "", nil, err
	}

	// 4. Update snapshots
	if err := recordLeaderboardSnapshots(ctx, tx, runID, "admin_override"); err != nil {
		return "", nil, err
	}

	// 5. Invalidate achievements if run was rejected
	if status == "rejected" {
		_, err = tx.ExecContext(ctx, `DELETE FROM participant_achievements WHERE source_run_id = $1`, runID)
		if err != nil {
			return "", nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return "", nil, err
	}

	s.invalidateLeaderboardCache()
	return participantID, newBestTimeVal, nil
}

func (s *Store) invalidateLeaderboardCache() {
	s.leaderboardMutex.Lock()
	s.leaderboardCache = nil
	s.leaderboardCacheExpiresAt = time.Time{}
	s.leaderboardMutex.Unlock()
}

func (s *Store) GetLeaderboard(ctx context.Context) ([]LeaderboardEntry, error) {
	s.leaderboardMutex.RLock()
	if s.leaderboardCache != nil && time.Now().Before(s.leaderboardCacheExpiresAt) {
		cacheCopy := make([]LeaderboardEntry, len(s.leaderboardCache))
		copy(cacheCopy, s.leaderboardCache)
		s.leaderboardMutex.RUnlock()
		return cacheCopy, nil
	}
	s.leaderboardMutex.RUnlock()

	s.leaderboardMutex.Lock()
	defer s.leaderboardMutex.Unlock()

	// Double check under write lock
	if s.leaderboardCache != nil && time.Now().Before(s.leaderboardCacheExpiresAt) {
		cacheCopy := make([]LeaderboardEntry, len(s.leaderboardCache))
		copy(cacheCopy, s.leaderboardCache)
		return cacheCopy, nil
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT
		   p.id,
		   p.twitch_login,
		   COALESCE(p.twitch_display_name, p.twitch_login),
		   COALESCE(p.minecraft_nick, ''),
		   COALESCE(NULLIF(p.avatar_url, ''), NULLIF(p.twitch_profile_image_url, ''), ''),
		   p.best_time_ms,
		   COALESCE(r.id, ''),
		   r.finished_at,
		   COALESCE(last_snapshot.delta, 0),
		   r.nether_split_ms,
		   r.nether_exit_split_ms,
		   r.end_split_ms
		 FROM participants p
		 LEFT JOIN LATERAL (
		   SELECT id, finished_at, nether_split_ms, nether_exit_split_ms, end_split_ms
		   FROM runs
		   WHERE participant_id = p.id
		     AND status = 'approved'
		     AND time_ms = p.best_time_ms
		   ORDER BY
		     CASE WHEN nether_split_ms IS NOT NULL OR nether_exit_split_ms IS NOT NULL OR end_split_ms IS NOT NULL THEN 0 ELSE 1 END,
		     finished_at ASC,
		     created_at ASC,
		     id ASC
		   LIMIT 1
		 ) r ON TRUE
		 LEFT JOIN LATERAL (
		   SELECT delta
		   FROM leaderboard_snapshots
		   WHERE participant_id = p.id
		   ORDER BY created_at DESC
		   LIMIT 1
		 ) last_snapshot ON TRUE
		 WHERE status IN ('invited','active')
		   AND best_time_ms IS NOT NULL
		 ORDER BY p.best_time_ms ASC, p.updated_at ASC, p.id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var leaderboard []LeaderboardEntry
	for rows.Next() {
		var entry LeaderboardEntry
		var netherSplit sql.NullInt64
		var netherExitSplit sql.NullInt64
		var endSplit sql.NullInt64
		var finishedAt sql.NullTime
		if err := rows.Scan(
			&entry.ParticipantID,
			&entry.TwitchLogin,
			&entry.TwitchDisplayName,
			&entry.MinecraftNick,
			&entry.AvatarURL,
			&entry.BestTimeMS,
			&entry.BestRunID,
			&finishedAt,
			&entry.RankDelta,
			&netherSplit,
			&netherExitSplit,
			&endSplit,
		); err != nil {
			return nil, err
		}
		if netherSplit.Valid {
			value := int(netherSplit.Int64)
			entry.NetherSplitMS = &value
		}
		if netherExitSplit.Valid {
			value := int(netherExitSplit.Int64)
			entry.NetherExitSplitMS = &value
		}
		if endSplit.Valid {
			value := int(endSplit.Int64)
			entry.EndSplitMS = &value
		}
		if finishedAt.Valid {
			value := finishedAt.Time
			entry.BestRunFinishedAt = &value
		}
		entry.Rank = len(leaderboard) + 1
		leaderboard = append(leaderboard, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	s.leaderboardCache = leaderboard
	s.leaderboardCacheExpiresAt = time.Now().Add(5 * time.Second)

	cacheCopy := make([]LeaderboardEntry, len(leaderboard))
	copy(cacheCopy, leaderboard)
	return cacheCopy, nil
}

func (s *Store) FindParticipantByTwitchUserID(ctx context.Context, twitchUserID string) (*Participant, error) {
	row := s.db.QueryRowContext(ctx, participantSelect+` WHERE twitch_user_id = $1`, twitchUserID)
	return scanParticipant(row)
}

func (s *Store) FindParticipantByID(ctx context.Context, id string) (*Participant, error) {
	row := s.db.QueryRowContext(ctx, participantSelect+` WHERE id = $1`, id)
	return scanParticipant(row)
}

func (s *Store) UpsertParticipantLoginIdentity(ctx context.Context, identity LoginIdentity, status string) (*Participant, error) {
	var returnedTwitchUserID string
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO participants (id, twitch_user_id, twitch_login, twitch_display_name, twitch_profile_image_url, status, last_login_at)
		 VALUES ($1, $2, $3, $4, $5, $6, NOW())
		 ON CONFLICT (twitch_user_id)
		 DO UPDATE SET
		   twitch_login = EXCLUDED.twitch_login,
		   twitch_display_name = EXCLUDED.twitch_display_name,
		   twitch_profile_image_url = EXCLUDED.twitch_profile_image_url,
		   last_login_at = NOW(),
		   updated_at = NOW()
		 RETURNING twitch_user_id`,
		uid("p"), identity.TwitchUserID, identity.TwitchLogin, nullableString(identity.TwitchDisplayName), nullableString(identity.TwitchProfileImageURL), status,
	).Scan(&returnedTwitchUserID)
	if err != nil {
		return nil, err
	}
	s.invalidateLeaderboardCache()
	return s.FindParticipantByTwitchUserID(ctx, returnedTwitchUserID)
}

func (s *Store) FindWhitelistedParticipant(ctx context.Context, twitchUserID, twitchLogin string) (*Participant, error) {
	row := s.db.QueryRowContext(ctx,
		participantSelect+`
		WHERE ($1 = '' OR twitch_user_id = $1)
		  AND ($2 = '' OR twitch_login = $2)
		  AND status IN ('invited','active')
		ORDER BY created_at ASC
		LIMIT 1`,
		twitchUserID, twitchLogin,
	)
	return scanParticipant(row)
}

func (s *Store) IsAdmin(ctx context.Context, twitchUserID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM admins WHERE twitch_user_id = $1)`,
		twitchUserID,
	).Scan(&exists)
	return exists, err
}

func (s *Store) HasAdmins(ctx context.Context) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM admins)`).Scan(&exists)
	return exists, err
}

func (s *Store) UpsertAdmin(ctx context.Context, twitchUserID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO admins (id, twitch_user_id, role)
		 VALUES ($1, $2, 'superadmin')
		 ON CONFLICT (twitch_user_id) DO NOTHING`,
		uid("adm"), twitchUserID,
	)
	return err
}

func (s *Store) UpdateParticipantLoginIdentity(ctx context.Context, identity LoginIdentity) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE participants
		 SET twitch_login = $1,
		     twitch_display_name = $2,
		     twitch_profile_image_url = $3,
		     last_login_at = NOW(),
		     updated_at = NOW()
		 WHERE twitch_user_id = $4`,
		identity.TwitchLogin, identity.TwitchDisplayName, identity.TwitchProfileImageURL, identity.TwitchUserID,
	)
	if err != nil {
		return err
	}
	s.invalidateLeaderboardCache()
	return nil
}

func (s *Store) UpdateParticipantLoginIdentityByID(ctx context.Context, participantID string, identity LoginIdentity) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE participants
		 SET twitch_user_id = $1,
		     twitch_login = $2,
		     twitch_display_name = $3,
		     twitch_profile_image_url = $4,
		     status = CASE
		       WHEN status = 'blocked' THEN status
		       ELSE 'active'
		     END,
		     last_login_at = NOW(),
		     updated_at = NOW()
		 WHERE id = $5`,
		identity.TwitchUserID, identity.TwitchLogin, nullableString(identity.TwitchDisplayName), nullableString(identity.TwitchProfileImageURL), participantID,
	)
	if err != nil {
		return err
	}
	s.invalidateLeaderboardCache()
	return nil
}

func (s *Store) UpdateMinecraftNick(ctx context.Context, twitchUserID, minecraftNick string) (*Participant, error) {
	var returnedTwitchUserID string
	err := s.db.QueryRowContext(ctx,
		`UPDATE participants
		 SET minecraft_nick = $1,
		     updated_at = NOW()
		 WHERE twitch_user_id = $2
		 RETURNING twitch_user_id`,
		minecraftNick, twitchUserID,
	).Scan(&returnedTwitchUserID)
	if err != nil {
		return nil, err
	}
	s.invalidateLeaderboardCache()
	return s.FindParticipantByTwitchUserID(ctx, returnedTwitchUserID)
}

func (s *Store) UpdateAvatar(ctx context.Context, participantID, avatarURL string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE participants
		 SET avatar_url = $1,
		     updated_at = NOW()
		 WHERE id = $2`,
		avatarURL, participantID,
	)
	if err != nil {
		return err
	}
	s.invalidateLeaderboardCache()
	return nil
}

func (s *Store) SetStreamOnline(ctx context.Context, twitchUserID string, online bool) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE participants
		 SET stream_online = $1,
		     stream_title = CASE WHEN $1 THEN stream_title ELSE NULL END,
		     stream_game_name = CASE WHEN $1 THEN stream_game_name ELSE NULL END,
		     last_stream_status_sync_at = NOW(),
		     updated_at = NOW()
		 WHERE twitch_user_id = $2`,
		online, twitchUserID,
	)
	return err
}

func (s *Store) SetStreamInfo(ctx context.Context, twitchUserID string, online bool, title, gameName string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE participants
		 SET stream_online = $1,
		     stream_title = NULLIF($2, ''),
		     stream_game_name = NULLIF($3, ''),
		     last_stream_status_sync_at = NOW(),
		     updated_at = NOW()
		 WHERE twitch_user_id = $4`,
		online, strings.TrimSpace(title), strings.TrimSpace(gameName), twitchUserID,
	)
	return err
}

func (s *Store) UpdateNowPlayingSettings(ctx context.Context, twitchUserID string, show bool) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE participants
		 SET show_in_now_playing = $1,
		     updated_at = NOW()
		 WHERE twitch_user_id = $2`,
		show, twitchUserID,
	)
	return err
}

func (s *Store) SetNowPlayingAuthorized(ctx context.Context, twitchUserID string, authorized bool) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE participants
		 SET now_playing_authorized = $1,
		     show_in_now_playing = CASE WHEN $1 THEN TRUE ELSE show_in_now_playing END,
		     updated_at = NOW()
		 WHERE twitch_user_id = $2`,
		authorized, twitchUserID,
	)
	return err
}

func (s *Store) ListParticipants(ctx context.Context) ([]Participant, error) {
	rows, err := s.db.QueryContext(ctx, participantSelect+` ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var participants []Participant
	for rows.Next() {
		participant, err := scanParticipant(rows)
		if err != nil {
			return nil, err
		}
		if participant != nil {
			participants = append(participants, *participant)
		}
	}

	return participants, rows.Err()
}

func (s *Store) UpsertTournamentApplication(ctx context.Context, input TournamentApplicationInput) (*TournamentApplication, error) {
	var returnedID string
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO tournament_applications (
		   id, twitch_user_id, twitch_login, twitch_display_name, twitch_profile_image_url,
		   twitch_channel_url, discord_username, timezone, referral, understands_stream_required, status
		 )
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'pending')
		 ON CONFLICT (twitch_user_id)
		 DO UPDATE SET
		   twitch_login = EXCLUDED.twitch_login,
		   twitch_display_name = EXCLUDED.twitch_display_name,
		   twitch_profile_image_url = EXCLUDED.twitch_profile_image_url,
		   twitch_channel_url = EXCLUDED.twitch_channel_url,
		   discord_username = EXCLUDED.discord_username,
		   timezone = EXCLUDED.timezone,
		   referral = EXCLUDED.referral,
		   understands_stream_required = EXCLUDED.understands_stream_required,
		   status = CASE
		     WHEN tournament_applications.status = 'approved' THEN tournament_applications.status
		     ELSE 'pending'
		   END,
		   updated_at = NOW()
		 RETURNING id`,
		uid("app"),
		strings.TrimSpace(input.TwitchUserID),
		strings.ToLower(strings.TrimSpace(input.TwitchLogin)),
		nullableString(strings.TrimSpace(input.TwitchDisplayName)),
		nullableString(strings.TrimSpace(input.TwitchProfileImageURL)),
		strings.TrimSpace(input.TwitchChannelURL),
		strings.TrimSpace(input.DiscordUsername),
		strings.TrimSpace(input.Timezone),
		nullableString(strings.ToLower(strings.TrimSpace(input.Referral))),
		input.UnderstandsStreamRequired,
	).Scan(&returnedID)
	if err != nil {
		return nil, err
	}
	return s.FindTournamentApplicationByID(ctx, returnedID)
}

func (s *Store) FindTournamentApplicationByID(ctx context.Context, id string) (*TournamentApplication, error) {
	row := s.db.QueryRowContext(ctx, tournamentApplicationSelect+` WHERE id = $1`, id)
	return scanTournamentApplication(row)
}

func (s *Store) FindTournamentApplicationByTwitchUserID(ctx context.Context, twitchUserID string) (*TournamentApplication, error) {
	row := s.db.QueryRowContext(ctx, tournamentApplicationSelect+` WHERE twitch_user_id = $1`, twitchUserID)
	return scanTournamentApplication(row)
}

func (s *Store) FindTournamentApplicationByNumber(ctx context.Context, number int64) (*TournamentApplication, error) {
	row := s.db.QueryRowContext(ctx, tournamentApplicationSelect+` WHERE application_number = $1`, number)
	return scanTournamentApplication(row)
}

func (s *Store) ListTournamentApplications(ctx context.Context) ([]TournamentApplication, error) {
	rows, err := s.db.QueryContext(ctx, tournamentApplicationSelect+`
		ORDER BY
		  CASE status WHEN 'pending' THEN 0 WHEN 'approved' THEN 1 ELSE 2 END,
		  created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var applications []TournamentApplication
	for rows.Next() {
		application, err := scanTournamentApplication(rows)
		if err != nil {
			return nil, err
		}
		if application != nil {
			applications = append(applications, *application)
		}
	}
	return applications, rows.Err()
}

func (s *Store) UpdateTournamentApplicationStatus(ctx context.Context, id, status, participantID string) (*TournamentApplication, error) {
	var returnedID string
	err := s.db.QueryRowContext(ctx,
		`UPDATE tournament_applications
		 SET status = $1,
		     participant_id = COALESCE(NULLIF($2, ''), participant_id),
		     reviewed_at = CASE WHEN $1 IN ('approved','rejected') THEN NOW() ELSE reviewed_at END,
		     updated_at = NOW()
		 WHERE id = $3
		 RETURNING id`,
		status, strings.TrimSpace(participantID), strings.TrimSpace(id),
	).Scan(&returnedID)
	if err != nil {
		return nil, err
	}
	return s.FindTournamentApplicationByID(ctx, returnedID)
}

func (s *Store) CreateTelegramSupportTicket(ctx context.Context, input TelegramSupportTicketInput) (*TelegramSupportTicket, error) {
	var returnedID string
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO telegram_support_tickets (
		   id, user_chat_id, user_telegram_id, user_username, question, status
		 )
		 VALUES ($1,$2,$3,$4,$5,'open')
		 RETURNING id`,
		uid("tgst"),
		input.UserChatID,
		input.UserTelegramID,
		nullableString(strings.TrimSpace(input.UserUsername)),
		strings.TrimSpace(input.Question),
	).Scan(&returnedID)
	if err != nil {
		return nil, err
	}
	return s.FindTelegramSupportTicketByID(ctx, returnedID)
}

func (s *Store) FindTelegramSupportTicketByID(ctx context.Context, id string) (*TelegramSupportTicket, error) {
	row := s.db.QueryRowContext(ctx, telegramSupportTicketSelect+` WHERE id = $1`, id)
	return scanTelegramSupportTicket(row)
}

func (s *Store) FindTelegramSupportTicketByNumber(ctx context.Context, number int64) (*TelegramSupportTicket, error) {
	row := s.db.QueryRowContext(ctx, telegramSupportTicketSelect+` WHERE ticket_number = $1`, number)
	return scanTelegramSupportTicket(row)
}

func (s *Store) ListOpenTelegramSupportTickets(ctx context.Context, limit int) ([]TelegramSupportTicket, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	rows, err := s.db.QueryContext(ctx, telegramSupportTicketSelect+`
		WHERE status = 'open'
		ORDER BY created_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []TelegramSupportTicket
	for rows.Next() {
		ticket, err := scanTelegramSupportTicket(rows)
		if err != nil {
			return nil, err
		}
		if ticket != nil {
			tickets = append(tickets, *ticket)
		}
	}
	return tickets, rows.Err()
}

func (s *Store) AnswerTelegramSupportTicket(ctx context.Context, number int64, answer string) (*TelegramSupportTicket, error) {
	var returnedID string
	err := s.db.QueryRowContext(ctx,
		`UPDATE telegram_support_tickets
		 SET answer = $1,
		     status = 'answered',
		     answered_at = NOW(),
		     updated_at = NOW()
		 WHERE ticket_number = $2
		 RETURNING id`,
		strings.TrimSpace(answer),
		number,
	).Scan(&returnedID)
	if err != nil {
		return nil, err
	}
	return s.FindTelegramSupportTicketByID(ctx, returnedID)
}

func (s *Store) SaveTelegramNewsPost(ctx context.Context, input TelegramNewsPostInput) (*TelegramNewsPost, error) {
	publishedAt := input.PublishedAt.UTC()
	if publishedAt.IsZero() {
		publishedAt = time.Now().UTC()
	}
	sourceURL := strings.TrimSpace(input.SourceURL)
	if sourceURL != "" {
		result, err := s.db.ExecContext(ctx,
			`UPDATE telegram_news_posts
			 SET telegram_chat_id = $1,
			     telegram_message_id = $2,
			     text = $3,
			     image_url = COALESCE($4, image_url),
			     published_at = $5,
			     updated_at = NOW()
			 WHERE source_url = $6`,
			input.TelegramChatID,
			input.TelegramMessageID,
			strings.TrimSpace(input.Text),
			nullableString(strings.TrimSpace(input.ImageURL)),
			publishedAt,
			sourceURL,
		)
		if err != nil {
			return nil, err
		}
		if affected, _ := result.RowsAffected(); affected > 0 {
			row := s.db.QueryRowContext(ctx, telegramNewsPostSelect+` WHERE source_url = $1`, sourceURL)
			return scanTelegramNewsPost(row)
		}
	}

	var returnedID string
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO telegram_news_posts (
		   id, telegram_chat_id, telegram_message_id, source_url, text, image_url, published_at
		 )
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 ON CONFLICT (telegram_chat_id, telegram_message_id)
		 DO UPDATE SET
		   source_url = EXCLUDED.source_url,
		   text = EXCLUDED.text,
		   image_url = COALESCE(EXCLUDED.image_url, telegram_news_posts.image_url),
		   published_at = EXCLUDED.published_at,
		   updated_at = NOW()
		 RETURNING id`,
		uid("tgn"),
		input.TelegramChatID,
		input.TelegramMessageID,
		nullableString(sourceURL),
		strings.TrimSpace(input.Text),
		nullableString(strings.TrimSpace(input.ImageURL)),
		publishedAt,
	).Scan(&returnedID)
	if err != nil {
		return nil, err
	}
	return s.FindTelegramNewsPostByID(ctx, returnedID)
}

func (s *Store) FindTelegramNewsPostByID(ctx context.Context, id string) (*TelegramNewsPost, error) {
	row := s.db.QueryRowContext(ctx, telegramNewsPostSelect+` WHERE id = $1`, id)
	return scanTelegramNewsPost(row)
}

func (s *Store) ListTelegramNewsPosts(ctx context.Context, limit int) ([]TelegramNewsPost, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.db.QueryContext(ctx, telegramNewsPostSelect+`
		ORDER BY published_at DESC, created_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []TelegramNewsPost
	for rows.Next() {
		post, err := scanTelegramNewsPost(rows)
		if err != nil {
			return nil, err
		}
		if post != nil {
			posts = append(posts, *post)
		}
	}
	return posts, rows.Err()
}

func (s *Store) ListNowPlayingParticipants(ctx context.Context) ([]Participant, error) {
	rows, err := s.db.QueryContext(ctx, participantSelect+`
		WHERE status IN ('invited','active')
		  AND show_in_now_playing = TRUE
		  AND now_playing_authorized = TRUE
		ORDER BY stream_online DESC, twitch_login ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var participants []Participant
	for rows.Next() {
		participant, err := scanParticipant(rows)
		if err != nil {
			return nil, err
		}
		if participant != nil {
			participants = append(participants, *participant)
		}
	}

	return participants, rows.Err()
}

func (s *Store) FindBestApprovedRunByParticipantID(ctx context.Context, participantID string) (*Run, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, participant_id, time_ms, nether_split_ms, nether_exit_split_ms, end_split_ms, started_at, finished_at, source, status, created_at
		 FROM runs
		 WHERE participant_id = $1
		   AND status = 'approved'
		 ORDER BY
		   time_ms ASC,
		   CASE WHEN nether_split_ms IS NOT NULL OR nether_exit_split_ms IS NOT NULL OR end_split_ms IS NOT NULL THEN 0 ELSE 1 END,
		   finished_at ASC,
		   created_at ASC,
		   id ASC
		 LIMIT 1`,
		participantID,
	)
	return scanRun(row)
}

func (s *Store) UpsertWhitelistParticipant(ctx context.Context, twitchUserID, twitchLogin, displayName string) (*Participant, error) {
	var returnedTwitchUserID string
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO participants (id, twitch_user_id, twitch_login, twitch_display_name, status)
		 VALUES ($1, $2, $3, $4, 'invited')
		 ON CONFLICT (twitch_user_id)
		 DO UPDATE SET
		   twitch_login = EXCLUDED.twitch_login,
		   twitch_display_name = COALESCE(EXCLUDED.twitch_display_name, participants.twitch_display_name),
		   status = CASE
		     WHEN participants.status = 'blocked' THEN participants.status
		     ELSE 'invited'
		   END,
		   updated_at = NOW()
		 RETURNING twitch_user_id`,
		uid("p"), twitchUserID, twitchLogin, nullableString(displayName),
	).Scan(&returnedTwitchUserID)
	if err != nil {
		return nil, err
	}
	s.invalidateLeaderboardCache()
	return s.FindParticipantByTwitchUserID(ctx, returnedTwitchUserID)
}

func (s *Store) UpsertWhitelistLogin(ctx context.Context, twitchLogin string) (*Participant, error) {
	twitchLogin = strings.ToLower(twitchLogin)

	var returnedTwitchUserID string
	err := s.db.QueryRowContext(ctx,
		`UPDATE participants
		 SET status = CASE
		       WHEN status = 'blocked' THEN status
		       ELSE 'invited'
		     END,
		     updated_at = NOW()
		 WHERE twitch_login = $1
		 RETURNING twitch_user_id`,
		twitchLogin,
	).Scan(&returnedTwitchUserID)
	if err == nil {
		s.invalidateLeaderboardCache()
		return s.FindParticipantByTwitchUserID(ctx, returnedTwitchUserID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	pendingID := "pending:" + twitchLogin
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO participants (id, twitch_user_id, twitch_login, twitch_display_name, status)
		 VALUES ($1, $2, $3, $4, 'invited')
		 ON CONFLICT (twitch_user_id)
		 DO UPDATE SET
		   twitch_login = EXCLUDED.twitch_login,
		   twitch_display_name = EXCLUDED.twitch_display_name,
		   status = CASE
		     WHEN participants.status = 'blocked' THEN participants.status
		     ELSE 'invited'
		   END,
		   updated_at = NOW()
		 RETURNING twitch_user_id`,
		uid("p"), pendingID, twitchLogin, twitchLogin,
	).Scan(&returnedTwitchUserID)
	if err != nil {
		return nil, err
	}
	s.invalidateLeaderboardCache()
	return s.FindParticipantByTwitchUserID(ctx, returnedTwitchUserID)
}

func (s *Store) UpdateParticipantStatus(ctx context.Context, twitchUserID, status string) (*Participant, error) {
	var returnedTwitchUserID string
	err := s.db.QueryRowContext(ctx,
		`UPDATE participants
		 SET status = $1,
		     updated_at = NOW()
		 WHERE twitch_user_id = $2
		 RETURNING twitch_user_id`,
		status, twitchUserID,
	).Scan(&returnedTwitchUserID)
	if err != nil {
		return nil, err
	}
	s.invalidateLeaderboardCache()
	return s.FindParticipantByTwitchUserID(ctx, returnedTwitchUserID)
}

func (s *Store) HasOverlappingTournament(ctx context.Context, startsAt, endsAt time.Time) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS (
			SELECT 1
			FROM tournament_periods
			WHERE state IN ('scheduled', 'running')
			  AND tstzrange(starts_at, ends_at, '[)') && tstzrange($1::timestamptz, $2::timestamptz, '[)')
		)`,
		startsAt, endsAt,
	).Scan(&exists)
	return exists, err
}

func (s *Store) CreateTournament(ctx context.Context, startsAt, qualificationEndsAt, playoffEndsAt, endsAt time.Time, state string, playoffSlots int) (*Tournament, error) {
	row := s.db.QueryRowContext(ctx,
		`INSERT INTO tournament_periods (id, starts_at, qualification_ends_at, playoff_ends_at, ends_at, state, playoff_slots, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		 RETURNING id, starts_at, qualification_ends_at, playoff_ends_at, ends_at, state, playoff_slots, bracket_generated_at`,
		uid("tp"), startsAt, qualificationEndsAt, playoffEndsAt, endsAt, state, playoffSlots,
	)
	return scanTournament(row)
}

func (s *Store) FinishRunningTournaments(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE tournament_periods
		 SET state = 'finished',
		     updated_at = NOW()
		 WHERE state = 'running'`,
	)
	return err
}

func (s *Store) ClearLeaderboard(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM leaderboard_snapshots`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM participant_achievements`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM runs`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE participants
		 SET best_time_ms = NULL,
		     updated_at = NOW()
		 WHERE best_time_ms IS NOT NULL`,
	); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	s.invalidateLeaderboardCache()
	return nil
}

func (s *Store) ClearTestRuns(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	testRuns := `SELECT id FROM runs WHERE source = 'admin' AND phase = 'test'`
	if _, err := tx.ExecContext(ctx, `DELETE FROM leaderboard_snapshots WHERE run_id IN (`+testRuns+`)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM participant_achievements WHERE source_run_id IN (`+testRuns+`)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM runs WHERE source = 'admin' AND phase = 'test'`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`WITH best_runs AS (
		   SELECT participant_id, MIN(time_ms) AS best_time_ms
		   FROM runs
		   WHERE status = 'approved'
		   GROUP BY participant_id
		 )
		 UPDATE participants participant
		    SET best_time_ms = best_runs.best_time_ms,
		        updated_at = NOW()
		   FROM best_runs
		  WHERE participant.id = best_runs.participant_id
		    AND participant.best_time_ms IS DISTINCT FROM best_runs.best_time_ms`,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE participants participant
		    SET best_time_ms = NULL,
		        updated_at = NOW()
		  WHERE participant.best_time_ms IS NOT NULL
		    AND NOT EXISTS (
		      SELECT 1
		      FROM runs run
		      WHERE run.participant_id = participant.id
		        AND run.status = 'approved'
		    )`,
	); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	s.invalidateLeaderboardCache()
	return nil
}

func (s *Store) InsertModNonce(ctx context.Context, nonce string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO mod_nonces (nonce) VALUES ($1)`,
		nonce,
	)
	return err
}

func (s *Store) CleanupExpiredModNonces(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM mod_nonces
		 WHERE created_at < NOW() - INTERVAL '2 hours'`,
	)
	return err
}

func (s *Store) GetModAuthTokenByParticipantID(ctx context.Context, participantID string) (*ModAuthToken, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, participant_id, token_preview, token_hash, last_used_at, created_at, updated_at
		 FROM mod_auth_tokens
		 WHERE participant_id = $1`,
		participantID,
	)
	return scanModAuthToken(row)
}

func (s *Store) RotateModAuthToken(ctx context.Context, participantID, rawToken string) (*ModAuthToken, error) {
	tokenHash := hashModToken(rawToken)
	tokenPreview := tokenPreview(rawToken)

	row := s.db.QueryRowContext(ctx,
		`INSERT INTO mod_auth_tokens (id, participant_id, token_preview, token_hash, updated_at)
		 VALUES ($1, $2, $3, $4, NOW())
		 ON CONFLICT (participant_id)
		 DO UPDATE SET
		   token_preview = EXCLUDED.token_preview,
		   token_hash = EXCLUDED.token_hash,
		   updated_at = NOW()
		 RETURNING id, participant_id, token_preview, token_hash, last_used_at, created_at, updated_at`,
		uid("mat"), participantID, tokenPreview, tokenHash,
	)
	return scanModAuthToken(row)
}

func (s *Store) FindParticipantByModToken(ctx context.Context, rawToken string) (*Participant, *ModAuthToken, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT p.id, p.twitch_user_id, p.twitch_login, COALESCE(p.twitch_display_name, ''), COALESCE(p.twitch_profile_image_url, ''), COALESCE(p.minecraft_nick, ''), COALESCE(p.avatar_url, ''), p.best_time_ms, p.status, p.stream_online, p.last_stream_status_sync_at, p.last_login_at,
		        mat.id, mat.participant_id, mat.token_preview, mat.token_hash, mat.last_used_at, mat.created_at, mat.updated_at
		FROM participants p
		JOIN mod_auth_tokens mat ON mat.participant_id = p.id
		WHERE mat.token_hash = $1`,
		hashModToken(rawToken),
	)

	var participant Participant
	var bestTime sql.NullInt64
	var lastSync sql.NullTime
	var lastLogin sql.NullTime
	var token ModAuthToken
	var tokenLastUsed sql.NullTime

	err := row.Scan(
		&participant.ID,
		&participant.TwitchUserID,
		&participant.TwitchLogin,
		&participant.TwitchDisplayName,
		&participant.TwitchProfileImageURL,
		&participant.MinecraftNick,
		&participant.AvatarURL,
		&bestTime,
		&participant.Status,
		&participant.StreamOnline,
		&lastSync,
		&lastLogin,
		&token.ID,
		&token.ParticipantID,
		&token.TokenPreview,
		&token.TokenHash,
		&tokenLastUsed,
		&token.CreatedAt,
		&token.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	if bestTime.Valid {
		value := int(bestTime.Int64)
		participant.BestTimeMS = &value
	}
	if lastSync.Valid {
		value := lastSync.Time
		participant.LastStreamStatusSyncAt = &value
	}
	if lastLogin.Valid {
		value := lastLogin.Time
		participant.LastLoginAt = &value
	}
	if tokenLastUsed.Valid {
		value := tokenLastUsed.Time
		token.LastUsedAt = &value
	}

	return &participant, &token, nil
}

func (s *Store) TouchModAuthToken(ctx context.Context, tokenID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE mod_auth_tokens
		 SET last_used_at = NOW(),
		     updated_at = NOW()
		 WHERE id = $1`,
		tokenID,
	)
	return err
}

func (s *Store) StoreRun(ctx context.Context, twitchUserID string, timeMS int, startedAt, finishedAt time.Time, source string, splits RunSplitInput, runContext RunContext) (*Run, error) {
	var run *Run
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		run, err = s.storeRunTx(ctx, twitchUserID, timeMS, startedAt, finishedAt, source, splits, runContext)
		if err == nil {
			return run, nil
		}
		if isBadConn(err) {
			log.Printf("[WARNING] DB connection failed during StoreRun (attempt %d/3): %v", attempt, err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		return nil, err
	}
	return nil, err
}

func isBadConn(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, driver.ErrBadConn) {
		return true
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "bad connection") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connreset") ||
		strings.Contains(errStr, "broken pipe") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "eof")
}

func (s *Store) storeRunTx(ctx context.Context, twitchUserID string, timeMS int, startedAt, finishedAt time.Time, source string, splits RunSplitInput, runContext RunContext) (*Run, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx,
		participantSelect+` WHERE twitch_user_id = $1 FOR UPDATE`,
		twitchUserID,
	)

	participant, err := scanParticipant(row)
	if err != nil {
		return nil, err
	}
	if participant == nil || participant.Status == "blocked" {
		return nil, sql.ErrNoRows
	}

	run := &Run{}
	var netherSplit sql.NullInt64
	var netherExitSplit sql.NullInt64
	var endSplit sql.NullInt64
	err = tx.QueryRowContext(ctx,
		`INSERT INTO runs (
		   id, participant_id, time_ms, nether_split_ms, nether_exit_split_ms, end_split_ms, started_at, finished_at, source, status, tournament_id, phase, match_id
		 )
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'approved',$10,$11,$12)
		 RETURNING id, participant_id, time_ms, nether_split_ms, nether_exit_split_ms, end_split_ms, started_at, finished_at, source, status, COALESCE(tournament_id, ''), COALESCE(phase, ''), COALESCE(match_id, ''), created_at`,
		uid("run"), participant.ID, timeMS, nullableInt(splits.NetherSplitMS), nil, nullableInt(splits.EndSplitMS), startedAt, finishedAt, source, nullableString(runContext.TournamentID), nullableString(runContext.Phase), nullableString(runContext.MatchID),
	).Scan(
		&run.ID,
		&run.ParticipantID,
		&run.TimeMS,
		&netherSplit,
		&netherExitSplit,
		&endSplit,
		&run.StartedAt,
		&run.FinishedAt,
		&run.Source,
		&run.Status,
		&run.TournamentID,
		&run.Phase,
		&run.MatchID,
		&run.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if netherSplit.Valid {
		value := int(netherSplit.Int64)
		run.NetherSplitMS = &value
	}
	if netherExitSplit.Valid {
		value := int(netherExitSplit.Int64)
		run.NetherExitSplitMS = &value
	}
	if endSplit.Valid {
		value := int(endSplit.Int64)
		run.EndSplitMS = &value
	}

	improvedBest := participant.BestTimeMS != nil && timeMS < *participant.BestTimeMS
	if participant.BestTimeMS == nil || improvedBest {
		if _, err := tx.ExecContext(ctx,
			`UPDATE participants
			 SET best_time_ms = $1,
			     status = 'active',
			     updated_at = NOW()
			 WHERE id = $2`,
			timeMS, participant.ID,
		); err != nil {
			return nil, err
		}

		if err := recordLeaderboardSnapshots(ctx, tx, run.ID, source); err != nil {
			return nil, err
		}
	}

	achievements, err := s.unlockRunAchievements(ctx, tx, participant, run, improvedBest)
	if err != nil {
		return nil, err
	}
	run.Achievements = achievements

	if err := syncCurrentLeaderboardAchievements(ctx, tx, run.ID); err != nil {
		return nil, err
	}

	if runContext.MatchID != "" {
		if _, err := updateMatchWinnerFromRuns(ctx, tx, runContext.MatchID); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	s.invalidateLeaderboardCache()

	return run, nil
}

func recordLeaderboardSnapshots(ctx context.Context, tx *sql.Tx, runID, reason string) error {
	rows, err := tx.QueryContext(ctx,
		`SELECT id, best_time_ms
		 FROM participants
		 WHERE status IN ('invited','active')
		   AND best_time_ms IS NOT NULL
		 ORDER BY best_time_ms ASC, updated_at ASC, id ASC`,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	type rankRow struct {
		id         string
		bestTimeMS int
	}

	var current []rankRow
	for rows.Next() {
		var row rankRow
		if err := rows.Scan(&row.id, &row.bestTimeMS); err != nil {
			return err
		}
		current = append(current, row)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	previousRows, err := tx.QueryContext(ctx,
		`SELECT DISTINCT ON (participant_id) participant_id, rank
		 FROM leaderboard_snapshots
		 ORDER BY participant_id, created_at DESC`,
	)
	if err != nil {
		return err
	}
	defer previousRows.Close()

	previousRanks := map[string]int{}
	for previousRows.Next() {
		var participantID string
		var rank int
		if err := previousRows.Scan(&participantID, &rank); err != nil {
			return err
		}
		previousRanks[participantID] = rank
	}
	if err := previousRows.Err(); err != nil {
		return err
	}
	previousRows.Close()

	for index, row := range current {
		rank := index + 1
		previousRank, exists := previousRanks[row.id]
		if !exists {
			previousRank = rank
		}
		delta := previousRank - rank
		if exists && delta == 0 {
			continue
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO leaderboard_snapshots (id, participant_id, run_id, rank, best_time_ms, delta, reason)
			 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			uid("lbs"), row.id, runID, rank, row.bestTimeMS, delta, reason,
		); err != nil {
			return err
		}
	}

	return nil
}

const participantSelect = `
SELECT id, twitch_user_id, twitch_login, COALESCE(twitch_display_name, ''), COALESCE(twitch_profile_image_url, ''), COALESCE(minecraft_nick, ''), COALESCE(avatar_url, ''), best_time_ms, status, stream_online, show_in_now_playing, now_playing_authorized, COALESCE(stream_title, ''), COALESCE(stream_game_name, ''), last_stream_status_sync_at, last_login_at
FROM participants`

const tournamentApplicationSelect = `
SELECT id, application_number, twitch_user_id, twitch_login, COALESCE(twitch_display_name, ''), COALESCE(twitch_profile_image_url, ''), twitch_channel_url, discord_username, timezone, COALESCE(referral, ''), understands_stream_required, status, COALESCE(participant_id, ''), reviewed_at, created_at, updated_at
FROM tournament_applications`

const telegramSupportTicketSelect = `
SELECT id, ticket_number, user_chat_id, COALESCE(user_telegram_id, 0), COALESCE(user_username, ''), question, COALESCE(answer, ''), status, answered_at, created_at, updated_at
FROM telegram_support_tickets`

const telegramNewsPostSelect = `
SELECT id, telegram_chat_id, telegram_message_id, COALESCE(source_url, ''), text, COALESCE(image_url, ''), published_at, created_at, updated_at
FROM telegram_news_posts`

func scanTournament(scanner interface{ Scan(dest ...any) error }) (*Tournament, error) {
	var tournament Tournament
	var bracketGeneratedAt sql.NullTime
	err := scanner.Scan(
		&tournament.ID,
		&tournament.StartsAt,
		&tournament.QualificationEndsAt,
		&tournament.PlayoffEndsAt,
		&tournament.EndsAt,
		&tournament.State,
		&tournament.PlayoffSlots,
		&bracketGeneratedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if bracketGeneratedAt.Valid {
		value := bracketGeneratedAt.Time
		tournament.BracketGeneratedAt = &value
	}
	tournament.Phase = tournamentPhase(tournament, time.Now())
	return &tournament, nil
}

func tournamentPhase(tournament Tournament, now time.Time) string {
	if now.Before(tournament.StartsAt) {
		return "scheduled"
	}
	if !now.Before(tournament.EndsAt) {
		return "finished"
	}
	if tournament.State != "running" {
		return tournament.State
	}
	if now.Before(tournament.QualificationEndsAt) {
		return "qualification"
	}
	if now.Before(tournament.PlayoffEndsAt) {
		return "playoff"
	}
	return "final"
}

func scanParticipant(scanner interface{ Scan(dest ...any) error }) (*Participant, error) {
	var participant Participant
	var bestTime sql.NullInt64
	var lastSync sql.NullTime
	var lastLogin sql.NullTime

	err := scanner.Scan(
		&participant.ID,
		&participant.TwitchUserID,
		&participant.TwitchLogin,
		&participant.TwitchDisplayName,
		&participant.TwitchProfileImageURL,
		&participant.MinecraftNick,
		&participant.AvatarURL,
		&bestTime,
		&participant.Status,
		&participant.StreamOnline,
		&participant.ShowInNowPlaying,
		&participant.NowPlayingAuthorized,
		&participant.StreamTitle,
		&participant.StreamGameName,
		&lastSync,
		&lastLogin,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if bestTime.Valid {
		value := int(bestTime.Int64)
		participant.BestTimeMS = &value
	}
	if lastSync.Valid {
		value := lastSync.Time
		participant.LastStreamStatusSyncAt = &value
	}
	if lastLogin.Valid {
		value := lastLogin.Time
		participant.LastLoginAt = &value
	}

	return &participant, nil
}

func scanTournamentApplication(scanner interface{ Scan(dest ...any) error }) (*TournamentApplication, error) {
	var application TournamentApplication
	var reviewedAt sql.NullTime
	err := scanner.Scan(
		&application.ID,
		&application.ApplicationNumber,
		&application.TwitchUserID,
		&application.TwitchLogin,
		&application.TwitchDisplayName,
		&application.TwitchProfileImageURL,
		&application.TwitchChannelURL,
		&application.DiscordUsername,
		&application.Timezone,
		&application.Referral,
		&application.UnderstandsStreamRequired,
		&application.Status,
		&application.ParticipantID,
		&reviewedAt,
		&application.CreatedAt,
		&application.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if reviewedAt.Valid {
		value := reviewedAt.Time
		application.ReviewedAt = &value
	}
	return &application, nil
}

func scanTelegramSupportTicket(scanner interface{ Scan(dest ...any) error }) (*TelegramSupportTicket, error) {
	var ticket TelegramSupportTicket
	var answeredAt sql.NullTime
	err := scanner.Scan(
		&ticket.ID,
		&ticket.TicketNumber,
		&ticket.UserChatID,
		&ticket.UserTelegramID,
		&ticket.UserUsername,
		&ticket.Question,
		&ticket.Answer,
		&ticket.Status,
		&answeredAt,
		&ticket.CreatedAt,
		&ticket.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if answeredAt.Valid {
		value := answeredAt.Time
		ticket.AnsweredAt = &value
	}
	return &ticket, nil
}

func scanTelegramNewsPost(scanner interface{ Scan(dest ...any) error }) (*TelegramNewsPost, error) {
	var post TelegramNewsPost
	err := scanner.Scan(
		&post.ID,
		&post.TelegramChatID,
		&post.TelegramMessageID,
		&post.SourceURL,
		&post.Text,
		&post.ImageURL,
		&post.PublishedAt,
		&post.CreatedAt,
		&post.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &post, nil
}

func scanRun(scanner interface{ Scan(dest ...any) error }) (*Run, error) {
	var run Run
	var netherSplit sql.NullInt64
	var netherExitSplit sql.NullInt64
	var endSplit sql.NullInt64
	err := scanner.Scan(
		&run.ID,
		&run.ParticipantID,
		&run.TimeMS,
		&netherSplit,
		&netherExitSplit,
		&endSplit,
		&run.StartedAt,
		&run.FinishedAt,
		&run.Source,
		&run.Status,
		&run.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if netherSplit.Valid {
		value := int(netherSplit.Int64)
		run.NetherSplitMS = &value
	}
	if netherExitSplit.Valid {
		value := int(netherExitSplit.Int64)
		run.NetherExitSplitMS = &value
	}
	if endSplit.Valid {
		value := int(endSplit.Int64)
		run.EndSplitMS = &value
	}
	return &run, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func scanModAuthToken(scanner interface{ Scan(dest ...any) error }) (*ModAuthToken, error) {
	var token ModAuthToken
	var lastUsed sql.NullTime

	err := scanner.Scan(
		&token.ID,
		&token.ParticipantID,
		&token.TokenPreview,
		&token.TokenHash,
		&lastUsed,
		&token.CreatedAt,
		&token.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if lastUsed.Valid {
		value := lastUsed.Time
		token.LastUsedAt = &value
	}

	return &token, nil
}

func hashModToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

func tokenPreview(rawToken string) string {
	if len(rawToken) <= 8 {
		return rawToken
	}
	return rawToken[:4] + "..." + rawToken[len(rawToken)-4:]
}

func uid(prefix string) string {
	return fmt.Sprintf("%s_%d%x", prefix, time.Now().UnixNano(), rand.Uint64())
}

func (s *Store) StartExpiredNonceCleaner(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				_ = s.CleanupExpiredModNonces(cleanupCtx)
				cancel()
			}
		}
	}()
}

func (s *Store) GetParticipantCertificateStatus(ctx context.Context, participantID string) (status string, uniqueNumber string, available bool, err error) {
	tournament, err := s.GetCurrentTournament(ctx)
	if err != nil {
		return "", "", false, err
	}
	if tournament == nil {
		return "", "", false, nil
	}

	h := fnv.New32a()
	h.Write([]byte(participantID + "_" + tournament.ID))
	certPrefix := strings.TrimSpace(os.Getenv("CERTIFICATE_PREFIX"))
	if certPrefix == "" {
		certPrefix = "CERT-2026"
	}
	uniqueNumber = fmt.Sprintf("%s-%06d", certPrefix, h.Sum32()%1000000)

	if time.Now().Before(tournament.QualificationEndsAt) {
		return "", "", false, nil
	}

	var inPlayoff bool
	err = s.db.QueryRowContext(ctx,
		`SELECT EXISTS(
			SELECT 1
			FROM tournament_matches
			WHERE tournament_id = $1
			  AND (player1_participant_id = $2 OR player2_participant_id = $2)
		)`,
		tournament.ID, participantID,
	).Scan(&inPlayoff)
	if err != nil {
		return "", "", false, err
	}

	if !inPlayoff {
		return "КВАЛИФИКАНТ", uniqueNumber, true, nil
	}

	var finalStatus sql.NullString
	var finalWinnerID sql.NullString
	err = s.db.QueryRowContext(ctx,
		`SELECT status, winner_participant_id
		 FROM tournament_matches
		 WHERE tournament_id = $1
		   AND round = 'final'
		   AND (player1_participant_id = $2 OR player2_participant_id = $2)`,
		tournament.ID, participantID,
	).Scan(&finalStatus, &finalWinnerID)

	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", "", false, err
	}

	if err == nil && finalStatus.Valid {
		if finalStatus.String == "finished" {
			if finalWinnerID.Valid && finalWinnerID.String == participantID {
				return "ПОБЕДИТЕЛЬ", uniqueNumber, true, nil
			}
			return "ФИНАЛИСТ", uniqueNumber, true, nil
		}
		return "", "", false, nil
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT round, status, COALESCE(winner_participant_id, '')
		 FROM tournament_matches
		 WHERE tournament_id = $1
		   AND (player1_participant_id = $2 OR player2_participant_id = $2)
		 ORDER BY CASE round
		   WHEN 'quarterfinal' THEN 1
		   WHEN 'semifinal' THEN 2
		   WHEN 'third_place' THEN 3
		   WHEN 'final' THEN 4
		 END DESC`,
		tournament.ID, participantID,
	)
	if err != nil {
		return "", "", false, err
	}
	defer rows.Close()

	type matchInfo struct {
		round    string
		status   string
		winnerID string
	}

	var matches []matchInfo
	for rows.Next() {
		var m matchInfo
		if err := rows.Scan(&m.round, &m.status, &m.winnerID); err != nil {
			return "", "", false, err
		}
		matches = append(matches, m)
	}

	if len(matches) == 0 {
		return "", "", false, nil
	}

	latestMatch := matches[0]
	if latestMatch.status == "finished" {
		if latestMatch.winnerID != participantID {
			return "ИГРОК ПЛЕЙ-ОФФ", uniqueNumber, true, nil
		}
		return "", "", false, nil
	}

	return "", "", false, nil
}

func (s *Store) FindParticipantByCertificateNumber(ctx context.Context, number string) (*Participant, string, bool, error) {
	participants, err := s.ListParticipants(ctx)
	if err != nil {
		return nil, "", false, err
	}

	for _, p := range participants {
		certStatus, certNo, available, err := s.GetParticipantCertificateStatus(ctx, p.ID)
		if err == nil && available && strings.EqualFold(certNo, number) {
			return &p, certStatus, true, nil
		}
	}

	return nil, "", false, nil
}

func (s *Store) HasSeenModrinthVersion(ctx context.Context, versionID string) (bool, error) {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM seen_modrinth_versions WHERE id = $1)`, versionID).Scan(&exists)
	return exists, err
}

func (s *Store) SaveSeenModrinthVersion(ctx context.Context, versionID, projectID, versionNumber string, publishedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO seen_modrinth_versions (id, project_id, version_number, published_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO NOTHING
	`, versionID, projectID, versionNumber, publishedAt)
	return err
}
