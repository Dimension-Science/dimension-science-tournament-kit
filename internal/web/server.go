package web

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"image/png"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dimension-science/tournament-kit/internal/config"
	"github.com/dimension-science/tournament-kit/internal/discordbot"
	"github.com/dimension-science/tournament-kit/internal/session"
	"github.com/dimension-science/tournament-kit/internal/store"
	"github.com/dimension-science/tournament-kit/internal/telegrambot"
	"github.com/dimension-science/tournament-kit/internal/twitch"
)

type Server struct {
	cfg           config.Config
	store         *store.Store
	sessions      *session.Signer
	twitch        *twitch.Client
	discord       *discordbot.Client
	templates     *template.Template
	uploadDir     string
	stateCookie   string
	authLimiter   *rateLimiter
	modLimiter    *rateLimiter
	uploadLimiter *rateLimiter
}

func NewServer(cfg config.Config, db *store.Store, discord *discordbot.Client) (*Server, error) {
	templates, err := template.New("").
		Funcs(template.FuncMap{
			"formatDuration":      formatDuration,
			"formatDateTime":      formatDateTime,
			"formatShortDateTime": formatShortDateTime,
			"add":                 func(a, b int) int { return a + b },
		}).
		ParseFS(assets, "templates/*.html")
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(cfg.UploadDir, 0o755); err != nil {
		return nil, err
	}

	return &Server{
		cfg:           cfg,
		store:         db,
		sessions:      session.New(cfg.SessionSecret, cfg.SessionTTL),
		twitch:        twitch.NewClient(cfg),
		discord:       discord,
		templates:     templates,
		uploadDir:     cfg.UploadDir,
		stateCookie:   cfg.AuthCookieName + "_oauth_state",
		authLimiter:   newRateLimiter(12, 10*time.Minute),
		modLimiter:    newRateLimiter(300, time.Minute),
		uploadLimiter: newRateLimiter(20, 10*time.Minute),
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	staticFS, _ := fs.Sub(assets, "static")
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))
	mux.Handle("GET /uploads/", http.StripPrefix("/uploads/", http.FileServer(noListFileSystem{root: http.Dir(s.uploadDir)})))

	mux.HandleFunc("GET /", s.handleIndexPage)
	mux.HandleFunc("GET /about", s.handleAboutPage)
	mux.HandleFunc("GET /about.html", func(w http.ResponseWriter, r *http.Request) {
		redirectPreserveQuery(w, r, "/about")
	})
	mux.HandleFunc("GET /guide", s.handleGuidePage)
	mux.HandleFunc("GET /guide.html", func(w http.ResponseWriter, r *http.Request) {
		redirectPreserveQuery(w, r, "/guide")
	})
	mux.HandleFunc("GET /faq", s.handleFAQPage)
	mux.HandleFunc("GET /faq.html", func(w http.ResponseWriter, r *http.Request) {
		redirectPreserveQuery(w, r, "/faq")
	})
	mux.HandleFunc("GET /streamers", s.handleStreamersPage)
	mux.HandleFunc("GET /streamers.html", func(w http.ResponseWriter, r *http.Request) {
		redirectPreserveQuery(w, r, "/streamers")
	})
	mux.HandleFunc("GET /news", s.handleNewsPage)
	mux.HandleFunc("GET /news.html", func(w http.ResponseWriter, r *http.Request) {
		redirectPreserveQuery(w, r, "/news")
	})
	mux.HandleFunc("GET /how-to-participate", s.handleHowToParticipatePage)
	mux.HandleFunc("GET /how-to-participate.html", func(w http.ResponseWriter, r *http.Request) {
		redirectPreserveQuery(w, r, "/how-to-participate")
	})
	mux.HandleFunc("GET /apply", s.handleApplyPage)
	mux.HandleFunc("GET /apply.html", func(w http.ResponseWriter, r *http.Request) {
		redirectPreserveQuery(w, r, "/apply")
	})
	mux.HandleFunc("GET /bracket", s.handleBracketPage)
	mux.HandleFunc("GET /bracket.html", func(w http.ResponseWriter, r *http.Request) {
		redirectPreserveQuery(w, r, "/bracket")
	})
	mux.HandleFunc("GET /match-control", s.handleMatchControlPage)
	mux.HandleFunc("GET /match-control.html", func(w http.ResponseWriter, r *http.Request) {
		redirectPreserveQuery(w, r, "/match-control")
	})
	mux.HandleFunc("GET /admin", s.handleAdminPage)
	mux.HandleFunc("GET /admin.html", func(w http.ResponseWriter, r *http.Request) {
		redirectPreserveQuery(w, r, "/admin")
	})
	mux.HandleFunc("GET /admin/runs", s.handleAdminRunsPage)
	mux.HandleFunc("GET /admin/runs/{id}", s.handleAdminParticipantRunsPage)
	mux.HandleFunc("GET /login", s.handleLoginPage)
	mux.HandleFunc("GET /login.html", func(w http.ResponseWriter, r *http.Request) {
		redirectPreserveQuery(w, r, "/login")
	})
	mux.HandleFunc("GET /cabinet", s.handleCabinetPage)
	mux.HandleFunc("GET /cabinet/certificate", s.handleDownloadCertificate)
	mux.HandleFunc("GET /cabinet.html", func(w http.ResponseWriter, r *http.Request) {
		redirectPreserveQuery(w, r, "/cabinet")
	})
	mux.HandleFunc("GET /notifications", s.handleNotificationsPage)
	mux.HandleFunc("GET /pickems", s.handlePickemsPage)
	mux.HandleFunc("GET /pickems.html", func(w http.ResponseWriter, r *http.Request) {
		redirectPreserveQuery(w, r, "/pickems")
	})
	mux.HandleFunc("POST /api/predictions", s.handleSavePredictionAPI)

	mux.HandleFunc("POST /auth/twitch/mock", s.handleMockLoginForm)
	mux.HandleFunc("POST /auth/logout", s.handleLogoutForm)
	mux.HandleFunc("POST /apply", s.withRateLimit(s.authLimiter, s.handleApplyForm))
	mux.HandleFunc("POST /cabinet/profile", s.handleCabinetProfileForm)
	mux.HandleFunc("POST /cabinet/avatar", s.handleCabinetAvatarForm)
	mux.HandleFunc("POST /cabinet/mod-token", s.handleCabinetModTokenForm)
	mux.HandleFunc("POST /cabinet/now-playing", s.handleCabinetNowPlayingForm)

	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/public/config", s.handlePublicConfig)
	mux.HandleFunc("GET /api/tournament/current", s.handleCurrentTournamentAPI)
	mux.HandleFunc("GET /api/tournament/bracket", s.handleTournamentBracketAPI)
	mux.HandleFunc("GET /api/achievements/feed", s.handleAchievementFeedAPI)
	mux.HandleFunc("GET /api/leaderboard", s.handleLeaderboardAPI)
	mux.HandleFunc("GET /api/me", s.handleMeAPI)
	mux.HandleFunc("GET /api/me/mod-token", s.handleModTokenStatusAPI)
	mux.HandleFunc("POST /api/runs", s.handleRunsAPI)

	mux.HandleFunc("GET /api/auth/twitch/start", s.withRateLimit(s.authLimiter, s.handleTwitchStart))
	mux.HandleFunc("GET /api/auth/twitch/callback", s.withRateLimit(s.authLimiter, s.handleTwitchCallback))
	mux.HandleFunc("POST /api/auth/twitch/mock", s.withRateLimit(s.authLimiter, s.handleMockLoginAPI))
	mux.HandleFunc("POST /api/auth/logout", s.handleLogoutAPI)

	mux.HandleFunc("PATCH /api/me/profile", s.handleProfileAPI)
	mux.HandleFunc("POST /api/me/avatar", s.withRateLimit(s.uploadLimiter, s.handleAvatarAPI))
	mux.HandleFunc("POST /api/me/mod-token", s.handleModTokenAPI)

	mux.HandleFunc("POST /api/mod/authorize", s.withRateLimit(s.modLimiter, s.handleModAuthorize))
	mux.HandleFunc("POST /api/mod/run-sessions", s.withRateLimit(s.modLimiter, s.handleModRunSessionCreate))
	mux.HandleFunc("POST /api/mod/run-sessions/validate", s.withRateLimit(s.modLimiter, s.handleModRunSessionValidate))
	mux.HandleFunc("POST /api/mod/matches/ready", s.withRateLimit(s.modLimiter, s.handleModMatchReady))
	mux.HandleFunc("POST /api/mod/runs", s.withRateLimit(s.modLimiter, s.handleModRuns))

	mux.HandleFunc("GET /api/admin/participants", s.handleAdminParticipants)
	mux.HandleFunc("GET /api/admin/applications", s.handleAdminApplications)
	mux.HandleFunc("POST /api/admin/applications/{id}/approve", s.handleAdminApplicationApprove)
	mux.HandleFunc("POST /api/admin/applications/{id}/reject", s.handleAdminApplicationReject)
	mux.HandleFunc("GET /api/admin/discord-applications", s.handleAdminDiscordApplications)
	mux.HandleFunc("POST /api/admin/discord-applications/{id}/approve", s.handleAdminDiscordApplicationApprove)
	mux.HandleFunc("POST /api/admin/discord-applications/{id}/reject", s.handleAdminDiscordApplicationReject)
	mux.HandleFunc("POST /api/admin/discord/sync-roles", s.handleAdminDiscordSyncRoles)
	mux.HandleFunc("POST /api/admin/news/sync", s.handleAdminNewsSync)
	mux.HandleFunc("POST /api/admin/whitelist", s.handleAdminWhitelist)
	mux.HandleFunc("PATCH /api/admin/participants/{twitchUserId}/status", s.handleAdminStatus)
	mux.HandleFunc("POST /api/admin/tournament", s.handleAdminTournament)
	mux.HandleFunc("PATCH /api/admin/tournament/{id}", s.handleAdminTournamentUpdate)
	mux.HandleFunc("POST /api/admin/tournament/{id}/bracket", s.handleAdminTournamentBracket)
	mux.HandleFunc("GET /api/admin/tournament/{id}/matches/control", s.handleAdminTournamentMatchesControl)
	mux.HandleFunc("PATCH /api/admin/tournament/{id}/matches", s.handleAdminTournamentMatchesUpdate)
	mux.HandleFunc("POST /api/admin/tournament/{id}/matches/{matchId}/start", s.handleAdminTournamentMatchStart)
	mux.HandleFunc("POST /api/admin/tournament/{id}/matches/{matchId}/reset", s.handleAdminTournamentMatchReset)
	mux.HandleFunc("DELETE /api/admin/tournament/running", s.handleAdminTournamentStop)
	mux.HandleFunc("DELETE /api/admin/leaderboard", s.handleAdminLeaderboardClear)
	mux.HandleFunc("DELETE /api/admin/test-runs", s.handleAdminTestRunsClear)
	mux.HandleFunc("GET /api/admin/test-mode", s.handleAdminTestMode)
	mux.HandleFunc("GET /api/admin/test-certificate", s.handleAdminTestCertificate)
	mux.HandleFunc("POST /api/admin/test-mode", s.handleAdminTestModeEnable)
	mux.HandleFunc("DELETE /api/admin/test-mode", s.handleAdminTestModeDisable)
	mux.HandleFunc("GET /api/admin/applications-status", s.handleAdminApplicationsStatusGet)
	mux.HandleFunc("POST /api/admin/applications-status", s.handleAdminApplicationsStatusSet)
	mux.HandleFunc("GET /api/admin/push-notification", s.handleAdminPushNotificationGet)
	mux.HandleFunc("POST /api/admin/push-notification", s.handleAdminPushNotificationSet)
	mux.HandleFunc("DELETE /api/admin/push-notification", s.handleAdminPushNotificationClear)
	mux.HandleFunc("POST /api/admin/runs/{id}/reject", s.handleAdminRunReject)
	mux.HandleFunc("POST /api/admin/runs/{id}/approve", s.handleAdminRunApprove)
	mux.HandleFunc("POST /api/admin/participants/{id}/runs/add", s.handleAdminRunAdd)

	return s.withMiddleware(mux)
}

