package store

import (
	"context"
	"errors"
	"strconv"
)

var (
	ErrPredictionsLocked  = errors.New("predictions_locked")
	ErrPickemsSubmitted   = errors.New("pickems_already_submitted")
	ErrIncompletePickems  = errors.New("incomplete_pickems")
	ErrInvalidPickemChain = errors.New("invalid_pickem_chain")
)

func PredictionPointsForRound(round string) int {
	switch round {
	case "quarterfinal":
		return 5
	case "semifinal":
		return 15
	case "final":
		return 50
	default:
		return 0
	}
}

func validPickemChain(quarterfinals [4]string, semifinals [2]string, final string) bool {
	return (semifinals[0] == quarterfinals[0] || semifinals[0] == quarterfinals[1]) &&
		(semifinals[1] == quarterfinals[2] || semifinals[1] == quarterfinals[3]) &&
		(final == semifinals[0] || final == semifinals[1])
}

func (s *Store) HasSubmittedPickems(ctx context.Context, twitchUserID, tournamentID string) (bool, error) {
	var submitted bool
	err := s.db.QueryRowContext(ctx,
		`SELECT EXISTS(
		   SELECT 1 FROM pickem_submissions
		   WHERE twitch_user_id = $1 AND tournament_id = $2
		 )`,
		twitchUserID, tournamentID,
	).Scan(&submitted)
	return submitted, err
}

func (s *Store) SubmitPickems(ctx context.Context, twitchUserID, twitchLogin, tournamentID string, predictions map[string]string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var submitted bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS(
		   SELECT 1 FROM pickem_submissions
		   WHERE twitch_user_id = $1 AND tournament_id = $2
		 )`, twitchUserID, tournamentID,
	).Scan(&submitted); err != nil {
		return err
	}
	if submitted {
		return ErrPickemsSubmitted
	}

	type matchInfo struct {
		id       string
		round    string
		position int
		started  bool
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT id, round, position, official_started_at IS NOT NULL
		 FROM tournament_matches
		 WHERE tournament_id = $1 AND round IN ('quarterfinal', 'semifinal', 'final')
		 ORDER BY CASE round WHEN 'quarterfinal' THEN 1 WHEN 'semifinal' THEN 2 ELSE 3 END, position`,
		tournamentID,
	)
	if err != nil {
		return err
	}
	var matches []matchInfo
	for rows.Next() {
		var m matchInfo
		if err := rows.Scan(&m.id, &m.round, &m.position, &m.started); err != nil {
			rows.Close()
			return err
		}
		matches = append(matches, m)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(matches) != 7 || len(predictions) != len(matches) {
		return ErrIncompletePickems
	}

	matchByRoundPosition := make(map[string]matchInfo, len(matches))
	for _, m := range matches {
		if m.started {
			return ErrPredictionsLocked
		}
		if predictions[m.id] == "" {
			return ErrIncompletePickems
		}
		matchByRoundPosition[m.round+":"+strconv.Itoa(m.position)] = m
	}

	qfWinner := func(position int) string {
		return predictions[matchByRoundPosition["quarterfinal:"+strconv.Itoa(position)].id]
	}
	sf1 := matchByRoundPosition["semifinal:1"]
	sf2 := matchByRoundPosition["semifinal:2"]
	final := matchByRoundPosition["final:1"]
	sf1Winner := predictions[sf1.id]
	sf2Winner := predictions[sf2.id]
	if !validPickemChain(
		[4]string{qfWinner(1), qfWinner(2), qfWinner(3), qfWinner(4)},
		[2]string{sf1Winner, sf2Winner},
		predictions[final.id],
	) {
		return ErrInvalidPickemChain
	}

	for _, m := range matches {
		winnerID := predictions[m.id]
		var exists bool
		if err := tx.QueryRowContext(ctx,
			`SELECT EXISTS(
			   SELECT 1 FROM tournament_matches
			   WHERE tournament_id = $1 AND round = 'quarterfinal'
			     AND (player1_participant_id = $2 OR player2_participant_id = $2)
			 )`, tournamentID, winnerID,
		).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrInvalidPickemChain
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO match_predictions (twitch_user_id, twitch_login, match_id, predicted_winner_id, updated_at)
			 VALUES ($1, $2, $3, $4, NOW())
			 ON CONFLICT (twitch_user_id, match_id)
			 DO UPDATE SET predicted_winner_id = EXCLUDED.predicted_winner_id, twitch_login = EXCLUDED.twitch_login, updated_at = NOW()`,
			twitchUserID, twitchLogin, m.id, winnerID,
		); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO pickem_submissions (twitch_user_id, tournament_id, twitch_login)
		 VALUES ($1, $2, $3)`, twitchUserID, tournamentID, twitchLogin,
	); err != nil {
		return err
	}
	return tx.Commit()
}

