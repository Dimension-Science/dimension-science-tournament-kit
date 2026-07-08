package web

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dimension-science/tournament-kit/internal/config"
	"github.com/dimension-science/tournament-kit/internal/store"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func loadEnv() {
	bytes, err := os.ReadFile("../../.env")
	if err != nil {
		return
	}
	lines := strings.Split(string(bytes), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			os.Setenv(parts[0], parts[1])
		}
	}
	// Override placeholders for test
	os.Setenv("MOD_API_KEY", "test_mod_api_key_must_be_real")
	os.Setenv("MOD_SIGNING_SECRET", "test_mod_signing_secret_must_be_real")
}

func TestModAuthorizeResponse(t *testing.T) {
	loadEnv()

	cfg, err := config.Load()
	if err != nil {
		t.Skipf("Skipping DB test: config load failed: %v", err)
		return
	}
	db, err := store.Open(cfg.DatabaseURL)
	if err != nil {
		t.Skipf("Skipping DB test: DB connection failed: %v", err)
		return
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.EnsureSchema(ctx); err != nil {
		t.Fatalf("failed to ensure schema: %v", err)
	}

	// Insert temporary participant
	p, err := db.UpsertWhitelistParticipant(ctx, "test_auth_user", "testauthlogin", "Test Auth Display")
	if err != nil {
		t.Fatalf("failed to create participant: %v", err)
	}

	// Set twitch login identity
	_, err = db.UpsertParticipantLoginIdentity(ctx, store.LoginIdentity{
		TwitchUserID:      "test_auth_user",
		TwitchLogin:       "testauthlogin",
		TwitchDisplayName: "Test Auth Display",
	}, "active")
	if err != nil {
		t.Fatalf("failed to update login identity: %v", err)
	}

	// Set minecraft nick
	_, err = db.UpdateMinecraftNick(ctx, "test_auth_user", "MinecraftTestNick")
	if err != nil {
		t.Fatalf("failed to update minecraft nick: %v", err)
	}

	rawToken := "msrm_testtoken1234567890abcdef"
	token, err := db.RotateModAuthToken(ctx, p.ID, rawToken)
	if err != nil {
		t.Fatalf("failed to rotate token: %v", err)
	}

	s := &Server{
		store:  db,
		cfg:    cfg,
		twitch: nil,
	}

	payload := map[string]string{
		"modToken": rawToken,
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest("POST", "/api/mod/authorize", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleModAuthorize(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	t.Logf("Response JSON: %s", w.Body.String())

	// Verify twitchLogin and twitchUserId at top-level
	if resp["twitchLogin"] != "testauthlogin" {
		t.Errorf("expected top-level twitchLogin to be 'testauthlogin', got '%v'", resp["twitchLogin"])
	}
	if resp["twitchUserId"] != "test_auth_user" {
		t.Errorf("expected top-level twitchUserId to be 'test_auth_user', got '%v'", resp["twitchUserId"])
	}
	if resp["twitchDisplayName"] != "Test Auth Display" {
		t.Errorf("expected top-level twitchDisplayName to be 'Test Auth Display', got '%v'", resp["twitchDisplayName"])
	}
	if resp["minecraftNick"] != "MinecraftTestNick" {
		t.Errorf("expected top-level minecraftNick to be 'MinecraftTestNick', got '%v'", resp["minecraftNick"])
	}

	// Verify participant object
	part, ok := resp["participant"].(map[string]any)
	if !ok {
		t.Fatalf("missing or invalid 'participant' object")
	}
	if part["twitchLogin"] != "testauthlogin" {
		t.Errorf("expected participant.twitchLogin to be 'testauthlogin', got '%v'", part["twitchLogin"])
	}
	if part["twitchUserId"] != "test_auth_user" {
		t.Errorf("expected participant.twitchUserId to be 'test_auth_user', got '%v'", part["twitchUserId"])
	}
	if part["twitchDisplayName"] != "Test Auth Display" {
		t.Errorf("expected participant.twitchDisplayName to be 'Test Auth Display', got '%v'", part["twitchDisplayName"])
	}
	if part["minecraftNick"] != "MinecraftTestNick" {
		t.Errorf("expected participant.minecraftNick to be 'MinecraftTestNick', got '%v'", part["minecraftNick"])
	}

	// Clean up using direct SQL connection
	rawDB, err := sql.Open("pgx", cfg.DatabaseURL)
	if err == nil {
		defer rawDB.Close()
		_, _ = rawDB.ExecContext(ctx, "DELETE FROM mod_auth_tokens WHERE id = $1", token.ID)
		_, _ = rawDB.ExecContext(ctx, "DELETE FROM participants WHERE id = $1", p.ID)
	}
}