func redirectPreserveQuery(w http.ResponseWriter, r *http.Request, path string) {
	if r.URL.RawQuery != "" {
		path += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, path, http.StatusMovedPermanently)
}

type noListFileSystem struct {
	root http.FileSystem
}

func (n noListFileSystem) Open(name string) (http.File, error) {
	file, err := n.root.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if info.IsDir() {
		_ = file.Close()
		return nil, fs.ErrNotExist
	}
	return file, nil
}

func (s *Server) handleIndexPage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	leaderboard, err := s.store.GetLeaderboard(ctx)
	if err != nil {
		http.Error(w, "failed to load leaderboard", http.StatusInternalServerError)
		return
	}
	tournament, err := s.store.GetCurrentTournament(ctx)
	if err != nil {
		http.Error(w, "failed to load tournament", http.StatusInternalServerError)
		return
	}
	tournament = s.ensureBracketAfterQualification(ctx, tournament)
	nowPlaying, err := s.loadNowPlaying(ctx)
	if err != nil {
		http.Error(w, "failed to load current runners", http.StatusInternalServerError)
		return
	}

	var matches []store.TournamentMatch
	var rounds []bracketRoundView
	if tournament != nil {
		matches, err = s.store.ListTournamentMatches(ctx, tournament.ID)
		if err == nil {
			rounds = buildBracketRounds(matches)
		}
	}

	_ = s.renderTemplate(w, "index.html", map[string]any{
		"TournamentTitle": s.cfg.TournamentTitle,
		"Tournament":      buildTournamentStatusView(tournament),
		"Leaderboard":     buildLeaderboardRows(leaderboard, playoffCutoff(tournament)),
		"NowPlaying":      buildNowPlayingRows(nowPlaying),
		"AccountCard":     s.homeAccountCard(ctx, r),
		"Rounds":          rounds,
	})
}

func (s *Server) handleBracketPage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	tournament, err := s.store.GetCurrentTournament(ctx)
	if err != nil {
		http.Error(w, "failed to load tournament", http.StatusInternalServerError)
		return
	}
	tournament = s.ensureBracketAfterQualification(ctx, tournament)
	var matches []store.TournamentMatch
	if tournament != nil {
		matches, err = s.store.ListTournamentMatches(ctx, tournament.ID)
		if err != nil {
			http.Error(w, "failed to load bracket", http.StatusInternalServerError)
			return
		}
	}
	_ = s.renderTemplate(w, "bracket.html", map[string]any{
		"TournamentTitle": s.cfg.TournamentTitle,
		"Tournament":      buildTournamentStatusView(tournament),
		"Rounds":          buildBracketRounds(matches),
	})
}

func (s *Server) handleMatchControlPage(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.currentClaims(r)
	if !ok {
		http.Redirect(w, r, "/login?error=login_required", http.StatusSeeOther)
		return
	}
	if claims.Role != "admin" {
		http.Error(w, "admin role required", http.StatusForbidden)
		return
	}

	_ = s.renderTemplate(w, "match-control.html", map[string]any{
		"TournamentTitle": s.cfg.TournamentTitle,
	})
}

func (s *Server) handleAdminPage(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.currentClaims(r)
	if !ok {
		http.Redirect(w, r, "/login?error=login_required", http.StatusSeeOther)
		return
	}
	if claims.Role != "admin" {
		http.Error(w, "admin role required", http.StatusForbidden)
		return
	}

	_ = s.renderTemplate(w, "admin.html", map[string]any{
		"TournamentTitle": s.cfg.TournamentTitle,
	})
}

func (s *Server) handleAboutPage(w http.ResponseWriter, r *http.Request) {
	_ = s.renderTemplate(w, "about.html", map[string]any{
		"TournamentTitle": s.cfg.TournamentTitle,
		"AboutEnglish":    strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("lang")), "en"),
	})
}

func (s *Server) handleGuidePage(w http.ResponseWriter, r *http.Request) {
	_ = s.renderTemplate(w, "guide.html", map[string]any{
		"TournamentTitle": s.cfg.TournamentTitle,
	})
}

func (s *Server) handleFAQPage(w http.ResponseWriter, r *http.Request) {
	_ = s.renderTemplate(w, "faq.html", map[string]any{
		"TournamentTitle": s.cfg.TournamentTitle,
	})
}

func (s *Server) handleStreamersPage(w http.ResponseWriter, r *http.Request) {
	_ = s.renderTemplate(w, "streamers.html", map[string]any{
		"TournamentTitle": s.cfg.TournamentTitle,
		"BaseURL":         s.cfg.BaseURL,
	})
}

