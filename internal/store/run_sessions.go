package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

const qualificationRunSessionTTL = 240 * time.Minute

func (s *Store) CreateModRunSession(ctx context.Context, participantID, worldFingerprint, seedHash string, runContext RunContext) (*ModRunSession, error) {
	worldFingerprint = strings.TrimSpace(worldFingerprint)
	seedHash = strings.TrimSpace(seedHash)
	if participantID == "" || worldFingerprint == "" || seedHash == "" {
		return nil, sql.ErrNoRows
	}

	startedAt := time.Now().UTC()
	expiresAt := startedAt.Add(qualificationRunSessionTTL)
	if runContext.MatchID != "" {
		var matchEndsAt time.Time
		err := s.db.QueryRowContext(ctx,
			`SELECT ends_at
			   FROM tournament_matches
			  WHERE id = $1
			    AND (player1_participant_id = $2 OR player2_participant_id = $2)`,
			runContext.MatchID, participantID,
		).Scan(&matchEndsAt)
		if err != nil {
			return nil, err
		}
		if matchEndsAt.After(expiresAt) {
			expiresAt = matchEndsAt.UTC()
		}
	}

	session := &ModRunSession{}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO mod_run_sessions (
		   id, participant_id, world_fingerprint, seed_hash, tournament_id, phase, match_id, status, started_at, expires_at
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,'active',$8,$9)
		 RETURNING id, participant_id, world_fingerprint, seed_hash,
		           COALESCE(tournament_id, ''), COALESCE(phase, ''), COALESCE(match_id, ''),
		           status, started_at, expires_at, COALESCE(completed_run_id, ''), completed_at`,
		uid("rs"), participantID, worldFingerprint, seedHash, nullableString(runContext.TournamentID), nullableString(runContext.Phase), nullableString(runContext.MatchID), startedAt, expiresAt,
	).Scan(
		&session.ID,
		&session.ParticipantID,
		&session.WorldFingerprint,
		&session.SeedHash,
		&session.TournamentID,
		&session.Phase,
		&session.MatchID,
		&session.Status,
		&session.StartedAt,
		&session.ExpiresAt,
		&session.CompletedRunID,
		&session.CompletedAt,
	)
	return session, err
}

func (s *Store) GetModRunSession(ctx context.Context, sessionID, participantID string) (*ModRunSession, error) {
	sessionID = strings.TrimSpace(sessionID)
	participantID = strings.TrimSpace(participantID)
	if sessionID == "" || participantID == "" {
		return nil, sql.ErrNoRows
	}
	session := &ModRunSession{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, participant_id, world_fingerprint, seed_hash,
		        COALESCE(tournament_id, ''), COALESCE(phase, ''), COALESCE(match_id, ''),
		        status, started_at, expires_at, COALESCE(completed_run_id, ''), completed_at
		   FROM mod_run_sessions
		  WHERE id = $1
		    AND participant_id = $2`,
		sessionID, participantID,
	).Scan(
		&session.ID,
		&session.ParticipantID,
		&session.WorldFingerprint,
		&session.SeedHash,
		&session.TournamentID,
		&session.Phase,
		&session.MatchID,
		&session.Status,
		&session.StartedAt,
		&session.ExpiresAt,
		&session.CompletedRunID,
		&session.CompletedAt,
	)
	if err != nil {
		return nil, err
	}
	return session, nil
}

func (s *Store) CompleteModRunSession(ctx context.Context, sessionID, participantID, runID string) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE mod_run_sessions
		    SET status = 'completed',
		        completed_run_id = $3,
		        completed_at = NOW(),
		        updated_at = NOW()
		  WHERE id = $1
		    AND participant_id = $2
		    AND status = 'active'
		    AND expires_at >= NOW()`,
		sessionID, participantID, runID,
	)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func ValidateModRunSession(session *ModRunSession, worldFingerprint, seedHash string, runContext RunContext) string {
	if session == nil {
		return "run_session_missing"
	}
	if session.Status != "active" {
		return "run_session_closed"
	}
	if !session.ExpiresAt.After(time.Now().UTC()) {
		return "run_session_expired"
	}
	if session.WorldFingerprint != strings.TrimSpace(worldFingerprint) || session.SeedHash != strings.TrimSpace(seedHash) {
		return "run_session_world_mismatch"
	}
	if session.TournamentID != runContext.TournamentID || session.Phase != runContext.Phase || session.MatchID != runContext.MatchID {
		return "run_session_context_changed"
	}
	return ""
}

func IsRunSessionNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
