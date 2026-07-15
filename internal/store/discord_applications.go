package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

type DiscordApplication struct {
	ID                string     `json:"id"`
	ApplicationNumber int64      `json:"applicationNumber"`
	GuildID           string     `json:"guildId"`
	DiscordUserID     string     `json:"discordUserId"`
	DiscordUsername   string     `json:"discordUsername"`
	ProjectKey        string     `json:"projectKey"`
	GameNick          string     `json:"gameNick"`
	Timezone          string     `json:"timezone"`
	Experience        string     `json:"experience"`
	Motivation        string     `json:"motivation"`
	Links             string     `json:"links"`
	Status            string     `json:"status"`
	ReviewerDiscordID string     `json:"reviewerDiscordId,omitempty"`
	ReviewReason      string     `json:"reviewReason,omitempty"`
	LogChannelID      string     `json:"logChannelId,omitempty"`
	LogMessageID      string     `json:"logMessageId,omitempty"`
	ReviewedAt        *time.Time `json:"reviewedAt,omitempty"`
	RoleGrantedAt     *time.Time `json:"roleGrantedAt,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

type DiscordApplicationInput struct {
	GuildID         string
	DiscordUserID   string
	DiscordUsername string
	ProjectKey      string
	GameNick        string
	Timezone        string
	Experience      string
	Motivation      string
	Links           string
}

var ErrActiveDiscordApplication = errors.New("active Discord application already exists")

func (s *Store) CreateDiscordApplication(ctx context.Context, input DiscordApplicationInput) (*DiscordApplication, error) {
	id, err := randomDiscordApplicationID()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.ProjectKey) == "" {
		input.ProjectKey = "chaos"
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO discord_applications (
			id, guild_id, discord_user_id, discord_username, project_key,
			game_nick, timezone, experience, motivation, links
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		id, strings.TrimSpace(input.GuildID), strings.TrimSpace(input.DiscordUserID),
		strings.TrimSpace(input.DiscordUsername), strings.TrimSpace(input.ProjectKey),
		strings.TrimSpace(input.GameNick), strings.TrimSpace(input.Timezone),
		strings.TrimSpace(input.Experience), strings.TrimSpace(input.Motivation), strings.TrimSpace(input.Links))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "discord_applications_active_user_idx") {
			return nil, ErrActiveDiscordApplication
		}
		return nil, err
	}
	return s.FindDiscordApplicationByID(ctx, id)
}

func (s *Store) FindDiscordApplicationByID(ctx context.Context, id string) (*DiscordApplication, error) {
	return scanDiscordApplication(s.db.QueryRowContext(ctx, discordApplicationSelect+" WHERE id = $1", strings.TrimSpace(id)))
}

func (s *Store) ListDiscordApplications(ctx context.Context) ([]DiscordApplication, error) {
	rows, err := s.db.QueryContext(ctx, discordApplicationSelect+" ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []DiscordApplication
	for rows.Next() {
		app, err := scanDiscordApplication(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *app)
	}
	return result, rows.Err()
}

func (s *Store) SetDiscordApplicationLogMessage(ctx context.Context, id, channelID, messageID string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE discord_applications SET log_channel_id=$2, log_message_id=$3, updated_at=NOW() WHERE id=$1`, id, channelID, messageID)
	return err
}

func (s *Store) ReviewDiscordApplication(ctx context.Context, id, status, reviewerID, reason string, roleGranted bool) (*DiscordApplication, error) {
	if status != "approved" && status != "rejected" && status != "needs_info" {
		return nil, errors.New("invalid Discord application status")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var current string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM discord_applications WHERE id=$1 FOR UPDATE`, id).Scan(&current); err != nil {
		return nil, err
	}
	if current != "pending" && current != "needs_info" {
		return nil, errors.New("application has already been reviewed")
	}
	_, err = tx.ExecContext(ctx, `UPDATE discord_applications SET status=$2, reviewer_discord_id=$3, review_reason=$4,
		reviewed_at=CASE WHEN $2 IN ('approved','rejected') THEN NOW() ELSE reviewed_at END,
		role_granted_at=CASE WHEN $5 THEN NOW() ELSE role_granted_at END, updated_at=NOW() WHERE id=$1`,
		id, status, reviewerID, strings.TrimSpace(reason), roleGranted)
	if err != nil {
		return nil, err
	}
	auditID, err := randomDiscordApplicationID()
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO discord_application_audit (id, application_id, actor_discord_id, action, details) VALUES ($1,$2,$3,$4,$5)`, auditID, id, reviewerID, status, strings.TrimSpace(reason)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.FindDiscordApplicationByID(ctx, id)
}

const discordApplicationSelect = `SELECT id, application_number, guild_id, discord_user_id, discord_username,
	project_key, game_nick, timezone, experience, motivation, COALESCE(links,''), status,
	COALESCE(reviewer_discord_id,''), COALESCE(review_reason,''), COALESCE(log_channel_id,''),
	COALESCE(log_message_id,''), reviewed_at, role_granted_at, created_at, updated_at FROM discord_applications`

func scanDiscordApplication(scanner interface{ Scan(...any) error }) (*DiscordApplication, error) {
	var app DiscordApplication
	err := scanner.Scan(&app.ID, &app.ApplicationNumber, &app.GuildID, &app.DiscordUserID, &app.DiscordUsername,
		&app.ProjectKey, &app.GameNick, &app.Timezone, &app.Experience, &app.Motivation, &app.Links,
		&app.Status, &app.ReviewerDiscordID, &app.ReviewReason, &app.LogChannelID, &app.LogMessageID,
		&app.ReviewedAt, &app.RoleGrantedAt, &app.CreatedAt, &app.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &app, nil
}

func randomDiscordApplicationID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "dsa_" + hex.EncodeToString(buf), nil
}