func (s *Server) handleHowToParticipatePage(w http.ResponseWriter, r *http.Request) {
	_ = s.renderTemplate(w, "how-to-participate.html", map[string]any{
		"TournamentTitle": s.cfg.TournamentTitle,
	})
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	if claims, ok := s.currentClaims(r); ok {
		if claims.Role == "applicant" {
			http.Redirect(w, r, "/apply", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/cabinet", http.StatusSeeOther)
		return
	}

	_ = s.renderTemplate(w, "login.html", map[string]any{
		"TournamentTitle":  s.cfg.TournamentTitle,
		"AllowDevMockAuth": s.cfg.AllowDevMockAuth,
		"Message":          flashMessage(r.URL.Query().Get("ok")),
		"Error":            flashMessage(r.URL.Query().Get("error")),
	})
}

func (s *Server) handleCabinetPage(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.currentClaims(r)
	if !ok {
		http.Redirect(w, r, "/login?error=login_required", http.StatusSeeOther)
		return
	}

	if claims.Role == "applicant" {
		participant, err := s.store.FindParticipantByTwitchUserID(r.Context(), claims.TwitchUserID)
		if err != nil || !participantCanAccessCabinet(participant) {
			http.Redirect(w, r, "/apply", http.StatusSeeOther)
			return
		}
		claims.Role = "streamer"
		s.setSessionCookie(w, claims.TwitchUserID, claims.TwitchLogin, claims.Role)
	}

	pageData, status, err := s.loadCabinetData(r.Context(), claims)
	if err != nil {
		if errors.Is(err, errNotWhitelisted) {
			s.clearSessionCookie(w)
			http.Redirect(w, r, "/apply", http.StatusSeeOther)
			return
		}
		http.Error(w, "failed to load cabinet", status)
		return
	}

	pageData["Message"] = flashMessage(r.URL.Query().Get("ok"))
	pageData["Error"] = flashMessage(r.URL.Query().Get("error"))
	pageData["OKCode"] = r.URL.Query().Get("ok")
	newModToken := s.consumeFlashValue(w, r, s.cfg.AuthCookieName+"_mod_token_flash")
	if newModToken == "" && claims.Role == "streamer" {
		if hasToken, _ := pageData["HasModToken"].(bool); !hasToken {
			if generatedToken, tokenErr := s.rotateModToken(r.Context(), claims.TwitchUserID); tokenErr == nil {
				newModToken = generatedToken
				pageData["HasModToken"] = true
			}
		}
	}
	pageData["NewModToken"] = newModToken
	_ = s.renderTemplate(w, "cabinet.html", pageData)
}

func (s *Server) handleMockLoginForm(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.AllowDevMockAuth {
		http.Redirect(w, r, "/login?error=mock_disabled", http.StatusSeeOther)
		return
	}
	_ = r.ParseForm()
	s.handleMockLogin(w, r, strings.TrimSpace(r.FormValue("twitch_user_id")), strings.ToLower(strings.TrimSpace(r.FormValue("twitch_login"))), false)
}

func (s *Server) handleLogoutForm(w http.ResponseWriter, r *http.Request) {
	s.clearSessionCookie(w)
	http.Redirect(w, r, "/login?ok=logged_out", http.StatusSeeOther)
}

func (s *Server) handleCabinetProfileForm(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.currentClaims(r)
	if !ok || (claims.Role != "streamer" && claims.Role != "admin") {
		http.Redirect(w, r, "/login?error=login_required", http.StatusSeeOther)
		return
	}
	if s.redirectCabinetAccessError(w, r, s.ensureCabinetAccess(r.Context(), claims), "profile_update_failed") {
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/cabinet?error=invalid_form", http.StatusSeeOther)
		return
	}
	nick := strings.TrimSpace(r.FormValue("minecraft_nick"))
	if !validMinecraftNick(nick) {
		http.Redirect(w, r, "/cabinet?error=invalid_minecraft_nick", http.StatusSeeOther)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if _, err := s.store.UpdateMinecraftNick(ctx, claims.TwitchUserID, nick); err != nil {
		http.Redirect(w, r, "/cabinet?error=profile_update_failed", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/cabinet?ok=profile_saved", http.StatusSeeOther)
}

func (s *Server) handleCabinetAvatarForm(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.currentClaims(r)
	if !ok || (claims.Role != "streamer" && claims.Role != "admin") {
		http.Redirect(w, r, "/login?error=login_required", http.StatusSeeOther)
		return
	}
	if s.redirectCabinetAccessError(w, r, s.ensureCabinetAccess(r.Context(), claims), "profile_update_failed") {
		return
	}
	if !s.uploadLimiter.Allow(clientIP(r)) {
		http.Redirect(w, r, "/cabinet?error=too_many_requests", http.StatusSeeOther)
		return
	}

	if _, err := s.updateAvatar(r.Context(), claims.TwitchUserID, r); err != nil {
		http.Redirect(w, r, "/cabinet?error="+flashCode(err), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/cabinet?ok=avatar_saved", http.StatusSeeOther)
}

func (s *Server) handleCabinetModTokenForm(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.currentClaims(r)
	if !ok || (claims.Role != "streamer" && claims.Role != "admin") {
		http.Redirect(w, r, "/login?error=login_required", http.StatusSeeOther)
		return
	}
	if s.redirectCabinetAccessError(w, r, s.ensureCabinetAccess(r.Context(), claims), "profile_update_failed") {
		return
	}

	rawToken, err := s.rotateModToken(r.Context(), claims.TwitchUserID)
	if err != nil {
		http.Redirect(w, r, "/cabinet?error=profile_update_failed", http.StatusSeeOther)
		return
	}

	s.setFlashValue(w, s.cfg.AuthCookieName+"_mod_token_flash", rawToken, 180)
	http.Redirect(w, r, "/cabinet?ok=mod_token_rotated", http.StatusSeeOther)
}

func (s *Server) handleCabinetNowPlayingForm(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.currentClaims(r)
	if !ok || (claims.Role != "streamer" && claims.Role != "admin") {
		http.Redirect(w, r, "/login?error=login_required", http.StatusSeeOther)
		return
	}
	if s.redirectCabinetAccessError(w, r, s.ensureCabinetAccess(r.Context(), claims), "profile_update_failed") {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/cabinet?error=invalid_form", http.StatusSeeOther)
		return
	}

	show := r.FormValue("show_in_now_playing") == "1"
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.store.UpdateNowPlayingSettings(ctx, claims.TwitchUserID, show); err != nil {
		http.Redirect(w, r, "/cabinet?error=profile_update_failed", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/cabinet?ok=now_playing_saved", http.StatusSeeOther)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "at": time.Now().UTC()})
}

func (s *Server) handlePublicConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"allowDevMockAuth":  s.cfg.AllowDevMockAuth,
		"twitchAuthEnabled": s.cfg.TwitchEnabled(),
		"tournamentTitle":   s.cfg.TournamentTitle,
		"profile":           s.cfg.PublicProfile(),
	})
}

func (s *Server) handleAchievementFeedAPI(w http.ResponseWriter, r *http.Request) {
	var since *time.Time
	if rawSince := strings.TrimSpace(r.URL.Query().Get("since")); rawSince != "" {
		parsed, err := time.Parse(time.RFC3339Nano, rawSince)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid since")
			return
		}
		since = &parsed
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	items, err := s.store.ListRecentAchievementFeed(ctx, since, 12)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load achievement feed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":      publicAchievementFeedItems(items),
		"serverTime": time.Now().UTC(),
	})
}

func (s *Server) handleCurrentTournamentAPI(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tournament, err := s.store.GetCurrentTournament(ctx)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load tournament")
		return
	}
	tournament = s.ensureBracketAfterQualification(ctx, tournament)
	running, err := s.store.IsTournamentRunning(ctx)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load tournament status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tournament": tournament, "isRunning": running})
}

func (s *Server) handleTournamentBracketAPI(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tournament, err := s.store.GetCurrentTournament(ctx)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load tournament")
		return
	}
	tournament = s.ensureBracketAfterQualification(ctx, tournament)
	if tournament == nil {
		writeJSON(w, http.StatusOK, map[string]any{"tournament": nil, "matches": []publicTournamentMatch{}})
		return
	}
	matches, err := s.store.ListTournamentMatches(ctx, tournament.ID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load bracket")
		return
	}
	if matches == nil {
		matches = []store.TournamentMatch{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tournament": publicTournament(tournament), "matches": publicTournamentMatches(matches)})
}

type publicTournamentView struct {
	StartsAt            time.Time  `json:"startsAt"`
	QualificationEndsAt time.Time  `json:"qualificationEndsAt"`
	PlayoffEndsAt       time.Time  `json:"playoffEndsAt"`
	EndsAt              time.Time  `json:"endsAt"`
	State               string     `json:"state"`
	Phase               string     `json:"phase"`
	PlayoffSlots        int        `json:"playoffSlots"`
	BracketGeneratedAt  *time.Time `json:"bracketGeneratedAt,omitempty"`
}

type publicTournamentMatch struct {
	ID                string                  `json:"id,omitempty"`
	Round             string                  `json:"round"`
	Position          int                     `json:"position"`
	Player1           *publicMatchParticipant `json:"player1,omitempty"`
	Player2           *publicMatchParticipant `json:"player2,omitempty"`
	Player1Seed       *int                    `json:"player1Seed,omitempty"`
	Player2Seed       *int                    `json:"player2Seed,omitempty"`
	StartsAt          time.Time               `json:"startsAt"`
	EndsAt            time.Time               `json:"endsAt"`
	Status            string                  `json:"status"`
	BestOf            int                     `json:"bestOf"`
	Player1BestTimeMS *int                    `json:"player1BestTimeMs,omitempty"`
	Player2BestTimeMS *int                    `json:"player2BestTimeMs,omitempty"`
	WinnerDisplayName string                  `json:"winnerDisplayName,omitempty"`
}

type publicMatchParticipant struct {
	TwitchLogin       string `json:"twitchLogin,omitempty"`
	TwitchDisplayName string `json:"twitchDisplayName,omitempty"`
	MinecraftNick     string `json:"minecraftNick,omitempty"`
	AvatarURL         string `json:"avatarUrl,omitempty"`
}

func publicTournament(tournament *store.Tournament) *publicTournamentView {
	if tournament == nil {
		return nil
	}
	return &publicTournamentView{
		StartsAt:            tournament.StartsAt,
		QualificationEndsAt: tournament.QualificationEndsAt,
		PlayoffEndsAt:       tournament.PlayoffEndsAt,
		EndsAt:              tournament.EndsAt,
		State:               tournament.State,
		Phase:               tournament.Phase,
		PlayoffSlots:        tournament.PlayoffSlots,
		BracketGeneratedAt:  tournament.BracketGeneratedAt,
	}
}

func publicTournamentMatches(matches []store.TournamentMatch) []publicTournamentMatch {
	publicMatches := make([]publicTournamentMatch, 0, len(matches))
	for _, match := range matches {
		publicMatches = append(publicMatches, publicTournamentMatch{
			ID:                match.ID,
			Round:             match.Round,
			Position:          match.Position,
			Player1:           publicMatchParticipantFromStore(match.Player1),
			Player2:           publicMatchParticipantFromStore(match.Player2),
			Player1Seed:       match.Player1Seed,
			Player2Seed:       match.Player2Seed,
			StartsAt:          match.StartsAt,
			EndsAt:            match.EndsAt,
			Status:            match.Status,
			BestOf:            match.BestOf,
			Player1BestTimeMS: match.Player1BestTimeMS,
			Player2BestTimeMS: match.Player2BestTimeMS,
			WinnerDisplayName: match.WinnerDisplayName,
		})
	}
	return publicMatches
}

func publicMatchParticipantFromStore(participant *store.Participant) *publicMatchParticipant {
	if participant == nil {
		return nil
	}
	return &publicMatchParticipant{
		TwitchLogin:       participant.TwitchLogin,
		TwitchDisplayName: participant.TwitchDisplayName,
		MinecraftNick:     participant.MinecraftNick,
		AvatarURL:         participant.AvatarURL,
	}
}

func (s *Server) ensureBracketAfterQualification(ctx context.Context, tournament *store.Tournament) *store.Tournament {
	if tournament == nil || tournament.BracketGeneratedAt != nil || time.Now().Before(tournament.QualificationEndsAt) {
		return tournament
	}
	if _, err := s.store.GeneratePlayoffBracket(ctx, tournament.ID); err != nil {
		return tournament
	}
	refreshed, err := s.store.GetCurrentTournament(ctx)
	if err != nil || refreshed == nil {
		return tournament
	}
	return refreshed
}

func (s *Server) handleLeaderboardAPI(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	leaderboard, err := s.store.GetLeaderboard(ctx)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load leaderboard")
		return
	}
	writeJSON(w, http.StatusOK, publicLeaderboardEntries(leaderboard))
}

type publicLeaderboardEntry struct {
	Rank              int        `json:"rank"`
	TwitchLogin       string     `json:"twitchLogin"`
	TwitchDisplayName string     `json:"twitchDisplayName"`
	MinecraftNick     string     `json:"minecraftNick"`
	AvatarURL         string     `json:"avatarUrl,omitempty"`
	BestTimeMS        int        `json:"bestTimeMs"`
	BestRunFinishedAt *time.Time `json:"bestRunFinishedAt,omitempty"`
	RankDelta         int        `json:"rankDelta,omitempty"`
	NetherSplitMS     *int       `json:"netherSplitMs,omitempty"`
	EndSplitMS        *int       `json:"endSplitMs,omitempty"`
}

func publicLeaderboardEntries(entries []store.LeaderboardEntry) []publicLeaderboardEntry {
	publicEntries := make([]publicLeaderboardEntry, 0, len(entries))
	for _, entry := range entries {
		publicEntries = append(publicEntries, publicLeaderboardEntry{
			Rank:              entry.Rank,
			TwitchLogin:       entry.TwitchLogin,
			TwitchDisplayName: entry.TwitchDisplayName,
			MinecraftNick:     entry.MinecraftNick,
			AvatarURL:         entry.AvatarURL,
			BestTimeMS:        entry.BestTimeMS,
			BestRunFinishedAt: entry.BestRunFinishedAt,
			RankDelta:         entry.RankDelta,
			NetherSplitMS:     entry.NetherSplitMS,
			EndSplitMS:        entry.EndSplitMS,
		})
	}
	return publicEntries
}

type publicAchievementFeedItem struct {
	ID                     string    `json:"id"`
	TwitchLogin            string    `json:"twitchLogin"`
	DisplayName            string    `json:"displayName"`
	AvatarURL              string    `json:"avatarUrl,omitempty"`
	AchievementSlug        string    `json:"achievementSlug"`
	AchievementName        string    `json:"achievementName"`
	AchievementDescription string    `json:"achievementDescription,omitempty"`
	UnlockedAt             time.Time `json:"unlockedAt"`
}

func publicAchievementFeedItems(items []store.AchievementFeedItem) []publicAchievementFeedItem {
	publicItems := make([]publicAchievementFeedItem, 0, len(items))
	for _, item := range items {
		publicItems = append(publicItems, publicAchievementFeedItem{
			ID:                     item.TwitchLogin + ":" + item.AchievementSlug + ":" + item.UnlockedAt.UTC().Format(time.RFC3339Nano),
			TwitchLogin:            item.TwitchLogin,
			DisplayName:            item.DisplayName,
			AvatarURL:              item.AvatarURL,
			AchievementSlug:        item.AchievementSlug,
			AchievementName:        item.AchievementName,
			AchievementDescription: item.AchievementDescription,
			UnlockedAt:             item.UnlockedAt,
		})
	}
	return publicItems
}

func (s *Server) handleMeAPI(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.currentClaims(r)
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	data, status, err := s.loadCabinetData(r.Context(), claims)
	if err != nil {
		if errors.Is(err, errNotWhitelisted) {
			writeAPIError(w, status, "not whitelisted")
			return
		}
		if status == http.StatusNotFound {
			writeAPIError(w, status, "participant not found")
			return
		}
		writeAPIError(w, status, "failed to load profile")
		return
	}
	writeJSON(w, http.StatusOK, data["Profile"])
}

func (s *Server) handleModTokenStatusAPI(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.currentClaims(r)
	if !ok || claims.Role != "streamer" {
		writeAPIError(w, http.StatusUnauthorized, "streamer authentication required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	participant, err := s.store.FindParticipantByTwitchUserID(ctx, claims.TwitchUserID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load participant")
		return
	}
	if participant == nil {
		writeAPIError(w, http.StatusNotFound, "participant not found")
		return
	}
	if !participantCanAccessCabinet(participant) {
		writeAPIError(w, http.StatusForbidden, "not whitelisted")
		return
	}

	token, err := s.store.GetModAuthTokenByParticipantID(ctx, participant.ID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load mod token")
		return
	}

	writeJSON(w, http.StatusOK, s.modTokenPayload(token, ""))
}

func (s *Server) handleRunsAPI(w http.ResponseWriter, r *http.Request) {
	writeAPIError(w, http.StatusGone, "dashboard run submission is disabled; use the official mod")
}

func (s *Server) handleTwitchStart(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.TwitchEnabled() {
		writeAPIError(w, http.StatusServiceUnavailable, "twitch oauth is not configured")
		return
	}
	state := randomState()
	purpose := strings.TrimSpace(r.URL.Query().Get("purpose"))
	scopes := []string{}
	if purpose == "now-playing" {
		scopes = append(scopes, "user:read:broadcast")
		http.SetCookie(w, &http.Cookie{
			Name:     s.stateCookie + "_purpose",
			Value:    purpose,
			Path:     "/",
			HttpOnly: true,
			Secure:   s.cfg.AuthCookieSecure,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   600,
		})
	}
	http.SetCookie(w, &http.Cookie{
		Name:     s.stateCookie,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.AuthCookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})
	http.Redirect(w, r, s.twitch.AuthorizeURLWithRedirectURI(s.twitchRedirectURI(r), state, scopes...), http.StatusFound)
}

func (s *Server) twitchRedirectURI(r *http.Request) string {
	origin := requestOrigin(r)
	if s.oauthOriginAllowed(origin) {
		return origin + "/api/auth/twitch/callback"
	}
	return s.cfg.TwitchRedirectURI
}

func (s *Server) oauthOriginAllowed(origin string) bool {
	if origin == "" {
		return false
	}
	if s.cfg.OriginAllowed(origin) || strings.EqualFold(origin, s.cfg.BaseOrigin) {
		return true
	}
	return strings.EqualFold(origin, "https://ru.example.com")
}

func requestOrigin(r *http.Request) string {
	if r == nil || strings.TrimSpace(r.Host) == "" {
		return ""
	}
	scheme := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])
	if scheme == "" {
		if r.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	return scheme + "://" + strings.TrimSpace(r.Host)
}

func (s *Server) handleTwitchCallback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie(s.stateCookie)
	if err != nil || stateCookie.Value == "" || stateCookie.Value != r.URL.Query().Get("state") {
		http.Redirect(w, r, "/login?error=oauth_state", http.StatusSeeOther)
		return
	}
	purpose := ""
	if purposeCookie, purposeErr := r.Cookie(s.stateCookie + "_purpose"); purposeErr == nil {
		purpose = purposeCookie.Value
	}
	http.SetCookie(w, &http.Cookie{Name: s.stateCookie, Value: "", Path: "/", HttpOnly: true, Secure: s.cfg.AuthCookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: s.stateCookie + "_purpose", Value: "", Path: "/", HttpOnly: true, Secure: s.cfg.AuthCookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: s.stateCookie + "_ref", Value: "", Path: "/", HttpOnly: true, Secure: s.cfg.AuthCookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1})

	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		http.Redirect(w, r, "/login?error=oauth_failed", http.StatusSeeOther)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	token, err := s.twitch.ExchangeCodeWithRedirectURI(ctx, code, s.twitchRedirectURI(r))
	if err != nil {
		http.Redirect(w, r, "/login?error=oauth_failed", http.StatusSeeOther)
		return
	}
	validation, err := s.twitch.ValidateUserToken(ctx, token.AccessToken)
	if err != nil || (s.cfg.TwitchClientID != "" && validation.ClientID != s.cfg.TwitchClientID) {
		http.Redirect(w, r, "/login?error=oauth_failed", http.StatusSeeOther)
		return
	}
	user, err := s.twitch.GetUser(ctx, token.AccessToken, validation.UserID)
	if err != nil {
		http.Redirect(w, r, "/login?error=oauth_failed", http.StatusSeeOther)
		return
	}

	identity := store.LoginIdentity{
		TwitchUserID:          validation.UserID,
		TwitchLogin:           strings.ToLower(user.Login),
		TwitchDisplayName:     user.DisplayName,
		TwitchProfileImageURL: user.ProfileImageURL,
	}

	role, loginErr := s.loginWithIdentity(ctx, identity)
	if loginErr != nil {
		http.Redirect(w, r, "/login?error=not_whitelisted", http.StatusSeeOther)
		return
	}

	s.setSessionCookie(w, validation.UserID, strings.ToLower(user.Login), role)
	if role == "viewer" {
		http.Redirect(w, r, "/pickems", http.StatusSeeOther)
		return
	}
	if purpose == "now-playing" {
		if !hasScope(validation.Scopes, "user:read:broadcast") {
			http.Redirect(w, r, "/cabinet?error=now_playing_permission_missing", http.StatusSeeOther)
			return
		}
		if err := s.store.SetNowPlayingAuthorized(ctx, validation.UserID, true); err != nil {
			http.Redirect(w, r, "/cabinet?error=profile_update_failed", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/cabinet?ok=now_playing_authorized", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/cabinet", http.StatusSeeOther)
}

func (s *Server) handleMockLoginAPI(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.AllowDevMockAuth {
		writeAPIError(w, http.StatusNotFound, "not found")
		return
	}

	var payload struct {
		TwitchUserID string `json:"twitchUserId"`
		TwitchLogin  string `json:"twitchLogin"`
	}
	if err := decodeJSON(r, &payload); err != nil && !errors.Is(err, errEmptyBody) {
		writeAPIError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	s.handleMockLogin(w, r, strings.TrimSpace(payload.TwitchUserID), strings.ToLower(strings.TrimSpace(payload.TwitchLogin)), true)
}

func (s *Server) handleLogoutAPI(w http.ResponseWriter, _ *http.Request) {
	s.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleProfileAPI(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.currentClaims(r)
	if !ok || claims.Role != "streamer" {
		writeAPIError(w, http.StatusUnauthorized, "streamer authentication required")
		return
	}
	if writeCabinetAccessAPIError(w, s.ensureCabinetAccess(r.Context(), claims), "failed to load participant") {
		return
	}

	var payload struct {
		MinecraftNick string `json:"minecraftNick"`
	}
	if err := decodeJSON(r, &payload); err != nil || !validMinecraftNick(strings.TrimSpace(payload.MinecraftNick)) {
		writeAPIError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	participant, err := s.store.UpdateMinecraftNick(ctx, claims.TwitchUserID, strings.TrimSpace(payload.MinecraftNick))
	if err != nil || participant == nil {
		writeAPIError(w, http.StatusNotFound, "participant not found")
		return
	}
	writeJSON(w, http.StatusOK, s.profilePayload(participant, claims, participant.StreamOnline))
}

func (s *Server) handleAvatarAPI(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.currentClaims(r)
	if !ok || claims.Role != "streamer" {
		writeAPIError(w, http.StatusUnauthorized, "streamer authentication required")
		return
	}
	if writeCabinetAccessAPIError(w, s.ensureCabinetAccess(r.Context(), claims), "failed to load participant") {
		return
	}

	avatarURL, err := s.updateAvatar(r.Context(), claims.TwitchUserID, r)
	if err != nil {
		writeAPIError(w, errorStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"avatarUrl": avatarURL})
}

func (s *Server) handleModTokenAPI(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.currentClaims(r)
	if !ok || claims.Role != "streamer" {
		writeAPIError(w, http.StatusUnauthorized, "streamer authentication required")
		return
	}
	if writeCabinetAccessAPIError(w, s.ensureCabinetAccess(r.Context(), claims), "failed to load participant") {
		return
	}

	rawToken, err := s.rotateModToken(r.Context(), claims.TwitchUserID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to rotate mod token")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	participant, err := s.store.FindParticipantByTwitchUserID(ctx, claims.TwitchUserID)
	if err != nil || participant == nil {
		writeAPIError(w, http.StatusNotFound, "participant not found")
		return
	}
	token, err := s.store.GetModAuthTokenByParticipantID(ctx, participant.ID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load mod token")
		return
	}

	writeJSON(w, http.StatusCreated, s.modTokenPayload(token, rawToken))
}

func (s *Server) handleModAuthorize(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		ModToken string `json:"modToken"`
	}
	if err := s.decodeAndVerifyModJSON(r, &payload); err != nil {
		writeAPIError(w, errorStatus(err), err.Error())
		return
	}

	participant, token, streamOnline, reason, err := s.authorizeModParticipant(r.Context(), payload.ModToken, false)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to authorize mod")
		return
	}
	if participant == nil {
		status := http.StatusForbidden
		if reason == "tournament_not_running" || reason == "stream_offline" {
			status = http.StatusConflict
		}
		writeJSON(w, status, map[string]any{"allowed": false, "reason": reason})
		return
	}

	testMode, err := s.store.GetTestModeStatus(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load test mode")
		return
	}
	if testMode.Enabled {
		writeJSON(w, http.StatusOK, map[string]any{
			"allowed":             true,
			"twitchUserId":        participant.TwitchUserID,
			"twitchLogin":         participant.TwitchLogin,
			"twitchDisplayName":   participant.TwitchDisplayName,
			"minecraftNick":       participant.MinecraftNick,
			"phase":               "test",
			"phaseLabel":          "Р СћР ВµРЎРѓРЎвЂљР С•Р Р†РЎвЂ№Р в„– РЎР‚Р ВµР В¶Р С‘Р С",
			"canStartOfficialRun": true,
			"reason":              "",
			"matchId":             "",
			"worldSeed":           "",
			"participant": map[string]any{
				"twitchUserId":      participant.TwitchUserID,
				"twitchLogin":       participant.TwitchLogin,
				"twitchDisplayName": participant.TwitchDisplayName,
				"minecraftNick":     participant.MinecraftNick,
			},
			"streamOnline": streamOnline,
			"tokenPreview": token.TokenPreview,
			"expiresAt":    token.UpdatedAt.Add(modTokenTTL),
			"testMode":     testMode,
			"tournament": map[string]any{
				"phase":               "test",
				"phaseLabel":          "Р СћР ВµРЎРѓРЎвЂљР С•Р Р†РЎвЂ№Р в„– РЎР‚Р ВµР В¶Р С‘Р С",
				"canStartOfficialRun": true,
				"reason":              "",
				"tournamentId":        "",
				"matchId":             "",
				"worldSeed":           "",
			},
		})
		return
	}

	runContext, runReason, err := s.store.OfficialRunContext(r.Context(), participant.ID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load tournament state")
		return
	}
	tournament, err := s.store.GetCurrentTournament(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load tournament")
		return
	}
	phase := phaseFromTournament(tournament)

	writeJSON(w, http.StatusOK, map[string]any{
		"allowed":             true,
		"twitchUserId":        participant.TwitchUserID,
		"twitchLogin":         participant.TwitchLogin,
		"twitchDisplayName":   participant.TwitchDisplayName,
		"minecraftNick":       participant.MinecraftNick,
		"phase":               phase,
		"phaseLabel":          phaseLabel(phase),
		"canStartOfficialRun": runReason == "",
		"reason":              runReason,
		"matchId":             runContext.MatchID,
		"worldSeed":           runContext.WorldSeed,
		"participant": map[string]any{
			"twitchUserId":      participant.TwitchUserID,
			"twitchLogin":       participant.TwitchLogin,
			"twitchDisplayName": participant.TwitchDisplayName,
			"minecraftNick":     participant.MinecraftNick,
		},
		"streamOnline": streamOnline,
		"tokenPreview": token.TokenPreview,
		"expiresAt":    token.UpdatedAt.Add(modTokenTTL),
		"tournament": map[string]any{
			"phase":               phase,
			"phaseLabel":          phaseLabel(phase),
			"canStartOfficialRun": runReason == "",
			"reason":              runReason,
			"tournamentId":        runContext.TournamentID,
			"matchId":             runContext.MatchID,
			"worldSeed":           runContext.WorldSeed,
		},
	})
}

func (s *Server) handleModMatchReady(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		ModToken string `json:"modToken"`
		MatchID  string `json:"matchId"`
	}
	if err := s.decodeAndVerifyModJSON(r, &payload); err != nil {
		writeAPIError(w, errorStatus(err), err.Error())
		return
	}

	participant, _, _, reason, err := s.authorizeModParticipant(r.Context(), payload.ModToken, true)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to authorize mod")
		return
	}
	if participant == nil {
		status := http.StatusForbidden
		if reason == "tournament_not_running" || reason == "stream_offline" || reason == "stream_not_minecraft" {
			status = http.StatusConflict
		}
		writeJSON(w, status, map[string]any{"allowed": false, "reason": reason})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	runContext, runReason, err := s.store.OfficialRunContext(ctx, participant.ID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load tournament state")
		return
	}
	if runContext.MatchID == "" {
		if runReason == "" {
			runReason = "match_window_inactive"
		}
		writeAPIError(w, http.StatusConflict, runReason)
		return
	}
	if payload.MatchID != "" && payload.MatchID != runContext.MatchID {
		writeAPIError(w, http.StatusConflict, "match window is no longer active")
		return
	}

	match, err := s.store.MarkActiveMatchParticipantConnected(ctx, runContext.MatchID, participant.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeAPIError(w, http.StatusConflict, "match_window_inactive")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to mark match ready")
		return
	}
	canStart := match.OfficialStartedAt != nil
	matchReason := ""
	if !canStart {
		matchReason = "match_not_started"
	}
	phase := "playoff"
	if match.Round == "final" {
		phase = "final"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"allowed":             true,
		"matchId":             match.ID,
		"worldSeed":           match.WorldSeed,
		"phase":               phase,
		"phaseLabel":          phaseLabel(phase),
		"canStartOfficialRun": canStart,
		"reason":              matchReason,
		"player1Connected":    match.Player1ConnectedAt != nil,
		"player2Connected":    match.Player2ConnectedAt != nil,
		"officialStartedAt":   match.OfficialStartedAt,
	})
}

type modRunSessionPayload struct {
	ModToken         string `json:"modToken"`
	RunSessionID     string `json:"runSessionId"`
	WorldFingerprint string `json:"worldFingerprint"`
	SeedHash         string `json:"seedHash"`
}

func (s *Server) handleModRunSessionCreate(w http.ResponseWriter, r *http.Request) {
	var payload modRunSessionPayload
	if err := s.decodeAndVerifyModJSON(r, &payload); err != nil {
		writeAPIError(w, errorStatus(err), err.Error())
		return
	}
	payload.WorldFingerprint = strings.TrimSpace(payload.WorldFingerprint)
	payload.SeedHash = strings.TrimSpace(payload.SeedHash)
	if payload.WorldFingerprint == "" || payload.SeedHash == "" {
		writeAPIError(w, http.StatusBadRequest, "run_session_world_required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	testMode, err := s.store.GetTestModeStatus(ctx)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load test mode")
		return
	}

	participant, token, _, reason, err := s.authorizeModParticipant(ctx, payload.ModToken, !testMode.Enabled)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to authorize mod")
		return
	}
	if participant == nil {
		status := http.StatusForbidden
		if reason == "tournament_not_running" || reason == "stream_offline" || reason == "stream_not_minecraft" {
			status = http.StatusConflict
		}
		writeAPIError(w, status, reason)
		return
	}
	if testMode.Enabled {
		if _, streamReason, err := s.validateParticipantStreamReady(ctx, participant); err != nil {
			writeAPIError(w, http.StatusConflict, "stream_check_unavailable")
			return
		} else if streamReason != "" {
			writeAPIError(w, http.StatusConflict, streamReason)
			return
		}
	}

	runContext := store.RunContext{Phase: "test"}
	runReason := ""
	if !testMode.Enabled {
		runContext, runReason, err = s.store.OfficialRunContext(ctx, participant.ID)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "failed to load tournament state")
			return
		}
		if runReason != "" {
			writeAPIError(w, http.StatusConflict, runReason)
			return
		}
	}

	session, err := s.store.CreateModRunSession(ctx, participant.ID, payload.WorldFingerprint, payload.SeedHash, runContext)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to create run session")
		return
	}
	if err := s.store.TouchModAuthToken(ctx, token.ID); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to update mod token usage")
		return
	}

	writeJSON(w, http.StatusCreated, modRunSessionResponse(participant, session, runContext))
}