type PredictionLeaderboardRow struct {
	TwitchLogin    string `json:"twitchLogin"`
	TotalPoints    int    `json:"totalPoints"`
	CorrectGuesses int    `json:"correctGuesses"`
}

type MatchPredictionStats struct {
	MatchID        string `json:"matchId"`
	Player1Votes   int    `json:"player1Votes"`
	Player2Votes   int    `json:"player2Votes"`
	Player1Percent int    `json:"player1Percent"`
	Player2Percent int    `json:"player2Percent"`
}

func (s *Store) GetUserPredictions(ctx context.Context, twitchUserID string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT match_id, predicted_winner_id
		   FROM match_predictions
		  WHERE twitch_user_id = $1`,
		twitchUserID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	predictions := make(map[string]string)
	for rows.Next() {
		var matchID, predictedWinnerID string
		if err := rows.Scan(&matchID, &predictedWinnerID); err != nil {
			return nil, err
		}
		predictions[matchID] = predictedWinnerID
	}
	return predictions, nil
}

func (s *Store) GetPredictionsLeaderboard(ctx context.Context) ([]PredictionLeaderboardRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT twitch_login,
		        SUM(COALESCE(points_awarded, 0)) as total_points,
		        COUNT(CASE WHEN points_awarded > 0 THEN 1 END) as correct_guesses
		   FROM match_predictions mp
		  WHERE EXISTS (
		    SELECT 1 FROM pickem_submissions ps
		    JOIN tournament_matches tm ON tm.tournament_id = ps.tournament_id
		    WHERE ps.twitch_user_id = mp.twitch_user_id AND tm.id = mp.match_id
		  )
		  GROUP BY twitch_login
		  ORDER BY total_points DESC, correct_guesses DESC, twitch_login ASC
		  LIMIT 50`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var leaderboard []PredictionLeaderboardRow
	for rows.Next() {
		var row PredictionLeaderboardRow
		if err := rows.Scan(&row.TwitchLogin, &row.TotalPoints, &row.CorrectGuesses); err != nil {
			return nil, err
		}
		leaderboard = append(leaderboard, row)
	}
	return leaderboard, nil
}

func (s *Store) GetMatchPredictionStats(ctx context.Context, matchID, player1ID, player2ID string) (MatchPredictionStats, error) {
	stats := MatchPredictionStats{MatchID: matchID}

	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(CASE WHEN predicted_winner_id = $2 THEN 1 END) as p1_votes,
		        COUNT(CASE WHEN predicted_winner_id = $3 THEN 1 END) as p2_votes
		   FROM match_predictions mp
		  WHERE match_id = $1
		    AND EXISTS (
		      SELECT 1 FROM pickem_submissions ps
		      JOIN tournament_matches tm ON tm.tournament_id = ps.tournament_id
		      WHERE ps.twitch_user_id = mp.twitch_user_id AND tm.id = mp.match_id
		    )`,
		matchID, player1ID, player2ID,
	).Scan(&stats.Player1Votes, &stats.Player2Votes)
	if err != nil {
		return stats, err
	}

	total := stats.Player1Votes + stats.Player2Votes
	if total > 0 {
		stats.Player1Percent = (stats.Player1Votes * 100) / total
		stats.Player2Percent = 100 - stats.Player1Percent
	} else {
		// Default to 50/50 if no votes
		stats.Player1Percent = 50
		stats.Player2Percent = 50
	}

	return stats, nil
}
