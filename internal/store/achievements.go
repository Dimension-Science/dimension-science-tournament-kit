package store

import (
	"context"
	"database/sql"
	"time"
)

const rockBottomMinLeaderboardSize = 5

type achievementDefinition struct {
	slug        string
	name        string
	description string
	goal        int
}

var achievementDefinitions = []achievementDefinition{
	{slug: "legend_return", name: "Квалификант", description: "Сделать засчитанный забег во время квалификации.", goal: 1},
	{slug: "first_dragon", name: "Первопроходец", description: "Впервые убить дракона.", goal: 1},
	{slug: "diligent", name: "Усердный", description: "Убить дракона 5 раз.", goal: 5},
	{slug: "hardened", name: "Закаленный", description: "Убить дракона 25 раз.", goal: 25},
	{slug: "machine", name: "Машина", description: "Убить дракона 50 раз.", goal: 50},
	{slug: "top_three_days", name: "Три дня на вершине", description: "Пробыть на 1 месте 3 дня.", goal: 3},
	{slug: "rock_bottom", name: "Rock bottom", description: "Попасть на самую нижнюю строчку лидерборда хотя бы раз.", goal: 1},
	{slug: "seconds_hunter", name: "Охотник за секундами", description: "Улучшить личный рекорд.", goal: 1},
	{slug: "marathoner", name: "Марафонец", description: "Сделать 10 засчитанных забегов за текущий турнир.", goal: 10},
	{slug: "stability", name: "Сама стабильность", description: "Спидранить 5 дней подряд.", goal: 5},
	{slug: "on_flow", name: "На потоке", description: "Сделать 3 засчитанных забега за день.", goal: 3},
}

func achievementDefinitionForSlug(slug string) (achievementDefinition, bool) {
	for _, definition := range achievementDefinitions {
		if definition.slug == slug {
			return definition, true
		}
	}
	return achievementDefinition{}, false
}