func (s *Server) handleModRunSessionValidate(w http.ResponseWriter, r *http.Request) {
	var payload modRunSessionPayload
	if err := s.decodeAndVerifyModJSON(r, &payload); err != nil {
		writeAPIError(w, errorStatus(err), err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	testMode, err := s.store.GetTestModeStatus(ctx)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load test mode")
		return
	}
	participant, _, _, reason, err := s.authorizeModParticipant(ctx, payload.ModToken, !testMode.Enabled)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to authorize mod")
		return
	}
	if participant == nil {
		writeAPIError(w, http.StatusForbidden, reason)
		return
	}

	runContext := store.RunContext{Phase: "test"}
	if !testMode.Enabled {
		runContext, reason, err = s.store.OfficialRunContext(ctx, participant.ID)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "failed to load tournament state")
			return
		}
		if reason != "" {
			writeAPIError(w, http.StatusConflict, reason)
			return
		}
	}
	session, err := s.store.GetModRunSession(ctx, payload.RunSessionID, participant.ID)
	if err != nil {
		if store.IsRunSessionNotFound(err) {
			writeAPIError(w, http.StatusConflict, "run_session_missing")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load run session")
		return
	}
	if reason := store.ValidateModRunSession(session, payload.WorldFingerprint, payload.SeedHash, runContext); reason != "" {
		writeAPIError(w, http.StatusConflict, reason)
		return
	}

	writeJSON(w, http.StatusOK, modRunSessionResponse(participant, session, runContext))
}

