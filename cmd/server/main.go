package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dimension-science/tournament-kit/internal/config"
	"github.com/dimension-science/tournament-kit/internal/discordbot"
	"github.com/dimension-science/tournament-kit/internal/modrinth"
	"github.com/dimension-science/tournament-kit/internal/store"
	"github.com/dimension-science/tournament-kit/internal/telegrambot"
	"github.com/dimension-science/tournament-kit/internal/web"
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

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := db.EnsureSchema(ctx); err != nil {
		log.Fatalf("ensure schema: %v", err)
	}
	if cfg.AppEnv == "development" && cfg.AllowDevMockAuth {
		participant, err := db.FindWhitelistedParticipant(ctx, "", "")
		if err != nil {
			log.Fatalf("check development demo accounts: %v", err)
		}
		if participant == nil {
			if err := db.SeedDemo(ctx); err != nil {
				log.Fatalf("seed development demo accounts: %v", err)
			}
			log.Print("development demo accounts created")
		}
	}

	discord := discordbot.New(discordbot.Config{
		Token:               cfg.DiscordBotToken,
		GuildID:             cfg.DiscordGuildID,
		NewsChannelID:       cfg.DiscordNewsChannelID,
		ModUpdatesChannelID: cfg.DiscordModUpdatesChannelID,
		SupportChannelID:    cfg.DiscordSupportID,
		ApplicationLogID:    cfg.DiscordApplicationLogID,
		ModeratorRoleID:     cfg.DiscordModeratorRoleID,
		RunnerRoleID:        cfg.DiscordRunnerRoleID,
		ChaosPlayerRoleID:   cfg.DiscordChaosPlayerRoleID,
	}, log.Default())

	server, err := web.NewServer(cfg, db, discord)
	if err != nil {
		log.Fatalf("new server: %v", err)
	}

	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()
	db.StartExpiredNonceCleaner(appCtx)
	discord.StartRoleSync(appCtx, db)
	telegrambot.Start(appCtx, cfg.TelegramBotToken, cfg.TelegramAdminChatID, cfg.TelegramNewsChat, cfg.BaseURL, cfg.DiscordURL, cfg.UploadDir, db, discord, log.Default())

	modrinthPoller := modrinth.NewPoller(db, discord, log.Default())
	modrinthPoller.Start(appCtx)

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("server listening on %s", cfg.BaseURL)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	appCancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}
