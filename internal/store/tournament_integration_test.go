package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestUpdateMatchWinnerFromRunsCompletesBO1Atomically(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for the tournament transaction test")
	}

	s, err := Open(databaseURL)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure test schema: %v", err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tournamentID := "test_tournament_" + suffix
	winnerID := "test_winner_" + suffix
	loserID := "test_loser_" + suffix
	matchID := "test_match_" + suffix
	nextMatchID := "test_next_match_" + suffix
	runID := "test_run_" + suffix
	predictorID := "test_predictor_" + suffix

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = s.db.ExecContext(cleanupCtx, `DELETE FROM tournament_periods WHERE id = $1`, tournamentID)
		_, _ = s.db.ExecContext(cleanupCtx, `DELETE FROM participants WHERE id IN ($1, $2)`, winnerID, loserID)
	}()

	for _, participant := range []struct {
		id    string
		login string
	}{
		{id: winnerID, login: "winner_" + suffix},
		{id: loserID, login: "loser_" + suffix},
	} {
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO participants (id, twitch_user_id, twitch_login, twitch_display_name, status)
			 VALUES ($1, $2, $3, $3, 'active')`,
			participant.id, participant.id, participant.login,
		); err != nil {
			t.Fatalf("insert participant %s: %v", participant.id, err)
		}
	}

	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO tournament_periods (
		   id, starts_at, qualification_ends_at, playoff_ends_at, ends_at,
		   state, playoff_slots, bracket_generated_at
		 ) VALUES ($1, $2, $3, $4, $5, 'running', 8, NOW())`,
		tournamentID,
		now.Add(-48*time.Hour),
		now.Add(-24*time.Hour),
		now.Add(24*time.Hour),
		now.Add(48*time.Hour),
	); err != nil {
		t.Fatalf("insert tournament: %v", err)
	}

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO tournament_matches (
		   id, tournament_id, round, position,
		   player1_participant_id, player2_participant_id,
		   starts_at, ends_at, status, best_of, official_started_at
		 ) VALUES
		   ($1, $3, 'quarterfinal', 1, $4, $5, $6, $7, 'running', 1, NOW()),
		   ($2, $3, 'semifinal', 1, NULL, NULL, $8, $9, 'scheduled', 1, NULL)`,
		matchID,
		nextMatchID,
		tournamentID,
		winnerID,
		loserID,
		now.Add(-time.Hour),
		now.Add(5*time.Hour),
		now.Add(24*time.Hour),
		now.Add(29*time.Hour),
	); err != nil {
		t.Fatalf("insert matches: %v", err)
	}

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO pickem_submissions (twitch_user_id, tournament_id, twitch_login)
		 VALUES ($1, $2, $1)`,
		predictorID, tournamentID,
	); err != nil {
		t.Fatalf("insert pickem submission: %v", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO match_predictions (
		   twitch_user_id, twitch_login, match_id, predicted_winner_id
		 ) VALUES ($1, $1, $2, $3)`,
		predictorID, matchID, winnerID,
	); err != nil {
		t.Fatalf("insert prediction: %v", err)
	}

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO runs (
		   id, participant_id, time_ms, started_at, finished_at,
		   source, status, tournament_id, phase, match_id
		 ) VALUES ($1, $2, 1000, $3, $4, 'mod', 'approved', $5, 'playoff', $6)`,
		runID,
		winnerID,
		now.Add(-2*time.Second),
		now.Add(-time.Second),
		tournamentID,
		matchID,
	); err != nil {
		t.Fatalf("insert approved run: %v", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin completion transaction: %v", err)
	}
	completed, err := updateMatchWinnerFromRuns(ctx, tx, matchID)
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("complete match: %v", err)
	}
	if !completed {
		_ = tx.Rollback()
		t.Fatal("expected running BO1 match to complete")
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit completion transaction: %v", err)
	}

	var status, actualWinnerID string
	if err := s.db.QueryRowContext(ctx,
		`SELECT status, COALESCE(winner_participant_id, '')
		 FROM tournament_matches
		 WHERE id = $1`,
		matchID,
	).Scan(&status, &actualWinnerID); err != nil {
		t.Fatalf("load completed match: %v", err)
	}
	if status != "finished" {
		t.Fatalf("match status = %q, want finished", status)
	}
	if actualWinnerID != winnerID {
		t.Fatalf("winner = %q, want %q", actualWinnerID, winnerID)
	}

	var advancedWinnerID string
	if err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(player1_participant_id, '')
		 FROM tournament_matches
		 WHERE id = $1`,
		nextMatchID,
	).Scan(&advancedWinnerID); err != nil {
		t.Fatalf("load next match: %v", err)
	}
	if advancedWinnerID != winnerID {
		t.Fatalf("advanced winner = %q, want %q", advancedWinnerID, winnerID)
	}

	var points int
	if err := s.db.QueryRowContext(ctx,
		`SELECT points_awarded
		 FROM match_predictions
		 WHERE twitch_user_id = $1 AND match_id = $2`,
		predictorID, matchID,
	).Scan(&points); err != nil {
		t.Fatalf("load prediction points: %v", err)
	}
	if points != 5 {
		t.Fatalf("prediction points = %d, want 5", points)
	}

	_, certificateNumber, available, err := s.GetParticipantCertificateStatus(ctx, loserID)
	if err != nil {
		t.Fatalf("get loser certificate status: %v", err)
	}
	if !available || certificateNumber == "" {
		t.Fatalf("loser certificate available=%v number=%q, want available certificate", available, certificateNumber)
	}

	retryTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin retry transaction: %v", err)
	}
	completedAgain, err := updateMatchWinnerFromRuns(ctx, retryTx, matchID)
	if err != nil {
		_ = retryTx.Rollback()
		t.Fatalf("repeat match completion: %v", err)
	}
	if completedAgain {
		_ = retryTx.Rollback()
		t.Fatal("completed match transitioned a second time")
	}
	if err := retryTx.Commit(); err != nil {
		t.Fatalf("commit retry transaction: %v", err)
	}
}