func modRunSessionResponse(participant *store.Participant, session *store.ModRunSession, runContext store.RunContext) map[string]any {
	phase := runContext.Phase
	if phase == "" {
		phase = session.Phase
	}
	return map[string]any{
		"allowed":             true,
		"runSessionId":        session.ID,
		"runSessionBeta":      true,
		"startedAt":           session.StartedAt,
		"expiresAt":           session.ExpiresAt,
		"phase":               phase,
		"phaseLabel":          phaseLabel(phase),
		"canStartOfficialRun": true,
		"reason":              "",
		"matchId":             runContext.MatchID,
		"worldSeed":           runContext.WorldSeed,
		"participant": map[string]any{
			"twitchUserId": participant.TwitchUserID,
			"twitchLogin":  participant.TwitchLogin,
		},
		"tournament": map[string]any{
			"phase":               phase,
			"phaseLabel":          phaseLabel(phase),
			"canStartOfficialRun": true,
			"reason":              "",
			"tournamentId":        runContext.TournamentID,
			"matchId":             runContext.MatchID,
			"worldSeed":           runContext.WorldSeed,
		},
	}
}

type modRunSubmission struct {
	ModToken         string    `json:"modToken"`
	RunSessionID     string    `json:"runSessionId"`
	WorldFingerprint string    `json:"worldFingerprint"`
	SeedHash         string    `json:"seedHash"`
	Phase            string    `json:"phase"`
	MatchID          string    `json:"matchId"`
	AdminRoute       bool      `json:"adminRoute"`
	TimeMS           int       `json:"timeMs"`
	NetherSplitMS    *int      `json:"netherSplitMs"`
	EndSplitMS       *int      `json:"endSplitMs"`
	StartedAt        time.Time `json:"startedAt"`
	FinishedAt       time.Time `json:"finishedAt"`
}

func (s *Server) handleModRuns(w http.ResponseWriter, r *http.Request) {
	var payload modRunSubmission
	if err := s.decodeAndVerifyModJSON(r, &payload); err != nil {
		writeAPIError(w, errorStatus(err), err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	testMode, err := s.store.GetTestModeStatus(ctx)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load test mode")
		return
	}
	if testMode.Enabled {
		s.storeModTestRun(w, r, ctx, payload)
		return
	}

	if payload.AdminRoute {
		writeAPIError(w, http.StatusConflict, "test_runs_disabled")
		return
	}

	participant, token, _, reason, err := s.authorizeModParticipant(r.Context(), payload.ModToken, true)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to authorize mod")
		return
	}
	if participant == nil {
		status := http.StatusForbidden
		if reason == "tournament_not_running" || reason == "stream_offline" {
			status = http.StatusConflict
		}
		writeAPIError(w, status, reason)
		return
	}
	if !validRunWindow(payload.TimeMS, payload.StartedAt, payload.FinishedAt) {
		writeAPIError(w, http.StatusBadRequest, "invalid run timing")
		return
	}
	if !validRunSplits(payload.TimeMS, payload.NetherSplitMS, payload.EndSplitMS) {
		writeAPIError(w, http.StatusBadRequest, "invalid run splits")
		return
	}

	runContext, runReason, err := s.store.OfficialRunContext(ctx, participant.ID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load tournament state")
		return
	}
	if runReason != "" {
		writeAPIError(w, http.StatusConflict, runReason)
		return
	}
	if payload.Phase != "" && payload.Phase != runContext.Phase {
		writeAPIError(w, http.StatusConflict, "run phase is no longer active")
		return
	}
	if payload.MatchID != "" && payload.MatchID != runContext.MatchID {
		writeAPIError(w, http.StatusConflict, "match window is no longer active")
		return
	}
	usesRunSession := strings.TrimSpace(payload.RunSessionID) != ""
	if usesRunSession {
		session, err := s.store.GetModRunSession(ctx, payload.RunSessionID, participant.ID)
		if err != nil {
			if store.IsRunSessionNotFound(err) {
				writeAPIError(w, http.StatusConflict, "run_session_missing")
				return
			}
			writeAPIError(w, http.StatusInternalServerError, "failed to load run session")
			return
		}
		if reason := store.ValidateModRunSession(session, payload.WorldFingerprint, payload.SeedHash, runContext); reason != "" {
			writeAPIError(w, http.StatusConflict, reason)
			return
		}
	}

	run, err := s.store.StoreRun(ctx, participant.TwitchUserID, payload.TimeMS, payload.StartedAt, payload.FinishedAt, "mod", store.RunSplitInput{
		NetherSplitMS: payload.NetherSplitMS,
		EndSplitMS:    payload.EndSplitMS,
	}, runContext)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeAPIError(w, http.StatusNotFound, "participant not found")
			return
		}
		log.Printf("[ERROR] failed to store run: %v", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to store run")
		return
	}
	if err := s.store.TouchModAuthToken(ctx, token.ID); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to update mod token usage")
		return
	}
	if usesRunSession {
		if err := s.store.CompleteModRunSession(ctx, payload.RunSessionID, participant.ID, run.ID); err != nil {
			writeAPIError(w, http.StatusConflict, "run_session_closed")
			return
		}
	}

	writeJSON(w, http.StatusCreated, run)
}

