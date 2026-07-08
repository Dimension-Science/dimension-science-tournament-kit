package main

import (
	"context"
	"flag"
	"log"
	"strings"
	"time"

	"github.com/dimension-science/tournament-kit/internal/config"
	"github.com/dimension-science/tournament-kit/internal/store"
)

func main() {
	userID := flag.String("user-id", "", "Twitch user id")
	login := flag.String("login", "", "Twitch login")
	displayName := flag.String("display-name", "", "Twitch display name")
	flag.Parse()

	trimmedUserID := strings.TrimSpace(*userID)
	trimmedLogin := strings.ToLower(strings.TrimSpace(*login))
	trimmedDisplayName := strings.TrimSpace(*displayName)
	if trimmedUserID == "" || trimmedLogin == "" {
		log.Fatal("both --user-id and --login are required")
	}
	if trimmedDisplayName == "" {
		trimmedDisplayName = trimmedLogin
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	db, err := store.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if err := db.EnsureSchema(ctx); err != nil {
		log.Fatalf("ensure schema: %v", err)
	}

	participant, err := db.UpsertWhitelistParticipant(ctx, trimmedUserID, trimmedLogin, trimmedDisplayName)
	if err != nil {
		log.Fatalf("whitelist participant: %v", err)
	}

	log.Printf("whitelisted twitch_user_id=%s twitch_login=%s status=%s", participant.TwitchUserID, participant.TwitchLogin, participant.Status)
}