func (s *Store) ListRecentAchievementFeed(ctx context.Context, since *time.Time, limit int) ([]AchievementFeedItem, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var sinceArg any
	if since != nil {
		sinceArg = *since
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT
		   pa.participant_id,
		   p.twitch_login,
		   COALESCE(NULLIF(p.twitch_display_name, ''), p.twitch_login),
		   COALESCE(NULLIF(p.avatar_url, ''), NULLIF(p.twitch_profile_image_url, ''), ''),
		   pa.achievement_slug,
		   pa.unlocked_at
		 FROM participant_achievements pa
		 JOIN participants p ON p.id = pa.participant_id
		 WHERE ($1::timestamptz IS NULL OR pa.unlocked_at > $1)
		 ORDER BY pa.unlocked_at DESC
		 LIMIT $2`,
		sinceArg, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]AchievementFeedItem, 0, limit)
	for rows.Next() {
		var item AchievementFeedItem
		if err := rows.Scan(
			&item.ParticipantID,
			&item.TwitchLogin,
			&item.DisplayName,
			&item.AvatarURL,
			&item.AchievementSlug,
			&item.UnlockedAt,
		); err != nil {
			return nil, err
		}
		item.ID = item.ParticipantID + ":" + item.AchievementSlug + ":" + item.UnlockedAt.UTC().Format(time.RFC3339Nano)
		if definition, ok := achievementDefinitionForSlug(item.AchievementSlug); ok {
			item.AchievementName = definition.name
			item.AchievementDescription = definition.description
		} else {
			item.AchievementName = item.AchievementSlug
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ResetParticipantAchievements(ctx context.Context, participantID string) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM participant_achievements
		 WHERE participant_id = $1`,
		participantID,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) ListAchievementProgress(ctx context.Context, participantID string) ([]AchievementProgress, error) {
	if err := s.syncParticipantLeaderboardAchievements(ctx, participantID); err != nil {
		return nil, err
	}
	unlocked, err := s.unlockedAchievements(ctx, s.db, participantID)
	if err != nil {
		return nil, err
	}
	stats, err := s.achievementStats(ctx, s.db, participantID)
	if err != nil {
		return nil, err
	}
	return buildAchievementProgress(unlocked, stats), nil
}

func (s *Store) unlockRunAchievements(ctx context.Context, tx *sql.Tx, participant *Participant, run *Run, improvedBest bool) ([]AchievementProgress, error) {
	unlocked, err := s.unlockedAchievements(ctx, tx, participant.ID)
	if err != nil {
		return nil, err
	}
	stats, err := s.achievementStats(ctx, tx, participant.ID)
	if err != nil {
		return nil, err
	}

	earned := map[string]bool{}
	justUnlocked := map[string]bool{}
	if stats.qualificationRuns >= 1 {
		earned["legend_return"] = true
	}
	if stats.totalRuns >= 1 {
		earned["first_dragon"] = true
	}
	if stats.totalRuns >= 5 {
		earned["diligent"] = true
	}
	if stats.totalRuns >= 25 {
		earned["hardened"] = true
	}
	if stats.totalRuns >= 50 {
		earned["machine"] = true
	}
	if stats.topRankDays >= 3 {
		earned["top_three_days"] = true
	}
	if rockBottomAchieved(stats) {
		earned["rock_bottom"] = true
	}
	if improvedBest {
		earned["seconds_hunter"] = true
	}
	if stats.currentTournamentRuns >= 10 {
		earned["marathoner"] = true
	}
	if stats.currentDayStreak >= 5 {
		earned["stability"] = true
	}
	if stats.runsToday >= 3 {
		earned["on_flow"] = true
	}

	for _, definition := range achievementDefinitions {
		if !earned[definition.slug] {
			continue
		}
		if _, exists := unlocked[definition.slug]; exists {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO participant_achievements (participant_id, achievement_slug, source_run_id)
			 VALUES ($1, $2, $3)
			 ON CONFLICT (participant_id, achievement_slug) DO NOTHING`,
			participant.ID, definition.slug, run.ID,
		); err != nil {
			return nil, err
		}
		now := time.Now()
		unlocked[definition.slug] = &now
		justUnlocked[definition.slug] = true
	}

	return markJustUnlocked(buildAchievementProgress(unlocked, stats), justUnlocked), nil
}

type achievementStats struct {
	totalRuns             int
	runsToday             int
	runsLast7Days         int
	currentDayStreak      int
	maxInactiveDays       int
	currentTournamentRuns int
	qualificationRuns     int
	currentRank           int
	leaderboardSize       int
	topRankDays           int
	everBottom            bool
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Store) achievementStats(ctx context.Context, q queryer, participantID string) (achievementStats, error) {
	var stats achievementStats
	if err := q.QueryRowContext(ctx,
		`SELECT
		   COUNT(*)::int,
		   COUNT(*) FILTER (WHERE finished_at::date = CURRENT_DATE)::int,
		   COUNT(*) FILTER (WHERE finished_at >= NOW() - INTERVAL '7 days')::int
		 FROM runs
		 WHERE participant_id = $1
		   AND status = 'approved'`,
		participantID,
	).Scan(&stats.totalRuns, &stats.runsToday, &stats.runsLast7Days); err != nil {
		return stats, err
	}

	if err := q.QueryRowContext(ctx,
		`WITH current_tournament AS (
		   SELECT id
		   FROM tournament_periods
		   ORDER BY starts_at DESC
		   LIMIT 1
		 )
		 SELECT
		   COUNT(*) FILTER (WHERE tournament_id = (SELECT id FROM current_tournament))::int,
		   COUNT(*) FILTER (WHERE tournament_id = (SELECT id FROM current_tournament) AND phase = 'qualification')::int
		 FROM runs
		 WHERE participant_id = $1
		   AND status = 'approved'`,
		participantID,
	).Scan(&stats.currentTournamentRuns, &stats.qualificationRuns); err != nil {
		return stats, err
	}

	rows, err := q.QueryContext(ctx,
		`SELECT DISTINCT finished_at::date
		 FROM runs
		 WHERE participant_id = $1
		   AND status = 'approved'
		 ORDER BY finished_at::date DESC`,
		participantID,
	)
	if err != nil {
		return stats, err
	}
	defer rows.Close()

	expected := time.Now().Truncate(24 * time.Hour)
	started := false
	for rows.Next() {
		var day time.Time
		if err := rows.Scan(&day); err != nil {
			return stats, err
		}
		day = day.Truncate(24 * time.Hour)
		if !started {
			if !sameDay(day, expected) {
				break
			}
			expected = day
			started = true
		}
		if !sameDay(day, expected) {
			break
		}
		stats.currentDayStreak++
		expected = expected.AddDate(0, 0, -1)
	}
	if err := rows.Err(); err != nil {
		return stats, err
	}
	rows.Close()

	if err := q.QueryRowContext(ctx,
		`WITH run_gaps AS (
		   SELECT finished_at - LAG(finished_at) OVER (ORDER BY finished_at) AS gap
		   FROM runs
		   WHERE participant_id = $1
		     AND status = 'approved'
		 ),
		 current_gap AS (
		   SELECT NOW() - MAX(finished_at) AS gap
		   FROM runs
		   WHERE participant_id = $1
		     AND status = 'approved'
		 )
		 SELECT COALESCE(MAX(EXTRACT(EPOCH FROM gap) / 86400)::int, 0)
		 FROM (
		   SELECT gap FROM run_gaps
		   UNION ALL
		   SELECT gap FROM current_gap
		 ) gaps`,
		participantID,
	).Scan(&stats.maxInactiveDays); err != nil {
		return stats, err
	}

	if err := q.QueryRowContext(ctx,
		`WITH ranked AS (
		   SELECT id,
		          ROW_NUMBER() OVER (ORDER BY best_time_ms ASC, updated_at ASC, id ASC)::int AS current_rank,
		          COUNT(*) OVER ()::int AS leaderboard_size
		   FROM participants
		   WHERE status IN ('invited','active')
		     AND best_time_ms IS NOT NULL
		 )
		 SELECT COALESCE(MAX(current_rank) FILTER (WHERE id = $1), 0)::int,
		        COALESCE(MAX(leaderboard_size), 0)::int
		 FROM ranked`,
		participantID,
	).Scan(&stats.currentRank, &stats.leaderboardSize); err != nil {
		return stats, err
	}

	if err := q.QueryRowContext(ctx,
		`WITH first_snapshots AS (
		   SELECT participant_id, MIN(created_at) AS first_seen_at
		   FROM leaderboard_snapshots
		   GROUP BY participant_id
		 ),
		 participant_snapshots AS (
		   SELECT snapshot.rank,
		          (
		            SELECT COUNT(*)::int
		            FROM first_snapshots first_snapshot
		            WHERE first_snapshot.first_seen_at <= snapshot.created_at
		          ) AS leaderboard_size_at_snapshot
		   FROM leaderboard_snapshots snapshot
		   WHERE snapshot.participant_id = $1
		 )
		 SELECT EXISTS (
		   SELECT 1
		   FROM participant_snapshots
		   WHERE rank > 1
		     AND leaderboard_size_at_snapshot >= $2
		     AND rank = leaderboard_size_at_snapshot
		 )`,
		participantID, rockBottomMinLeaderboardSize,
	).Scan(&stats.everBottom); err != nil {
		return stats, err
	}

	var latestRank int
	var latestRankAt time.Time
	if err := q.QueryRowContext(ctx,
		`SELECT rank, created_at
		 FROM leaderboard_snapshots
		 WHERE participant_id = $1
		 ORDER BY created_at DESC
		 LIMIT 1`,
		participantID,
	).Scan(&latestRank, &latestRankAt); err != nil {
		if err != sql.ErrNoRows {
			return stats, err
		}
	} else if latestRank == 1 {
		stats.topRankDays = int(time.Since(latestRankAt) / (24 * time.Hour))
		if stats.topRankDays < 0 {
			stats.topRankDays = 0
		}
	}

	return stats, nil
}

func (s *Store) syncParticipantLeaderboardAchievements(ctx context.Context, participantID string) error {
	unlocked, err := s.unlockedAchievements(ctx, s.db, participantID)
	if err != nil {
		return err
	}
	stats, err := s.achievementStats(ctx, s.db, participantID)
	if err != nil {
		return err
	}
	earned := leaderboardAchievementSlugs(stats)
	for slug := range earned {
		if _, exists := unlocked[slug]; exists {
			continue
		}
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO participant_achievements (participant_id, achievement_slug)
			 VALUES ($1, $2)
			 ON CONFLICT (participant_id, achievement_slug) DO NOTHING`,
			participantID, slug,
		); err != nil {
			return err
		}
	}
	return nil
}

func syncCurrentLeaderboardAchievements(ctx context.Context, tx *sql.Tx, sourceRunID string) error {
	rows, err := tx.QueryContext(ctx,
		`WITH ranked AS (
		   SELECT id,
		          ROW_NUMBER() OVER (ORDER BY best_time_ms ASC, updated_at ASC, id ASC)::int AS current_rank,
		          COUNT(*) OVER ()::int AS leaderboard_size
		   FROM participants
		   WHERE status IN ('invited','active')
		     AND best_time_ms IS NOT NULL
		 )
		 SELECT id, current_rank, leaderboard_size
		 FROM ranked`,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	type leaderboardParticipant struct {
		id              string
		currentRank     int
		leaderboardSize int
	}
	var participants []leaderboardParticipant
	for rows.Next() {
		var participant leaderboardParticipant
		if err := rows.Scan(&participant.id, &participant.currentRank, &participant.leaderboardSize); err != nil {
			return err
		}
		participants = append(participants, participant)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, participant := range participants {
		stats := achievementStats{
			currentRank:     participant.currentRank,
			leaderboardSize: participant.leaderboardSize,
		}
		var latestRank int
		var latestRankAt time.Time
		if err := tx.QueryRowContext(ctx,
			`SELECT rank, created_at
			 FROM leaderboard_snapshots
			 WHERE participant_id = $1
			 ORDER BY created_at DESC
			 LIMIT 1`,
			participant.id,
		).Scan(&latestRank, &latestRankAt); err != nil {
			if err != sql.ErrNoRows {
				return err
			}
		} else if latestRank == 1 {
			stats.topRankDays = int(time.Since(latestRankAt) / (24 * time.Hour))
			if stats.topRankDays < 0 {
				stats.topRankDays = 0
			}
		}

		for slug := range leaderboardAchievementSlugs(stats) {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO participant_achievements (participant_id, achievement_slug, source_run_id)
				 VALUES ($1, $2, $3)
				 ON CONFLICT (participant_id, achievement_slug) DO NOTHING`,
				participant.id, slug, sourceRunID,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func leaderboardAchievementSlugs(stats achievementStats) map[string]bool {
	earned := map[string]bool{}
	if stats.topRankDays >= 3 {
		earned["top_three_days"] = true
	}
	if rockBottomAchieved(stats) {
		earned["rock_bottom"] = true
	}
	return earned
}

func rockBottomAchieved(stats achievementStats) bool {
	return stats.leaderboardSize >= rockBottomMinLeaderboardSize &&
		(stats.everBottom || (stats.currentRank > 0 && stats.currentRank == stats.leaderboardSize))
}

func (s *Store) unlockedAchievements(ctx context.Context, q queryer, participantID string) (map[string]*time.Time, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT achievement_slug, unlocked_at
		 FROM participant_achievements
		 WHERE participant_id = $1`,
		participantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	unlocked := map[string]*time.Time{}
	for rows.Next() {
		var slug string
		var unlockedAt time.Time
		if err := rows.Scan(&slug, &unlockedAt); err != nil {
			return nil, err
		}
		value := unlockedAt
		unlocked[slug] = &value
	}
	return unlocked, rows.Err()
}

func buildAchievementProgress(unlocked map[string]*time.Time, stats achievementStats) []AchievementProgress {
	items := make([]AchievementProgress, 0, len(achievementDefinitions))
	for _, definition := range achievementDefinitions {
		progress := AchievementProgress{
			Slug:        definition.slug,
			Name:        definition.name,
			Description: definition.description,
			Goal:        definition.goal,
		}
		switch definition.slug {
		case "legend_return":
			progress.Progress = minInt(stats.qualificationRuns, definition.goal)
		case "first_dragon", "diligent", "hardened", "machine":
			progress.Progress = minInt(stats.totalRuns, definition.goal)
		case "top_three_days":
			progress.Progress = minInt(stats.topRankDays, definition.goal)
		case "rock_bottom":
			if rockBottomAchieved(stats) {
				progress.Progress = 1
			}
		case "seconds_hunter":
			if _, ok := unlocked[definition.slug]; ok {
				progress.Progress = 1
			}
		case "marathoner":
			progress.Progress = minInt(stats.currentTournamentRuns, definition.goal)
		case "stability":
			progress.Progress = minInt(stats.currentDayStreak, definition.goal)
		case "on_flow":
			progress.Progress = minInt(stats.runsToday, definition.goal)
		}
		if unlockedAt, ok := unlocked[definition.slug]; ok {
			progress.Unlocked = true
			progress.UnlockedAt = unlockedAt
			progress.Progress = definition.goal
		}
		items = append(items, progress)
	}
	return items
}

func markJustUnlocked(items []AchievementProgress, justUnlocked map[string]bool) []AchievementProgress {
	for i := range items {
		if justUnlocked[items[i].Slug] {
			items[i].JustUnlocked = true
		}
	}
	return items
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