func (s *Server) storeModTestRun(w http.ResponseWriter, r *http.Request, ctx context.Context, payload modRunSubmission) {
	participant, token, _, reason, err := s.authorizeModParticipant(r.Context(), payload.ModToken, false)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to authorize mod")
		return
	}
	if participant == nil {
		writeAPIError(w, http.StatusForbidden, reason)
		return
	}
	if _, streamReason, err := s.validateParticipantStreamReady(ctx, participant); err != nil {
		writeAPIError(w, http.StatusConflict, "stream_check_unavailable")
		return
	} else if streamReason != "" {
		writeAPIError(w, http.StatusConflict, streamReason)
		return
	}
	if !validRunWindow(payload.TimeMS, payload.StartedAt, payload.FinishedAt) {
		writeAPIError(w, http.StatusBadRequest, "invalid run timing")
		return
	}
	if !validRunSplits(payload.TimeMS, payload.NetherSplitMS, payload.EndSplitMS) {
		writeAPIError(w, http.StatusBadRequest, "invalid run splits")
		return
	}

	run, err := s.store.StoreRun(ctx, participant.TwitchUserID, payload.TimeMS, payload.StartedAt, payload.FinishedAt, "admin", store.RunSplitInput{
		NetherSplitMS: payload.NetherSplitMS,
		EndSplitMS:    payload.EndSplitMS,
	}, store.RunContext{Phase: "test"})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeAPIError(w, http.StatusNotFound, "participant not found")
			return
		}
		log.Printf("[ERROR] failed to store test run: %v", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to store run")
		return
	}
	if err := s.store.TouchModAuthToken(ctx, token.ID); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to update mod token usage")
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

func (s *Server) handleAdminParticipants(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	participants, err := s.store.ListParticipants(ctx)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load participants")
		return
	}
	if s.cfg.EnableTwitchStreams && s.cfg.TwitchEnabled() {
		for i := range participants {
			if strings.HasPrefix(participants[i].TwitchUserID, "pending:") {
				continue
			}
			if !staleSync(participants[i].LastStreamStatusSyncAt, s.cfg.StreamStatusCacheTTL) {
				continue
			}
			info, liveErr := s.twitch.GetStreamInfo(ctx, participants[i].TwitchUserID)
			if liveErr != nil {
				continue
			}
			live := info != nil
			title := ""
			gameName := ""
			if info != nil {
				title = info.Title
				gameName = info.GameName
			}
			participants[i].StreamOnline = live
			participants[i].StreamTitle = title
			participants[i].StreamGameName = gameName
			_ = s.store.SetStreamInfo(ctx, participants[i].TwitchUserID, live, title, gameName)
		}
	}
	writeJSON(w, http.StatusOK, participants)
}

func (s *Server) handleAdminWhitelist(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}

	var payload struct {
		TwitchUserID      string `json:"twitchUserId"`
		TwitchLogin       string `json:"twitchLogin"`
		TwitchDisplayName string `json:"twitchDisplayName"`
	}
	if err := decodeJSON(r, &payload); err != nil || !validTwitchLogin(payload.TwitchLogin) {
		writeAPIError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	var participant *store.Participant
	var err error
	if strings.TrimSpace(payload.TwitchUserID) == "" {
		participant, err = s.store.UpsertWhitelistLogin(ctx, strings.ToLower(payload.TwitchLogin))
	} else {
		participant, err = s.store.UpsertWhitelistParticipant(ctx, payload.TwitchUserID, strings.ToLower(payload.TwitchLogin), strings.TrimSpace(payload.TwitchDisplayName))
	}
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to update whitelist")
		return
	}
	writeJSON(w, http.StatusCreated, participant)
}

func (s *Server) handleAdminStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	twitchUserID := strings.TrimSpace(r.PathValue("twitchUserId"))
	var payload struct {
		Status string `json:"status"`
	}
	if err := decodeJSON(r, &payload); err != nil || !validParticipantStatus(payload.Status) {
		writeAPIError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	participant, err := s.store.UpdateParticipantStatus(ctx, twitchUserID, payload.Status)
	if err != nil || participant == nil {
		writeAPIError(w, http.StatusNotFound, "participant not found")
		return
	}
	writeJSON(w, http.StatusOK, participant)
}

func (s *Server) handleAdminTournament(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	var payload struct {
		StartsAt            time.Time `json:"startsAt"`
		QualificationEndsAt time.Time `json:"qualificationEndsAt"`
		PlayoffEndsAt       time.Time `json:"playoffEndsAt"`
		EndsAt              time.Time `json:"endsAt"`
		State               string    `json:"state"`
		PlayoffSlots        int       `json:"playoffSlots"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	if payload.PlayoffSlots == 0 {
		payload.PlayoffSlots = defaultPlayoffSlots
	}
	if payload.QualificationEndsAt.IsZero() || payload.PlayoffEndsAt.IsZero() || payload.EndsAt.IsZero() {
		startsAt := payload.StartsAt
		if startsAt.IsZero() {
			startsAt = time.Now()
		}
		payload.StartsAt, payload.QualificationEndsAt, payload.PlayoffEndsAt, payload.EndsAt, payload.PlayoffSlots = defaultTournamentSchedule(startsAt)
	}
	if !validTournamentState(payload.State) || !validTournamentSchedule(payload.StartsAt, payload.QualificationEndsAt, payload.PlayoffEndsAt, payload.EndsAt, payload.PlayoffSlots) {
		writeAPIError(w, http.StatusBadRequest, "invalid tournament schedule")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	overlap, err := s.store.HasOverlappingTournament(ctx, payload.StartsAt, payload.EndsAt)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to validate tournament window")
		return
	}
	if overlap {
		writeAPIError(w, http.StatusConflict, "tournament window overlaps an existing scheduled or running period")
		return
	}
	tournament, err := s.store.CreateTournament(ctx, payload.StartsAt, payload.QualificationEndsAt, payload.PlayoffEndsAt, payload.EndsAt, payload.State, payload.PlayoffSlots)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to create tournament")
		return
	}
	writeJSON(w, http.StatusCreated, tournament)
}

func (s *Server) handleAdminTournamentUpdate(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	tournamentID := strings.TrimSpace(r.PathValue("id"))
	var payload struct {
		StartsAt            time.Time `json:"startsAt"`
		QualificationEndsAt time.Time `json:"qualificationEndsAt"`
		PlayoffEndsAt       time.Time `json:"playoffEndsAt"`
		EndsAt              time.Time `json:"endsAt"`
		State               string    `json:"state"`
		PlayoffSlots        int       `json:"playoffSlots"`
	}
	if tournamentID == "" || decodeJSON(r, &payload) != nil || !validTournamentState(payload.State) || !validTournamentSchedule(payload.StartsAt, payload.QualificationEndsAt, payload.PlayoffEndsAt, payload.EndsAt, payload.PlayoffSlots) {
		writeAPIError(w, http.StatusBadRequest, "invalid tournament schedule")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tournament, err := s.store.UpdateTournamentSchedule(ctx, tournamentID, payload.StartsAt, payload.QualificationEndsAt, payload.PlayoffEndsAt, payload.EndsAt, payload.State, payload.PlayoffSlots)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeAPIError(w, http.StatusNotFound, "tournament not found")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to update tournament")
		return
	}
	writeJSON(w, http.StatusOK, tournament)
}

func (s *Server) handleAdminTournamentBracket(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	tournamentID := strings.TrimSpace(r.PathValue("id"))
	if tournamentID == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid tournament id")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	matches, err := s.store.GeneratePlayoffBracket(ctx, tournamentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeAPIError(w, http.StatusNotFound, "tournament not found")
			return
		}
		writeAPIError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"matches": matches})
}

func (s *Server) handleAdminTournamentMatchesControl(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	tournamentID := strings.TrimSpace(r.PathValue("id"))
	if tournamentID == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid tournament id")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tournament, err := s.store.GetCurrentTournament(ctx)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load tournament")
		return
	}
	if tournament == nil || tournament.ID != tournamentID {
		writeAPIError(w, http.StatusNotFound, "tournament not found")
		return
	}
	matches, err := s.store.ListTournamentMatches(ctx, tournamentID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load matches")
		return
	}
	if matches == nil {
		matches = []store.TournamentMatch{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"tournament": tournament, "matches": matches})
}

func (s *Server) handleAdminTournamentMatchesUpdate(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	tournamentID := strings.TrimSpace(r.PathValue("id"))
	var payload struct {
		Matches []struct {
			ID        string    `json:"id"`
			StartsAt  time.Time `json:"startsAt"`
			EndsAt    time.Time `json:"endsAt"`
			WorldSeed string    `json:"worldSeed"`
		} `json:"matches"`
	}
	if tournamentID == "" || decodeJSON(r, &payload) != nil || len(payload.Matches) == 0 || len(payload.Matches) > 64 {
		writeAPIError(w, http.StatusBadRequest, "invalid match schedule")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tournament, err := s.store.GetCurrentTournament(ctx)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load tournament")
		return
	}
	if tournament == nil || tournament.ID != tournamentID {
		writeAPIError(w, http.StatusNotFound, "tournament not found")
		return
	}

	updates := make([]store.TournamentMatchScheduleUpdate, 0, len(payload.Matches))
	seen := map[string]bool{}
	for _, match := range payload.Matches {
		match.ID = strings.TrimSpace(match.ID)
		if match.ID == "" || seen[match.ID] || !match.EndsAt.After(match.StartsAt) {
			writeAPIError(w, http.StatusBadRequest, "invalid match schedule")
			return
		}
		if match.StartsAt.Before(tournament.QualificationEndsAt) || match.EndsAt.After(tournament.EndsAt) {
			writeAPIError(w, http.StatusBadRequest, "match day must stay after qualification and inside tournament")
			return
		}
		match.WorldSeed = strings.TrimSpace(match.WorldSeed)
		if match.WorldSeed != "" {
			if _, err := strconv.ParseInt(match.WorldSeed, 10, 64); err != nil {
				writeAPIError(w, http.StatusBadRequest, "match seed must be a signed 64-bit number")
				return
			}
		}
		seen[match.ID] = true
		updates = append(updates, store.TournamentMatchScheduleUpdate{
			ID:        match.ID,
			StartsAt:  match.StartsAt,
			EndsAt:    match.EndsAt,
			WorldSeed: match.WorldSeed,
		})
	}

	matches, err := s.store.UpdateTournamentMatchSchedule(ctx, tournamentID, updates)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeAPIError(w, http.StatusNotFound, "match not found")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to update match schedule")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"matches": matches})
}

func (s *Server) handleAdminTournamentMatchStart(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	tournamentID := strings.TrimSpace(r.PathValue("id"))
	matchID := strings.TrimSpace(r.PathValue("matchId"))
	if tournamentID == "" || matchID == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid match id")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	match, err := s.store.StartTournamentMatch(ctx, tournamentID, matchID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeAPIError(w, http.StatusConflict, "both players must connect during the active match window")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to start match")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"match": match})
}

func (s *Server) handleAdminTournamentMatchReset(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	tournamentID := strings.TrimSpace(r.PathValue("id"))
	matchID := strings.TrimSpace(r.PathValue("matchId"))
	if tournamentID == "" || matchID == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid match id")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	match, err := s.store.ResetTournamentMatch(ctx, tournamentID, matchID)
	if err != nil {
		log.Printf("[ERROR] failed to reset match: %v", err)
		if errors.Is(err, sql.ErrNoRows) {
			writeAPIError(w, http.StatusNotFound, "match not found")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to reset match")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"match": match})
}

func (s *Server) handleAdminTournamentStop(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.store.FinishRunningTournaments(ctx); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to stop tournament")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminDiscordSyncRoles(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	if s.discord == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "discord bot is disabled")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 55*time.Second)
	defer cancel()
	message, err := s.discord.SyncRunnerRoles(ctx, s.store)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": message})
}

func (s *Server) handleAdminNewsSync(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 55*time.Second)
	defer cancel()
	result, err := telegrambot.SyncPublicChannelNewsHistory(ctx, s.cfg.TelegramNewsChat, s.uploadDir, s.store, nil)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleAdminLeaderboardClear(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.store.ClearLeaderboard(ctx); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to clear leaderboard")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminTestRunsClear(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.store.ClearTestRuns(ctx); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to clear test runs")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminTestMode(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	status, err := s.store.GetTestModeStatus(ctx)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load test mode")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleAdminTestModeEnable(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	var payload struct {
		Days int `json:"days"`
	}
	if err := decodeJSON(r, &payload); err != nil && !errors.Is(err, errEmptyBody) {
		writeAPIError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	if payload.Days <= 0 {
		payload.Days = 5
	}
	if payload.Days > 30 {
		payload.Days = 30
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	status, err := s.store.EnableTestMode(ctx, time.Now().UTC().Add(time.Duration(payload.Days)*24*time.Hour))
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to enable test mode")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleAdminTestModeDisable(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.store.DisableTestMode(ctx); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to disable test mode")
		return
	}
	writeJSON(w, http.StatusOK, store.TestModeStatus{})
}

func (s *Server) handleAdminTestCertificate(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}

	status := r.URL.Query().Get("status")
	var certStatus string
	switch status {
	case "winner":
		certStatus = "ПОБЕДИТЕЛЬ"
	case "finalist":
		certStatus = "ФИНАЛИСТ"
	case "playoff":
		certStatus = "ИГРОК ПЛЕЙ-ОФФ"
	default:
		certStatus = "КВАЛИФИКАНТ"
	}

	bestTime := 1423450 // 23:43.450
	p := &store.Participant{
		ID:                    "test-participant-id",
		TwitchLogin:           "test_streamer",
		TwitchDisplayName:     "Р СћР ВµРЎРѓРЎвЂљР С•Р Р†РЎвЂ№Р в„– Р РЋРЎвЂљРЎР‚Р С‘Р СР ВµРЎР‚",
		MinecraftNick:         "TestPlayer",
		TwitchProfileImageURL: "https://static-cdn.jtvnw.net/user-default-pictures-uv/cddc22b2-4298-4fcb-b00e-72cbd498b6d7-profile_image-70x70.png",
		AvatarURL:             "",
		BestTimeMS:            &bestTime,
	}

	img, err := s.generateCertificateImage(r.Context(), p, certStatus, "CERT-2026-999999")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to render test certificate: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"tournament_test_certificate_%s.png\"", status))
	_ = png.Encode(w, img)
}

func (s *Server) handleNotificationsPage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	notifications, err := s.store.ListGlobalNotifications(ctx, 50)
	if err != nil {
		http.Error(w, "Failed to load notifications", http.StatusInternalServerError)
		return
	}

	_ = s.renderTemplate(w, "notifications.html", map[string]any{
		"TournamentTitle": s.cfg.TournamentTitle,
		"Notifications":   notifications,
	})
}

func (s *Server) handleAdminApplicationsStatusGet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	closed, err := s.store.AreApplicationsClosed(ctx)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load applications status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"closed": closed})
}

func (s *Server) handleAdminApplicationsStatusSet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	var payload struct {
		Closed bool `json:"closed"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.store.SetApplicationsClosed(ctx, payload.Closed); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to update applications status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"closed": payload.Closed})
}

