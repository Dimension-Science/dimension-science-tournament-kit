package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"time"
)

func (s *Store) UpdateTournamentSchedule(ctx context.Context, id string, startsAt, qualificationEndsAt, playoffEndsAt, endsAt time.Time, state string, playoffSlots int) (*Tournament, error) {
	row := s.db.QueryRowContext(ctx,
		`UPDATE tournament_periods
		 SET starts_at = $2,
		     qualification_ends_at = $3,
		     playoff_ends_at = $4,
		     ends_at = $5,
		     state = $6,
		     playoff_slots = $7,
		     updated_at = NOW()
		 WHERE id = $1
		 RETURNING id, starts_at, qualification_ends_at, playoff_ends_at, ends_at, state, playoff_slots, bracket_generated_at`,
		id, startsAt, qualificationEndsAt, playoffEndsAt, endsAt, state, playoffSlots,
	)
	tournament, err := scanTournament(row)
	if err != nil {
		return nil, err
	}
	return tournament, nil
}

func (s *Store) UpdateTournamentMatchSchedule(ctx context.Context, tournamentID string, updates []TournamentMatchScheduleUpdate) ([]TournamentMatch, error) {
	if len(updates) == 0 {
		return s.ListTournamentMatches(ctx, tournamentID)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	tournament, err := scanTournament(tx.QueryRowContext(ctx,
		`SELECT id, starts_at, qualification_ends_at, playoff_ends_at, ends_at, state, playoff_slots, bracket_generated_at
		 FROM tournament_periods
		 WHERE id = $1
		 FOR UPDATE`,
		tournamentID,
	))
	if err != nil {
		return nil, err
	}
	if tournament == nil {
		return nil, sql.ErrNoRows
	}

	for _, update := range updates {
		result, err := tx.ExecContext(ctx,
			`UPDATE tournament_matches
			 SET starts_at = $3,
			     ends_at = $4,
			     world_seed = COALESCE(NULLIF($5, ''), world_seed),
			     player1_connected_at = NULL,
			     player2_connected_at = NULL,
			     official_started_at = NULL,
			     status = CASE WHEN status = 'finished' THEN status ELSE 'scheduled' END,
			     updated_at = NOW()
			 WHERE tournament_id = $1
			   AND id = $2`,
			tournamentID, update.ID, update.StartsAt, update.EndsAt, update.WorldSeed,
		)
		if err != nil {
			return nil, err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if affected == 0 {
			return nil, sql.ErrNoRows
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ListTournamentMatches(ctx, tournamentID)
}

func (s *Store) GeneratePlayoffBracket(ctx context.Context, tournamentID string) ([]TournamentMatch, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	tournament, err := scanTournament(tx.QueryRowContext(ctx,
		`SELECT id, starts_at, qualification_ends_at, playoff_ends_at, ends_at, state, playoff_slots, bracket_generated_at
		 FROM tournament_periods
		 WHERE id = $1
		 FOR UPDATE`,
		tournamentID,
	))
	if err != nil {
		return nil, err
	}
	if tournament == nil {
		return nil, sql.ErrNoRows
	}
	if time.Now().Before(tournament.QualificationEndsAt) {
		return nil, errors.New("qualification is still running")
	}

	var existing int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tournament_matches WHERE tournament_id = $1`,
		tournament.ID,
	).Scan(&existing); err != nil {
		return nil, err
	}
	if existing > 0 {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return s.ListTournamentMatches(ctx, tournamentID)
	}

	participants, err := topParticipantsForBracket(ctx, tx, tournament.PlayoffSlots)
	if err != nil {
		return nil, err
	}
	if len(participants) < 2 {
		return nil, errors.New("not enough leaderboard players for bracket")
	}
	participants = participants[:bracketSize(len(participants))]

	seedByPosition := map[int]bracketParticipant{}
	for _, participant := range participants {
		seedByPosition[participant.seed] = participant
	}

	round := openingRound(len(participants))
	pairs := seedPairs(len(participants))
	for index, pair := range pairs {
		player1 := seedByPosition[pair[0]]
		player2 := seedByPosition[pair[1]]
		startsAt, endsAt := defaultMatchWindow(tournament, round, index+1, round)
		if err := insertTournamentMatch(ctx, tx, tournament.ID, round, index+1, player1.id, player2.id, &player1.seed, &player2.seed, startsAt, endsAt, bestOfForRound(round)); err != nil {
			return nil, err
		}
	}
	if round == "quarterfinal" {
		for position := 1; position <= 2; position++ {
			startsAt, endsAt := defaultMatchWindow(tournament, "semifinal", position, round)
			if err := insertTournamentMatch(ctx, tx, tournament.ID, "semifinal", position, "", "", nil, nil, startsAt, endsAt, 1); err != nil {
				return nil, err
			}
		}
		startsAt, endsAt := defaultMatchWindow(tournament, "final", 1, round)
		if err := insertTournamentMatch(ctx, tx, tournament.ID, "final", 1, "", "", nil, nil, startsAt, endsAt, bestOfForRound("final")); err != nil {
			return nil, err
		}
	}
	if round == "semifinal" {
		startsAt, endsAt := defaultMatchWindow(tournament, "final", 1, round)
		if err := insertTournamentMatch(ctx, tx, tournament.ID, "final", 1, "", "", nil, nil, startsAt, endsAt, bestOfForRound("final")); err != nil {
			return nil, err
		}
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE tournament_periods
		 SET bracket_generated_at = NOW(), updated_at = NOW()
		 WHERE id = $1`,
		tournament.ID,
	); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.ListTournamentMatches(ctx, tournamentID)
}

func defaultMatchWindow(tournament *Tournament, round string, position int, openingRound string) (time.Time, time.Time) {
	const day = 24 * time.Hour

	offsetDays := 0
	switch round {
	case "quarterfinal":
		offsetDays = (position - 1) / 2
	case "semifinal":
		if openingRound == "quarterfinal" {
			offsetDays = 2 + position
		} else {
			offsetDays = position - 1
		}
	case "final":
		startsAt := tournament.PlayoffEndsAt
		if openingRound == "final" {
			startsAt = tournament.QualificationEndsAt
		}
		return clampMatchWindow(startsAt, tournament)
	}

	return clampMatchWindow(tournament.QualificationEndsAt.Add(time.Duration(offsetDays)*day), tournament)
}

func clampMatchWindow(startsAt time.Time, tournament *Tournament) (time.Time, time.Time) {
	const day = 24 * time.Hour

	if startsAt.Before(tournament.QualificationEndsAt) {
		startsAt = tournament.QualificationEndsAt
	}
	latestStart := tournament.EndsAt.Add(-day)
	if startsAt.After(latestStart) {
		startsAt = latestStart
	}
	if startsAt.Before(tournament.QualificationEndsAt) {
		startsAt = tournament.QualificationEndsAt
	}
	endsAt := startsAt.Add(day)
	if endsAt.After(tournament.EndsAt) {
		endsAt = tournament.EndsAt
	}
	if !endsAt.After(startsAt) {
		endsAt = startsAt.Add(time.Hour)
	}
	return startsAt, endsAt
}

func (s *Store) ListTournamentMatches(ctx context.Context, tournamentID string) ([]TournamentMatch, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT
		   m.id, m.tournament_id, m.round, m.position,
		   COALESCE(p1.id, ''), COALESCE(p1.twitch_login, ''), COALESCE(p1.twitch_display_name, ''), COALESCE(p1.minecraft_nick, ''), COALESCE(NULLIF(p1.avatar_url, ''), NULLIF(p1.twitch_profile_image_url, ''), ''),
		   COALESCE(p2.id, ''), COALESCE(p2.twitch_login, ''), COALESCE(p2.twitch_display_name, ''), COALESCE(p2.minecraft_nick, ''), COALESCE(NULLIF(p2.avatar_url, ''), NULLIF(p2.twitch_profile_image_url, ''), ''),
		   m.player1_seed, m.player2_seed, m.starts_at, m.ends_at,
		   m.status,
		   COALESCE(m.winner_participant_id, ''), m.best_of,
		   COALESCE(m.world_seed, ''), m.player1_connected_at, m.player2_connected_at, m.official_started_at,
		   p1_best.time_ms, p2_best.time_ms,
		   COALESCE(winner.twitch_display_name, winner.twitch_login, '')
		 FROM tournament_matches m
		 LEFT JOIN participants p1 ON p1.id = m.player1_participant_id
		 LEFT JOIN participants p2 ON p2.id = m.player2_participant_id
		 LEFT JOIN participants winner ON winner.id = m.winner_participant_id
		 LEFT JOIN LATERAL (
		   SELECT time_ms
		   FROM runs
		   WHERE match_id = m.id AND participant_id = m.player1_participant_id AND status = 'approved'
		   ORDER BY time_ms ASC, finished_at ASC
		   LIMIT 1
		 ) p1_best ON TRUE
		 LEFT JOIN LATERAL (
		   SELECT time_ms
		   FROM runs
		   WHERE match_id = m.id AND participant_id = m.player2_participant_id AND status = 'approved'
		   ORDER BY time_ms ASC, finished_at ASC
		   LIMIT 1
		 ) p2_best ON TRUE
		 WHERE m.tournament_id = $1
		 ORDER BY
		   CASE m.round
		     WHEN 'quarterfinal' THEN 1
		     WHEN 'semifinal' THEN 2
		     WHEN 'third_place' THEN 3
		     WHEN 'final' THEN 4
		     ELSE 9
		   END,
		   m.position`,
		tournamentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTournamentMatches(rows)
}

func (s *Store) OfficialRunContext(ctx context.Context, participantID string) (RunContext, string, error) {
	tournament, err := s.GetCurrentTournament(ctx)
	if err != nil {
		return RunContext{}, "", err
	}
	if tournament == nil || tournament.Phase == "scheduled" || tournament.Phase == "finished" {
		return RunContext{}, "tournament_not_running", nil
	}
	if tournament.Phase == "qualification" {
		return RunContext{TournamentID: tournament.ID, Phase: tournament.Phase}, "", nil
	}

	match, err := s.ActiveMatchForParticipant(ctx, tournament.ID, participantID)
	if err != nil {
		return RunContext{}, "", err
	}
	if match == nil {
		return RunContext{}, "match_window_inactive", nil
	}
	runContext := RunContext{TournamentID: tournament.ID, Phase: phaseForMatchRound(match.Round), MatchID: match.ID, WorldSeed: match.WorldSeed}
	if match.WinnerParticipantID != "" && match.WinnerParticipantID != participantID {
		return runContext, "match_lost", nil
	}
	if match.OfficialStartedAt == nil {
		return runContext, "match_not_started", nil
	}
	return runContext, "", nil
}

func (s *Store) ActiveMatchForParticipant(ctx context.Context, tournamentID, participantID string) (*TournamentMatch, error) {
	query :=
		`SELECT m.id, m.tournament_id, m.round, m.position,
		        '', '', '', '', '',
		        '', '', '', '', '',
		        m.player1_seed, m.player2_seed, m.starts_at, m.ends_at,
		        'running', COALESCE(m.winner_participant_id, ''), m.best_of,
		        COALESCE(m.world_seed, ''), m.player1_connected_at, m.player2_connected_at, m.official_started_at,
		        NULL::int, NULL::int, ''
		   FROM tournament_matches m
		  WHERE m.tournament_id = $1
		    AND (m.player1_participant_id = $2 OR m.player2_participant_id = $2)
		    AND m.status IN ('scheduled', 'running')
		  ORDER BY m.starts_at ASC, m.position ASC
		  LIMIT 1`
	rows, err := s.db.QueryContext(ctx, query, tournamentID, participantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	matches, err := scanTournamentMatches(rows)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return nil, nil
	}
	return &matches[0], nil
}

func (s *Store) MarkActiveMatchParticipantConnected(ctx context.Context, matchID, participantID string) (*TournamentMatch, error) {
	var tournamentID string
	err := s.db.QueryRowContext(ctx,
		`UPDATE tournament_matches
		    SET player1_connected_at = CASE WHEN player1_participant_id = $2 THEN NOW() ELSE player1_connected_at END,
		        player2_connected_at = CASE WHEN player2_participant_id = $2 THEN NOW() ELSE player2_connected_at END,
		        updated_at = NOW()
		  WHERE id = $1
		    AND (player1_participant_id = $2 OR player2_participant_id = $2)
		    AND status IN ('scheduled', 'running')
		  RETURNING tournament_id`,
		matchID, participantID,
	).Scan(&tournamentID)
	if err != nil {
		return nil, err
	}
	return s.TournamentMatchByID(ctx, tournamentID, matchID)
}

func (s *Store) StartTournamentMatch(ctx context.Context, tournamentID, matchID string) (*TournamentMatch, error) {
	err := s.db.QueryRowContext(ctx,
		`UPDATE tournament_matches
		    SET official_started_at = COALESCE(official_started_at, NOW()),
		        status = 'running',
		        updated_at = NOW()
		  WHERE tournament_id = $1
		    AND id = $2
		    AND player1_participant_id IS NOT NULL
		    AND player2_participant_id IS NOT NULL
		    AND player1_connected_at IS NOT NULL
		    AND player2_connected_at IS NOT NULL
		  RETURNING id`,
		tournamentID, matchID,
	).Scan(&matchID)
	if err != nil {
		return nil, err
	}
	return s.TournamentMatchByID(ctx, tournamentID, matchID)
}

func (s *Store) TournamentMatchByID(ctx context.Context, tournamentID, matchID string) (*TournamentMatch, error) {
	matches, err := s.ListTournamentMatches(ctx, tournamentID)
	if err != nil {
		return nil, err
	}
	for i := range matches {
		if matches[i].ID == matchID {
			return &matches[i], nil
		}
	}
	return nil, sql.ErrNoRows
}

func phaseForMatchRound(round string) string {
	if round == "final" {
		return "final"
	}
	return "playoff"
}

func updateMatchWinnerFromRuns(ctx context.Context, tx *sql.Tx, matchID string) (bool, error) {
	var winnerID string
	err := tx.QueryRowContext(ctx,
		`WITH best AS (
		   SELECT r.participant_id
		   FROM runs r
		   JOIN tournament_matches candidate_match ON candidate_match.id = r.match_id
		   WHERE r.match_id = $1
		     AND r.status = 'approved'
		     AND r.participant_id IN (
		       candidate_match.player1_participant_id,
		       candidate_match.player2_participant_id
		     )
		   ORDER BY r.time_ms ASC, r.finished_at ASC, r.created_at ASC
		   LIMIT 1
		 )
		 UPDATE tournament_matches
		 SET winner_participant_id = (SELECT participant_id FROM best),
		     status = 'finished',
		     updated_at = NOW()
		 WHERE id = $1
		   AND best_of = 1
		   AND status = 'running'
		   AND EXISTS (SELECT 1 FROM best)
		 RETURNING winner_participant_id`,
		matchID,
	).Scan(&winnerID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	if winnerID != "" {
		if _, err := tx.ExecContext(ctx,
			`UPDATE match_predictions
			    SET points_awarded = CASE
			      WHEN predicted_winner_id <> $2 THEN 0
			      WHEN (SELECT round FROM tournament_matches WHERE id = $1) = 'quarterfinal' THEN 5
			      WHEN (SELECT round FROM tournament_matches WHERE id = $1) = 'semifinal' THEN 15
			      WHEN (SELECT round FROM tournament_matches WHERE id = $1) = 'final' THEN 50
			      ELSE 0
			    END
			  WHERE match_id = $1
			    AND EXISTS (
			      SELECT 1 FROM pickem_submissions ps
			      JOIN tournament_matches tm ON tm.tournament_id = ps.tournament_id
			      WHERE ps.twitch_user_id = match_predictions.twitch_user_id AND tm.id = match_predictions.match_id
			    )`,
			matchID, winnerID,
		); err != nil {
			return false, err
		}
	}

	if err := advanceWinnerInBracket(ctx, tx, matchID); err != nil {
		return false, err
	}
	return true, nil
}

func insertTournamentMatch(ctx context.Context, tx *sql.Tx, tournamentID, round string, position int, player1ID, player2ID string, player1Seed, player2Seed *int, startsAt, endsAt time.Time, bestOf int) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO tournament_matches (
		   id, tournament_id, round, position, player1_participant_id, player2_participant_id,
		   player1_seed, player2_seed, starts_at, ends_at, status, best_of, world_seed
		 )
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'scheduled',$11,$12)`,
		uid("tm"), tournamentID, round, position, nullableString(player1ID), nullableString(player2ID), nullableInt(player1Seed), nullableInt(player2Seed), startsAt, endsAt, bestOf, newWorldSeed(),
	)
	return err
}

func newWorldSeed() string {
	limit := new(big.Int).Lsh(big.NewInt(1), 63)
	value, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	if value.Bit(0) == 1 {
		return "-" + value.String()
	}
	return value.String()
}

func advanceWinnerInBracket(ctx context.Context, tx *sql.Tx, matchID string) error {
	var tournamentID, round, winnerID string
	var position int
	if err := tx.QueryRowContext(ctx,
		`SELECT tournament_id, round, position, COALESCE(winner_participant_id, '')
		 FROM tournament_matches
		 WHERE id = $1`,
		matchID,
	).Scan(&tournamentID, &round, &position, &winnerID); err != nil {
		return err
	}
	if winnerID == "" {
		return nil
	}

	targetRound := ""
	targetPosition := 0
	targetColumn := ""
	switch round {
	case "quarterfinal":
		targetRound = "semifinal"
		targetPosition = (position + 1) / 2
		if position%2 == 1 {
			targetColumn = "player1_participant_id"
		} else {
			targetColumn = "player2_participant_id"
		}
	case "semifinal":
		targetRound = "final"
		targetPosition = 1
		if position == 1 {
			targetColumn = "player1_participant_id"
		} else {
			targetColumn = "player2_participant_id"
		}
	default:
		return nil
	}

	query := fmt.Sprintf(
		`UPDATE tournament_matches
		 SET %s = $1,
		     updated_at = NOW()
		 WHERE tournament_id = $2
		   AND round = $3
		   AND position = $4
		   AND (%s IS NULL OR %s = $1)`,
		targetColumn,
		targetColumn,
		targetColumn,
	)
	_, err := tx.ExecContext(ctx, query, winnerID, tournamentID, targetRound, targetPosition)
	return err
}

type bracketParticipant struct {
	id   string
	seed int
}

func topParticipantsForBracket(ctx context.Context, tx *sql.Tx, limit int) ([]bracketParticipant, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT id
		 FROM participants
		 WHERE status IN ('invited','active')
		   AND best_time_ms IS NOT NULL
		 ORDER BY best_time_ms ASC, updated_at ASC, id ASC
		 LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var participants []bracketParticipant
	for rows.Next() {
		var participant bracketParticipant
		if err := rows.Scan(&participant.id); err != nil {
			return nil, err
		}
		participant.seed = len(participants) + 1
		participants = append(participants, participant)
	}
	return participants, rows.Err()
}

func openingRound(size int) string {
	switch {
	case size <= 2:
		return "final"
	case size <= 4:
		return "semifinal"
	default:
		return "quarterfinal"
	}
}

func bestOfForRound(string) int {
	return 1
}

func bracketSize(size int) int {
	switch {
	case size >= 16:
		return 16
	case size >= 8:
		return 8
	case size >= 4:
		return 4
	default:
		return 2
	}
}

func seedPairs(size int) [][2]int {
	if size >= 8 {
		return [][2]int{{1, 8}, {4, 5}, {2, 7}, {3, 6}}
	}
	if size >= 4 {
		return [][2]int{{1, 4}, {2, 3}}
	}
	return [][2]int{{1, 2}}
}

func scanTournamentMatches(rows *sql.Rows) ([]TournamentMatch, error) {
	var matches []TournamentMatch
	for rows.Next() {
		var match TournamentMatch
		var player1ID, player1Login, player1DisplayName, player1MinecraftNick, player1Avatar string
		var player2ID, player2Login, player2DisplayName, player2MinecraftNick, player2Avatar string
		var player1Seed sql.NullInt64
		var player2Seed sql.NullInt64
		var player1Best sql.NullInt64
		var player2Best sql.NullInt64
		var player1ConnectedAt sql.NullTime
		var player2ConnectedAt sql.NullTime
		var officialStartedAt sql.NullTime
		if err := rows.Scan(
			&match.ID,
			&match.TournamentID,
			&match.Round,
			&match.Position,
			&player1ID,
			&player1Login,
			&player1DisplayName,
			&player1MinecraftNick,
			&player1Avatar,
			&player2ID,
			&player2Login,
			&player2DisplayName,
			&player2MinecraftNick,
			&player2Avatar,
			&player1Seed,
			&player2Seed,
			&match.StartsAt,
			&match.EndsAt,
			&match.Status,
			&match.WinnerParticipantID,
			&match.BestOf,
			&match.WorldSeed,
			&player1ConnectedAt,
			&player2ConnectedAt,
			&officialStartedAt,
			&player1Best,
			&player2Best,
			&match.WinnerDisplayName,
		); err != nil {
			return nil, err
		}
		if player1ID != "" {
			match.Player1 = &Participant{ID: player1ID, TwitchLogin: player1Login, TwitchDisplayName: player1DisplayName, MinecraftNick: player1MinecraftNick, AvatarURL: player1Avatar}
		}
		if player2ID != "" {
			match.Player2 = &Participant{ID: player2ID, TwitchLogin: player2Login, TwitchDisplayName: player2DisplayName, MinecraftNick: player2MinecraftNick, AvatarURL: player2Avatar}
		}
		if player1Seed.Valid {
			value := int(player1Seed.Int64)
			match.Player1Seed = &value
		}
		if player2Seed.Valid {
			value := int(player2Seed.Int64)
			match.Player2Seed = &value
		}
		if player1ConnectedAt.Valid {
			value := player1ConnectedAt.Time
			match.Player1ConnectedAt = &value
		}
		if player2ConnectedAt.Valid {
			value := player2ConnectedAt.Time
			match.Player2ConnectedAt = &value
		}
		if officialStartedAt.Valid {
			value := officialStartedAt.Time
			match.OfficialStartedAt = &value
		}
		if player1Best.Valid {
			value := int(player1Best.Int64)
			match.Player1BestTimeMS = &value
		}
		if player2Best.Valid {
			value := int(player2Best.Int64)
			match.Player2BestTimeMS = &value
		}
		matches = append(matches, match)
	}
	return matches, rows.Err()
}

func (s *Store) ResetTournamentMatch(ctx context.Context, tournamentID, matchID string) (*TournamentMatch, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var round, winnerID string
	var position int
	err = tx.QueryRowContext(ctx,
		`SELECT round, position, COALESCE(winner_participant_id, '')
		   FROM tournament_matches
		  WHERE tournament_id = $1 AND id = $2`,
		tournamentID, matchID,
	).Scan(&round, &position, &winnerID)
	if err != nil {
		return nil, err
	}

	_, err = tx.ExecContext(ctx,
		`DELETE FROM mod_run_sessions WHERE match_id = $1 AND tournament_id = $2`,
		matchID, tournamentID,
	)
	if err != nil {
		return nil, err
	}

	_, err = tx.ExecContext(ctx,
		`DELETE FROM runs WHERE match_id = $1 AND tournament_id = $2`,
		matchID, tournamentID,
	)
	if err != nil {
		return nil, err
	}

	if winnerID != "" {
		targetRound := ""
		targetPosition := 0
		targetColumn := ""
		switch round {
		case "quarterfinal":
			targetRound = "semifinal"
			targetPosition = (position + 1) / 2
			if position%2 == 1 {
				targetColumn = "player1_participant_id"
			} else {
				targetColumn = "player2_participant_id"
			}
		case "semifinal":
			targetRound = "final"
			targetPosition = 1
			if position == 1 {
				targetColumn = "player1_participant_id"
			} else {
				targetColumn = "player2_participant_id"
			}
		}

		if targetRound != "" {
			query := fmt.Sprintf(
				`UPDATE tournament_matches
				    SET %s = NULL,
				        updated_at = NOW()
				  WHERE tournament_id = $1
				    AND round = $2
				    AND position = $3
				    AND %s = $4`,
				targetColumn, targetColumn,
			)
			_, err = tx.ExecContext(ctx, query, tournamentID, targetRound, targetPosition, winnerID)
			if err != nil {
				return nil, err
			}
		}
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE match_predictions
		    SET points_awarded = NULL
		  WHERE match_id = $1`,
		matchID,
	)
	if err != nil {
		return nil, err
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE tournament_matches
		    SET status = 'scheduled',
		        official_started_at = NULL,
		        player1_connected_at = NULL,
		        player2_connected_at = NULL,
		        winner_participant_id = NULL,
		        updated_at = NOW()
		  WHERE tournament_id = $1 AND id = $2`,
		tournamentID, matchID,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.TournamentMatchByID(ctx, tournamentID, matchID)
}
