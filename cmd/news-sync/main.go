package main

import (
	"context"
	"log"
	"time"

	"github.com/dimension-science/tournament-kit/internal/config"
	"github.com/dimension-science/tournament-kit/internal/store"
	"github.com/dimension-science/tournament-kit/internal/telegrambot"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	db, err := store.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := db.EnsureSchema(ctx); err != nil {
		log.Fatalf("ensure schema: %v", err)
	}

	result, err := telegrambot.SyncPublicChannelNewsHistory(ctx, cfg.TelegramNewsChat, cfg.UploadDir, db, log.Default())
	if err != nil {
		log.Fatalf("sync news: %v", err)
	}
	log.Printf("news sync complete: imported=%d seen=%d", result.Imported, result.Seen)
}