func (s *Server) handleAdminPushNotificationGet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	title, text, err := s.store.GetCustomNotification(ctx)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load custom notification")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"title": title, "text": text})
}

func (s *Server) handleAdminPushNotificationSet(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	var payload struct {
		Title string `json:"title"`
		Text  string `json:"text"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	payload.Text = strings.TrimSpace(payload.Text)
	if payload.Text == "" {
		writeAPIError(w, http.StatusBadRequest, "notification text is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.store.SetCustomNotification(ctx, payload.Title, payload.Text); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to update custom notification")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"title": payload.Title, "text": payload.Text})
}

func (s *Server) handleAdminPushNotificationClear(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.store.ClearCustomNotification(ctx); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to clear custom notification")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminRunsPage(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.currentClaims(r)
	if !ok {
		http.Redirect(w, r, "/login?error=login_required", http.StatusSeeOther)
		return
	}
	if claims.Role != "admin" {
		http.Error(w, "admin role required", http.StatusForbidden)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	participants, err := s.store.ListParticipantsForAdminRuns(ctx)
	if err != nil {
		http.Error(w, "Failed to load participants", http.StatusInternalServerError)
		return
	}

	_ = s.renderTemplate(w, "admin_runs.html", map[string]any{
		"TournamentTitle": s.cfg.TournamentTitle,
		"Participants":    participants,
	})
}

func (s *Server) handleAdminParticipantRunsPage(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.currentClaims(r)
	if !ok {
		http.Redirect(w, r, "/login?error=login_required", http.StatusSeeOther)
		return
	}
	if claims.Role != "admin" {
		http.Error(w, "admin role required", http.StatusForbidden)
		return
	}

	participantID := strings.TrimSpace(r.PathValue("id"))
	if participantID == "" {
		http.Error(w, "invalid participant id", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	participant, err := s.store.FindParticipantByID(ctx, participantID)
	if err != nil || participant == nil {
		http.Error(w, "participant not found", http.StatusNotFound)
		return
	}

	runs, err := s.store.ListParticipantRuns(ctx, participantID)
	if err != nil {
		http.Error(w, "Failed to load participant runs", http.StatusInternalServerError)
		return
	}

	tournament, err := s.store.GetCurrentTournament(ctx)
	var currentPhase string
	if err == nil && tournament != nil {
		currentPhase = phaseFromTournament(tournament)
	}

	_ = s.renderTemplate(w, "admin_participant_runs.html", map[string]any{
		"TournamentTitle": s.cfg.TournamentTitle,
		"Participant":     participant,
		"Runs":            runs,
		"CurrentPhase":    currentPhase,
	})
}

func (s *Server) handleAdminRunReject(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	runID := strings.TrimSpace(r.PathValue("id"))
	if runID == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid run id")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	_, newBestTimeMS, err := s.store.UpdateRunStatusAndRecalculate(ctx, runID, "rejected")
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to reject run: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "rejected",
		"pb":     formatDuration(newBestTimeMS),
	})
}

func (s *Server) handleAdminRunApprove(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	runID := strings.TrimSpace(r.PathValue("id"))
	if runID == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid run id")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	_, newBestTimeMS, err := s.store.UpdateRunStatusAndRecalculate(ctx, runID, "approved")
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to approve run: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "approved",
		"pb":     formatDuration(newBestTimeMS),
	})
}

type adminRunAddRequest struct {
	Time        string `json:"time"`
	NetherSplit string `json:"netherSplit"`
	EndSplit    string `json:"endSplit"`
	Phase       string `json:"phase"`
	MatchID     string `json:"matchId"`
}

func (s *Server) handleAdminRunAdd(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	participantID := strings.TrimSpace(r.PathValue("id"))
	if participantID == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid participant id")
		return
	}

	var req adminRunAddRequest
	isForm := strings.Contains(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded")
	if isForm {
		if err := r.ParseForm(); err != nil {
			writeAPIError(w, http.StatusBadRequest, "failed to parse form data: "+err.Error())
			return
		}
		req.Time = r.FormValue("time")
		req.NetherSplit = r.FormValue("netherSplit")
		req.EndSplit = r.FormValue("endSplit")
		req.Phase = r.FormValue("phase")
		req.MatchID = r.FormValue("matchId")
	} else {
		if err := decodeJSON(r, &req); err != nil {
			writeAPIError(w, http.StatusBadRequest, "failed to decode request body: "+err.Error())
			return
		}
	}

	timeMS, err := parseDurationToMS(req.Time)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "Р СњР ВµР Р†Р ВµРЎР‚Р Р…РЎвЂ№Р в„– РЎвЂћР С•РЎР‚Р СР В°РЎвЂљ Р Р†РЎР‚Р ВµР СР ВµР Р…Р С‘ Р В·Р В°Р В±Р ВµР С–Р В°: "+err.Error())
		return
	}
	if timeMS <= 0 {
		writeAPIError(w, http.StatusBadRequest, "Р вЂ™РЎР‚Р ВµР СРЎРЏ Р В·Р В°Р В±Р ВµР С–Р В° Р Т‘Р С•Р В»Р В¶Р Р…Р С• Р В±РЎвЂ№РЎвЂљРЎРЉ Р В±Р С•Р В»РЎРЉРЎв‚¬Р Вµ Р Р…РЎС“Р В»РЎРЏ")
		return
	}

	var netherSplitMS *int
	if strings.TrimSpace(req.NetherSplit) != "" {
		ms, err := parseDurationToMS(req.NetherSplit)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "Р СњР ВµР Р†Р ВµРЎР‚Р Р…РЎвЂ№Р в„– РЎвЂћР С•РЎР‚Р СР В°РЎвЂљ РЎРѓР С—Р В»Р С‘РЎвЂљР В° Р СњР ВµР В·Р ВµРЎР‚Р В°: "+err.Error())
			return
		}
		netherSplitMS = &ms
	}

	var endSplitMS *int
	if strings.TrimSpace(req.EndSplit) != "" {
		ms, err := parseDurationToMS(req.EndSplit)
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, "Р СњР ВµР Р†Р ВµРЎР‚Р Р…РЎвЂ№Р в„– РЎвЂћР С•РЎР‚Р СР В°РЎвЂљ РЎРѓР С—Р В»Р С‘РЎвЂљР В° Р В­Р Р…Р Т‘Р В°: "+err.Error())
			return
		}
		endSplitMS = &ms
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	participant, err := s.store.FindParticipantByID(ctx, participantID)
	if err != nil || participant == nil {
		writeAPIError(w, http.StatusNotFound, "Р Р€РЎвЂЎР В°РЎРѓРЎвЂљР Р…Р С‘Р С” Р Р…Р Вµ Р Р…Р В°Р в„–Р Т‘Р ВµР Р…")
		return
	}

	splits := store.RunSplitInput{
		NetherSplitMS: netherSplitMS,
		EndSplitMS:    endSplitMS,
	}

	tournament, err := s.store.GetCurrentTournament(ctx)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Р С›РЎв‚¬Р С‘Р В±Р С”Р В° Р С—РЎР‚Р С‘ Р В·Р В°Р С–РЎР‚РЎС“Р В·Р С”Р Вµ РЎвЂљРЎС“РЎР‚Р Р…Р С‘РЎР‚Р В°: "+err.Error())
		return
	}

	var tournamentID string
	if tournament != nil {
		tournamentID = tournament.ID
	}

	runContext := store.RunContext{
		TournamentID: tournamentID,
		Phase:        strings.TrimSpace(req.Phase),
		MatchID:      strings.TrimSpace(req.MatchID),
	}

	finishedAt := time.Now()
	startedAt := finishedAt.Add(-time.Duration(timeMS) * time.Millisecond)

	run, err := s.store.StoreRun(ctx, participant.TwitchUserID, timeMS, startedAt, finishedAt, "admin", splits, runContext)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "Р С›РЎв‚¬Р С‘Р В±Р С”Р В° Р С—РЎР‚Р С‘ РЎРѓР С•РЎвЂ¦РЎР‚Р В°Р Р…Р ВµР Р…Р С‘Р С‘ Р В·Р В°Р В±Р ВµР С–Р В°: "+err.Error())
		return
	}

	// Recalculate best time after storing to get the new PB format
	updatedParticipant, err := s.store.FindParticipantByID(ctx, participantID)
	var pbStr string = "Р СњР ВµРЎвЂљ"
	if err == nil && updatedParticipant != nil && updatedParticipant.BestTimeMS != nil {
		pbStr = formatDuration(*updatedParticipant.BestTimeMS)
	}

	if isForm {
		http.Redirect(w, r, "/admin/runs/"+participantID, http.StatusSeeOther)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"runId":   run.ID,
		"pb":      pbStr,
	})
}

func parseDurationToMS(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("Р С—РЎС“РЎРѓРЎвЂљР С•Р Вµ Р В·Р Р…Р В°РЎвЂЎР ВµР Р…Р С‘Р Вµ")
	}

	// Р вЂўРЎРѓР В»Р С‘ РЎРЊРЎвЂљР С• Р С—РЎР‚Р С•РЎРѓРЎвЂљР С• РЎвЂЎР С‘РЎРѓР В»Р С• (Р СР С‘Р В»Р В»Р С‘РЎРѓР ВµР С”РЎС“Р Р…Р Т‘РЎвЂ№)
	if !strings.Contains(s, ":") {
		ms, err := strconv.Atoi(s)
		if err != nil {
			return 0, fmt.Errorf("Р Р…Р Вµ РЎС“Р Т‘Р В°Р В»Р С•РЎРѓРЎРЉ Р С—РЎР‚Р С•РЎвЂЎР С‘РЎвЂљР В°РЎвЂљРЎРЉ РЎвЂЎР С‘РЎРѓР В»Р С• Р СР С‘Р В»Р В»Р С‘РЎРѓР ВµР С”РЎС“Р Р…Р Т‘")
		}
		return ms, nil
	}

	// Р В¤Р С•РЎР‚Р СР В°РЎвЂљ РЎРѓ Р Т‘Р Р†Р С•Р ВµРЎвЂљР С•РЎвЂЎР С‘РЎРЏР СР С‘ (H:MM:SS.mmm Р С‘Р В»Р С‘ MM:SS.mmm Р С‘Р В»Р С‘ MM:SS)
	var msPart int
	mainPart := s
	if idx := strings.Index(s, "."); idx != -1 {
		mainPart = s[:idx]
		msStr := s[idx+1:]
		if len(msStr) > 3 {
			msStr = msStr[:3]
		}
		for len(msStr) < 3 {
			msStr += "0"
		}
		var err error
		msPart, err = strconv.Atoi(msStr)
		if err != nil {
			return 0, fmt.Errorf("Р Р…Р Вµ РЎС“Р Т‘Р В°Р В»Р С•РЎРѓРЎРЉ Р С—РЎР‚Р С•РЎвЂЎР С‘РЎвЂљР В°РЎвЂљРЎРЉ Р СР С‘Р В»Р В»Р С‘РЎРѓР ВµР С”РЎС“Р Р…Р Т‘РЎвЂ№ Р С—Р С•РЎРѓР В»Р Вµ РЎвЂљР С•РЎвЂЎР С”Р С‘")
		}
	}

	parts := strings.Split(mainPart, ":")
	if len(parts) == 2 {
		// MM:SS
		m, err1 := strconv.Atoi(parts[0])
		sec, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			return 0, fmt.Errorf("Р Р…Р ВµР Р†Р ВµРЎР‚Р Р…РЎвЂ№Р в„– РЎвЂћР С•РЎР‚Р СР В°РЎвЂљ Р СР С‘Р Р…РЎС“РЎвЂљ/РЎРѓР ВµР С”РЎС“Р Р…Р Т‘")
		}
		return (m*60+sec)*1000 + msPart, nil
	} else if len(parts) == 3 {
		// H:MM:SS
		h, err1 := strconv.Atoi(parts[0])
		m, err2 := strconv.Atoi(parts[1])
		sec, err3 := strconv.Atoi(parts[2])
		if err1 != nil || err2 != nil || err3 != nil {
			return 0, fmt.Errorf("Р Р…Р ВµР Р†Р ВµРЎР‚Р Р…РЎвЂ№Р в„– РЎвЂћР С•РЎР‚Р СР В°РЎвЂљ РЎвЂЎР В°РЎРѓР С•Р Р†/Р СР С‘Р Р…РЎС“РЎвЂљ/РЎРѓР ВµР С”РЎС“Р Р…Р Т‘")
		}
		return (h*3600+m*60+sec)*1000 + msPart, nil
	}

	return 0, fmt.Errorf("Р Р…Р ВµР С‘Р В·Р Р†Р ВµРЎРѓРЎвЂљР Р…РЎвЂ№Р в„– РЎвЂћР С•РЎР‚Р СР В°РЎвЂљ Р Р†РЎР‚Р ВµР СР ВµР Р…Р С‘ (Р С•Р В¶Р С‘Р Т‘Р В°Р ВµРЎвЂљРЎРѓРЎРЏ MM:SS Р С‘Р В»Р С‘ H:MM:SS)")
}

func (s *Server) handlePickemsPage(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.currentClaims(r)

	ctx := r.Context()
	tournament, err := s.store.GetCurrentTournament(ctx)
	if err != nil {
		http.Error(w, "failed to load tournament", http.StatusInternalServerError)
		return
	}

	view := buildTournamentStatusView(tournament)

	matches := []store.TournamentMatch{}
	if tournament != nil && view.BracketOpen {
		matches, _ = s.store.ListTournamentMatches(ctx, tournament.ID)
	}

	userPredictions := make(map[string]string)
	var userScore int
	var userCorrectGuesses int
	var pickemsSubmitted bool
	if ok {
		userPredictions, _ = s.store.GetUserPredictions(ctx, claims.TwitchUserID)
		if tournament != nil {
			pickemsSubmitted, _ = s.store.HasSubmittedPickems(ctx, claims.TwitchUserID, tournament.ID)
		}

		// Only a confirmed bracket participates in scoring.
		if pickemsSubmitted {
			for _, m := range matches {
				if pred, exists := userPredictions[m.ID]; exists && m.WinnerParticipantID != "" {
					if pred == m.WinnerParticipantID {
						userScore += store.PredictionPointsForRound(m.Round)
						userCorrectGuesses++
					}
				}
			}
		}
	}

	// Fetch match prediction statistics for each match
	matchStats := make(map[string]store.MatchPredictionStats)
	for _, m := range matches {
		if m.Player1 != nil && m.Player2 != nil {
			stats, _ := s.store.GetMatchPredictionStats(ctx, m.ID, m.Player1.ID, m.Player2.ID)
			matchStats[m.ID] = stats
		}
	}

	leaderboard, _ := s.store.GetPredictionsLeaderboard(ctx)

	pageData := map[string]any{
		"TournamentTitle":    s.cfg.TournamentTitle,
		"Tournament":         view,
		"Rounds":             buildBracketRounds(matches),
		"Matches":            matches,
		"User":               claims,
		"IsLoggedIn":         ok,
		"UserPredictions":    userPredictions,
		"PickemsSubmitted":   pickemsSubmitted,
		"UserScore":          userScore,
		"UserCorrectGuesses": userCorrectGuesses,
		"Leaderboard":        leaderboard,
		"MatchStats":         matchStats,
		"AllowDevMockAuth":   s.cfg.AllowDevMockAuth,
	}

	_ = s.renderTemplate(w, "pickems.html", pageData)
}

func (s *Server) handleSavePredictionAPI(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.currentClaims(r)
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		Predictions []struct {
			MatchID           string `json:"matchId"`
			PredictedWinnerID string `json:"predictedWinnerId"`
		} `json:"predictions"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid payload")
		return
	}

	if len(req.Predictions) == 0 {
		writeAPIError(w, http.StatusBadRequest, "predictions are required")
		return
	}

	tournament, err := s.store.GetCurrentTournament(r.Context())
	if err != nil || tournament == nil {
		writeAPIError(w, http.StatusBadRequest, "tournament_not_found")
		return
	}
	predictions := make(map[string]string, len(req.Predictions))
	for _, prediction := range req.Predictions {
		if prediction.MatchID == "" || prediction.PredictedWinnerID == "" {
			writeAPIError(w, http.StatusBadRequest, "invalid_prediction")
			return
		}
		if _, duplicate := predictions[prediction.MatchID]; duplicate {
			writeAPIError(w, http.StatusBadRequest, "duplicate_match")
			return
		}
		predictions[prediction.MatchID] = prediction.PredictedWinnerID
	}

	err = s.store.SubmitPickems(r.Context(), claims.TwitchUserID, claims.TwitchLogin, tournament.ID, predictions)
	if err != nil {
		if errors.Is(err, store.ErrPredictionsLocked) || errors.Is(err, store.ErrPickemsSubmitted) {
			writeAPIError(w, http.StatusConflict, "predictions_locked")
			return
		}
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
	})
}
