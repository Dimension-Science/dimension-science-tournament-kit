package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dimension-science/tournament-kit/internal/config"
	"github.com/dimension-science/tournament-kit/internal/store"
)

func TestPublicTournamentMatchesDoNotExposePrivateFields(t *testing.T) {
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	playerSeed := 1
	bestTime := 46174

	payload := publicTournamentMatches([]store.TournamentMatch{
		{
			ID:                  "match-public-id",
			TournamentID:        "tournament-private-id",
			Round:               "quarterfinal",
			Position:            1,
			Player1:             privateParticipantFixture(),
			Player1Seed:         &playerSeed,
			StartsAt:            now,
			EndsAt:              now.Add(24 * time.Hour),
			Status:              "scheduled",
			WinnerParticipantID: "participant-private-id",
			BestOf:              1,
			WorldSeed:           "123456789",
			Player1ConnectedAt:  &now,
			OfficialStartedAt:   &now,
			Player1BestTimeMS:   &bestTime,
		},
	})

	assertJSONOmits(t, payload,
		"participant-private-id",
		"twitch-user-private-id",
		"tournament-private-id",
		"winnerParticipantId",
		"worldSeed",
		"123456789",
		"player1ConnectedAt",
		"officialStartedAt",
		"nowPlayingAuthorized",
		"lastLoginAt",
	)
	assertJSONContains(t, payload, "runner_login", "Runner Display")
}

func TestPublicLeaderboardEntriesDoNotExposeInternalIDs(t *testing.T) {
	finishedAt := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	payload := publicLeaderboardEntries([]store.LeaderboardEntry{
		{
			ParticipantID:     "participant-private-id",
			Rank:              1,
			TwitchLogin:       "runner_login",
			TwitchDisplayName: "Runner Display",
			BestTimeMS:        46174,
			BestRunID:         "run-private-id",
			BestRunFinishedAt: &finishedAt,
		},
	})

	assertJSONOmits(t, payload, "participant-private-id", "participantId", "run-private-id", "bestRunId")
	assertJSONContains(t, payload, "runner_login", "bestRunFinishedAt")
}

func TestPublicAchievementFeedItemsDoNotExposeParticipantID(t *testing.T) {
	unlockedAt := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	payload := publicAchievementFeedItems([]store.AchievementFeedItem{
		{
			ID:              "participant-private-id:first_dragon:2026-05-20T12:00:00Z",
			ParticipantID:   "participant-private-id",
			TwitchLogin:     "runner_login",
			DisplayName:     "Runner Display",
			AchievementSlug: "first_dragon",
			AchievementName: "РџРµСЂРІРѕРїСЂРѕС…РѕРґРµС†",
			UnlockedAt:      unlockedAt,
		},
	})

	assertJSONOmits(t, payload, "participant-private-id", "participantId")
	assertJSONContains(t, payload, "runner_login", "first_dragon")
}

func TestDashboardRunSubmissionIsDisabled(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/runs", strings.NewReader(`{}`))

	(&Server{}).handleRunsAPI(recorder, request)

	if recorder.Code != http.StatusGone {
		t.Fatalf("expected dashboard run endpoint to be gone, got %d", recorder.Code)
	}
	assertJSONOmits(t, json.RawMessage(recorder.Body.Bytes()), "participantId", "run-private-id")
}

func TestModJSONAllowsUnsignedForBackwardCompatibility(t *testing.T) {
	server := &Server{cfg: config.Config{
		ModAPIKey:        "test-key",
		ModSigningSecret: "test-secret",
		ModMaxSkew:       2 * time.Minute,
	}}
	request := httptest.NewRequest(http.MethodPost, "/api/mod/runs", strings.NewReader(`{"modToken":"msrmod_test"}`))

	var payload struct {
		ModToken string `json:"modToken"`
	}
	err := server.decodeAndVerifyModJSON(request, &payload)
	if err != nil {
		t.Fatalf("expected unsigned mod request to be allowed, got error: %v", err)
	}
}

func TestModJSONRejectsInvalidSignature(t *testing.T) {
	server := &Server{cfg: config.Config{
		ModAPIKey:        "test-key",
		ModSigningSecret: "test-secret",
		ModMaxSkew:       2 * time.Minute,
	}}
	request := httptest.NewRequest(http.MethodPost, "/api/mod/runs", strings.NewReader(`{"modToken":"msrmod_test"}`))
	request.Header.Set("X-Mod-Key", "test-key")
	request.Header.Set("X-Mod-Timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	request.Header.Set("X-Mod-Nonce", "1234567890abcdef")
	request.Header.Set("X-Mod-Signature", "bad-signature")

	var payload struct {
		ModToken string `json:"modToken"`
	}
	err := server.decodeAndVerifyModJSON(request, &payload)
	if err == nil {
		t.Fatal("expected invalid signature to be rejected")
	}
	if got := errorStatus(err); got != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid signature, got %d", got)
	}
	if err.Error() != "invalid signature" {
		t.Fatalf("expected invalid signature error, got %q", err.Error())
	}
}

func privateParticipantFixture() *store.Participant {
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	bestTime := 46174
	return &store.Participant{
		ID:                     "participant-private-id",
		TwitchUserID:           "twitch-user-private-id",
		TwitchLogin:            "runner_login",
		TwitchDisplayName:      "Runner Display",
		TwitchProfileImageURL:  "https://static-cdn.jtvnw.net/avatar.png",
		MinecraftNick:          "RunnerMC",
		AvatarURL:              "/uploads/avatar.png",
		BestTimeMS:             &bestTime,
		Status:                 "active",
		StreamOnline:           true,
		ShowInNowPlaying:       true,
		NowPlayingAuthorized:   true,
		StreamTitle:            "private stream title",
		StreamGameName:         "Minecraft",
		LastStreamStatusSyncAt: &now,
		LastLoginAt:            &now,
	}
}

func assertJSONOmits(t *testing.T, value any, forbidden ...string) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, needle := range forbidden {
		if strings.Contains(text, needle) {
			t.Fatalf("public payload leaked %q in %s", needle, text)
		}
	}
}

func assertJSONContains(t *testing.T, value any, expected ...string) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, needle := range expected {
		if !strings.Contains(text, needle) {
			t.Fatalf("public payload did not contain %q in %s", needle, text)
		}
	}
}
