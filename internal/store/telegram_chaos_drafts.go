package store

import (
	"context"
	"database/sql"
	"errors"
)

type TelegramChaosApplicationDraft struct {
	ChatID          int64
	Step            int
	GameNick        string
	Timezone        string
	Experience      string
	DiscordID       string
	DiscordUsername string
	Motivation      string
	Links           string
}

func (s *Store) SaveTelegramChaosApplicationDraft(ctx context.Context, draft TelegramChaosApplicationDraft) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO telegram_chaos_application_drafts (
			chat_id, step, game_nick, timezone, experience, discord_id, discord_username, motivation, links, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW())
		ON CONFLICT (chat_id) DO UPDATE SET
			step = EXCLUDED.step,
			game_nick = EXCLUDED.game_nick,
			timezone = EXCLUDED.timezone,
			experience = EXCLUDED.experience,
			discord_id = EXCLUDED.discord_id,
			discord_username = EXCLUDED.discord_username,
			motivation = EXCLUDED.motivation,
			links = EXCLUDED.links,
			updated_at = NOW()`,
		draft.ChatID, draft.Step, draft.GameNick, draft.Timezone, draft.Experience,
		draft.DiscordID, draft.DiscordUsername, draft.Motivation, draft.Links)
	return err
}

func (s *Store) FindTelegramChaosApplicationDraft(ctx context.Context, chatID int64) (*TelegramChaosApplicationDraft, error) {
	var draft TelegramChaosApplicationDraft
	err := s.db.QueryRowContext(ctx, `
		SELECT chat_id, step, game_nick, timezone, experience, discord_id, discord_username, motivation, links
		FROM telegram_chaos_application_drafts
		WHERE chat_id = $1`, chatID).Scan(
		&draft.ChatID, &draft.Step, &draft.GameNick, &draft.Timezone, &draft.Experience,
		&draft.DiscordID, &draft.DiscordUsername, &draft.Motivation, &draft.Links)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &draft, nil
}

func (s *Store) DeleteTelegramChaosApplicationDraft(ctx context.Context, chatID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM telegram_chaos_application_drafts WHERE chat_id = $1`, chatID)
	return err
}
