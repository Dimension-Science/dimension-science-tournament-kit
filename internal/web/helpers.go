package web

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dimension-science/tournament-kit/internal/session"
	"github.com/dimension-science/tournament-kit/internal/store"
)

var (
	errEmptyBody      = errors.New("empty body")
	errNotWhitelisted = errors.New("not whitelisted")
)

const modTokenTTL = 30 * 24 * time.Hour

type rateLimiter struct {
	max     int
	window  time.Duration
	mu      sync.Mutex
	entries map[string]rateEntry
}

type rateEntry struct {
	count   int
	resetAt time.Time
}

func newRateLimiter(max int, window time.Duration) *rateLimiter {
	return &rateLimiter{max: max, window: window, entries: map[string]rateEntry{}}
}

func (r *rateLimiter) Allow(key string) bool {
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.entries[key]
	if !ok || now.After(entry.resetAt) {
		r.entries[key] = rateEntry{count: 1, resetAt: now.Add(r.window)}
		return true
	}

	if entry.count >= r.max {
		return false
	}

	entry.count++
	r.entries[key] = entry
	return true
}

func (s *Server) withRateLimit(limiter *rateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow(clientIP(r)) {
			writeAPIError(w, http.StatusTooManyRequests, "too many requests")
			return
		}
		next(w, r)
	}
}

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if !s.handleCORS(w, r) {
				return
			}
		}

		w.Header().Set("Content-Security-Policy", "default-src 'self'; base-uri 'self'; connect-src 'self' https://www.google-analytics.com https://*.google-analytics.com https://*.analytics.google.com https://*.googletagmanager.com; form-action 'self' https://id.twitch.tv; frame-ancestors 'none'; img-src 'self' data: https://static-cdn.jtvnw.net https://www.google-analytics.com https://*.google-analytics.com; object-src 'none'; script-src 'self' 'unsafe-inline' https://www.googletagmanager.com; style-src 'self'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleCORS(w http.ResponseWriter, r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}

	allowed := s.cfg.OriginAllowed(origin) || strings.EqualFold(origin, requestOrigin(r))

	// Fallback for Origin: "null" (which browsers send for native form POSTs)
	if !allowed && strings.EqualFold(origin, "null") {
		referer := r.Header.Get("Referer")
		if referer != "" {
			reqOrig := requestOrigin(r)
			if reqOrig != "" && strings.HasPrefix(strings.ToLower(referer), strings.ToLower(reqOrig)) {
				allowed = true
			}
		}
	}

	if !allowed {
		fmt.Printf("CORS REJECTED: origin=%q, requestOrigin=%q, Host=%q, X-Forwarded-Proto=%q\n", origin, requestOrigin(r), r.Host, r.Header.Get("X-Forwarded-Proto"))
		writeAPIError(w, http.StatusForbidden, "origin is not allowed")
		return false
	}

	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Credentials", "true")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Mod-Key, X-Mod-Timestamp, X-Mod-Nonce, X-Mod-Signature")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return false
	}
	return true
}

func (s *Server) renderTemplate(w http.ResponseWriter, name string, data map[string]any) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if data == nil {
		data = make(map[string]any)
	}
	profile := s.cfg.PublicProfile()
	data["Profile"] = profile
	data["TournamentTitle"] = profile.SiteName
	data["SiteShortName"] = profile.ShortName
	data["BaseURL"] = profile.BaseURL
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	closed, _ := s.store.AreApplicationsClosed(ctx)
	data["ApplicationsClosed"] = closed
	title, text, _ := s.store.GetCustomNotification(ctx)
	data["CustomNotificationTitle"] = title
	data["CustomNotificationText"] = text
	return s.templates.ExecuteTemplate(w, name, data)
}

func (s *Server) currentClaims(r *http.Request) (session.Claims, bool) {
	token := session.ReadToken(r, s.cfg.AuthCookieName)
	if token == "" {
		return session.Claims{}, false
	}
	claims, err := s.sessions.Parse(token)
	if err != nil {
		return session.Claims{}, false
	}
	return claims, true
}

func participantCanAccessCabinet(participant *store.Participant) bool {
	return participant != nil && (participant.Status == "invited" || participant.Status == "active")
}

func (s *Server) ensureCabinetAccess(ctx context.Context, claims session.Claims) error {
	if claims.Role == "admin" {
		return nil
	}
	participant, err := s.store.FindParticipantByTwitchUserID(ctx, claims.TwitchUserID)
	if err != nil {
		return err
	}
	if !participantCanAccessCabinet(participant) {
		return errNotWhitelisted
	}
	return nil
}

func (s *Server) redirectCabinetAccessError(w http.ResponseWriter, r *http.Request, err error, fallbackCode string) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errNotWhitelisted) {
		s.clearSessionCookie(w)
		http.Redirect(w, r, "/login?error=not_whitelisted", http.StatusSeeOther)
		return true
	}
	http.Redirect(w, r, "/cabinet?error="+fallbackCode, http.StatusSeeOther)
	return true
}

func writeCabinetAccessAPIError(w http.ResponseWriter, err error, fallbackMessage string) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errNotWhitelisted) {
		writeAPIError(w, http.StatusForbidden, "not whitelisted")
		return true
	}
	writeAPIError(w, http.StatusInternalServerError, fallbackMessage)
	return true
}

func (s *Server) requireAdminAPI(w http.ResponseWriter, r *http.Request) (session.Claims, bool) {
	claims, ok := s.currentClaims(r)
	if !ok {
		writeAPIError(w, http.StatusUnauthorized, "authentication required")
		return session.Claims{}, false
	}
	if claims.Role != "admin" {
		writeAPIError(w, http.StatusForbidden, "admin role required")
		return session.Claims{}, false
	}
	return claims, true
}

func (s *Server) setSessionCookie(w http.ResponseWriter, twitchUserID, twitchLogin, role string) {
	token, _ := s.sessions.Issue(twitchUserID, twitchLogin, role)
	http.SetCookie(w, &http.Cookie{
		Name:     s.cfg.AuthCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.AuthCookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.cfg.SessionTTL.Seconds()),
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     s.cfg.AuthCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.AuthCookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (s *Server) loginWithIdentity(ctx context.Context, identity store.LoginIdentity) (string, error) {
	isAdmin, err := s.store.IsAdmin(ctx, identity.TwitchUserID)
	if err != nil {
		return "", err
	}
	if isAdmin {
		if _, err := s.store.UpsertParticipantLoginIdentity(ctx, identity, "active"); err != nil {
			return "", err
		}
		return "admin", nil
	}

	hasAdmins, err := s.store.HasAdmins(ctx)
	if err != nil {
		return "", err
	}
	if !hasAdmins {
		if err := s.store.UpsertAdmin(ctx, identity.TwitchUserID); err != nil {
			return "", err
		}
		return "admin", nil
	}

	participant, err := s.store.FindParticipantByTwitchUserID(ctx, identity.TwitchUserID)
	if err != nil {
		return "", err
	}
	if participant == nil {
		participant, err = s.store.FindWhitelistedParticipant(ctx, "", identity.TwitchLogin)
		if err != nil {
			return "", err
		}
	}
	if participant == nil || !participantCanAccessCabinet(participant) {
		return "viewer", nil
	}

	if err := s.store.UpdateParticipantLoginIdentityByID(ctx, participant.ID, identity); err != nil {
		return "", err
	}

	return "streamer", nil
}

func (s *Server) handleMockLogin(w http.ResponseWriter, r *http.Request, twitchUserID, twitchLogin string, jsonMode bool) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Check if this is an admin login
	isAdmin, err := s.store.IsAdmin(ctx, twitchUserID)
	if err == nil && isAdmin {
		s.setSessionCookie(w, twitchUserID, twitchLogin, "admin")
		if jsonMode {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "role": "admin", "twitchLogin": twitchLogin})
		} else {
			http.Redirect(w, r, "/admin", http.StatusSeeOther)
		}
		return
	}

	participant, err := s.store.FindWhitelistedParticipant(ctx, twitchUserID, twitchLogin)
	if err != nil || participant == nil {
		if jsonMode {
			writeAPIError(w, http.StatusNotFound, "no whitelisted streamer found")
		} else {
			http.Redirect(w, r, "/login?error=no_whitelisted_streamer", http.StatusSeeOther)
		}
		return
	}

	s.setSessionCookie(w, participant.TwitchUserID, participant.TwitchLogin, "streamer")
	if jsonMode {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "role": "streamer", "twitchLogin": participant.TwitchLogin})
		return
	}
	http.Redirect(w, r, "/cabinet", http.StatusSeeOther)
}

func (s *Server) loadCabinetData(ctx context.Context, claims session.Claims) (map[string]any, int, error) {
	participant, err := s.store.FindParticipantByTwitchUserID(ctx, claims.TwitchUserID)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	if participant == nil && claims.Role == "admin" {
		participant, err = s.store.UpsertParticipantLoginIdentity(ctx, store.LoginIdentity{
			TwitchUserID:      claims.TwitchUserID,
			TwitchLogin:       claims.TwitchLogin,
			TwitchDisplayName: claims.TwitchLogin,
		}, "active")
		if err != nil {
			return nil, http.StatusInternalServerError, err
		}
	}
	if participant == nil {
		return nil, http.StatusNotFound, errors.New("participant not found")
	}
	if claims.Role != "admin" && !participantCanAccessCabinet(participant) {
		return nil, http.StatusForbidden, errNotWhitelisted
	}

	streamOnline := participant.StreamOnline
	if s.cfg.EnableTwitchStreams && s.cfg.TwitchEnabled() && staleSync(participant.LastStreamStatusSyncAt, s.cfg.StreamStatusCacheTTL) {
		if streamInfo, liveErr := s.twitch.GetStreamInfo(ctx, participant.TwitchUserID); liveErr == nil {
			streamOnline = streamInfo != nil
			title := ""
			gameName := ""
			if streamInfo != nil {
				title = streamInfo.Title
				gameName = streamInfo.GameName
			}
			_ = s.store.SetStreamInfo(ctx, participant.TwitchUserID, streamOnline, title, gameName)
		}
	}

	profile := s.profilePayload(participant, claims, streamOnline)
	modToken, _ := s.store.GetModAuthTokenByParticipantID(ctx, participant.ID)
	bestRun, _ := s.store.FindBestApprovedRunByParticipantID(ctx, participant.ID)
	achievements, _ := s.store.ListAchievementProgress(ctx, participant.ID)
	achievementSummary := cabinetAchievementSummary(achievements)
	tokenDisplay := ""
	tokenPreview := ""
	hasModToken := false
	if newToken, ok := s.consumeTokenPreviewValue(modToken).(string); ok {
		tokenPreview = newToken
		hasModToken = newToken != ""
		if hasModToken {
			tokenDisplay = "РўРѕРєРµРЅ СЃРєСЂС‹С‚. РћР±РЅРѕРІРёС‚Рµ РµРіРѕ, С‡С‚РѕР±С‹ СЃРєРѕРїРёСЂРѕРІР°С‚СЊ РїРѕР»РЅС‹Р№ С‚РѕРєРµРЅ."
		}
	}
	certStatus, certNo, certAvail, _ := s.store.GetParticipantCertificateStatus(ctx, participant.ID)

	timerModAvailable := true
	personalNotifications, _ := s.store.ListPersonalNotifications(ctx, participant.ID, 30)

	return map[string]any{
		"TimerModAvailable":    timerModAvailable,
		"TournamentTitle":      s.cfg.TournamentTitle,
		"Claims":               claims,
		"IsAdmin":              claims.Role == "admin",
		"StreamerName":         profile["streamerName"],
		"AvatarURL":            profile["avatarUrl"],
		"BestTimeMS":           derefInt(participant.BestTimeMS),
		"MinecraftNick":        participant.MinecraftNick,
		"StreamOnline":         streamOnline,
		"ShowInNowPlaying":     participant.ShowInNowPlaying,
		"NowPlayingAuthorized": participant.NowPlayingAuthorized,
		"StreamTitle":          participant.StreamTitle,
		"StreamGameName":       participant.StreamGameName,
		"ParticipantStatus":    participant.Status,
		"ModTokenStatus":       s.modTokenPayload(modToken, ""),
		"TokenDisplay":         tokenDisplay,
		"TokenPreview":         tokenPreview,
		"HasModToken":          hasModToken,
		"SplitRows":            cabinetSplitRows(bestRun),
		"Achievements":         achievementSummary["Rows"],
		"AchievementSummary":   achievementSummary,
		"Profile":              profile,
		"CertificateAvailable": certAvail,
		"CertificateStatus":    certStatus,
		"CertificateNumber":    certNo,
		"Notifications":        personalNotifications,
	}, http.StatusOK, nil
}

func (s *Server) consumeTokenPreviewValue(token *store.ModAuthToken) any {
	if token == nil {
		return ""
	}
	return token.TokenPreview
}

func (s *Server) loadNowPlaying(ctx context.Context) ([]store.Participant, error) {
	participants, err := s.store.ListNowPlayingParticipants(ctx)
	if err != nil {
		return nil, err
	}
	if !s.cfg.EnableTwitchStreams || !s.cfg.TwitchEnabled() {
		return nil, nil
	}

	current := make([]store.Participant, 0, len(participants))
	for i := range participants {
		if strings.HasPrefix(participants[i].TwitchUserID, "pending:") {
			continue
		}
		if staleSync(participants[i].LastStreamStatusSyncAt, s.cfg.StreamStatusCacheTTL) {
			info, liveErr := s.twitch.GetStreamInfo(ctx, participants[i].TwitchUserID)
			if liveErr != nil {
				continue
			}
			if info == nil {
				participants[i].StreamOnline = false
				participants[i].StreamTitle = ""
				participants[i].StreamGameName = ""
				_ = s.store.SetStreamInfo(ctx, participants[i].TwitchUserID, false, "", "")
			} else {
				participants[i].StreamOnline = true
				participants[i].StreamTitle = info.Title
				participants[i].StreamGameName = info.GameName
				_ = s.store.SetStreamInfo(ctx, participants[i].TwitchUserID, true, info.Title, info.GameName)
			}
		}
		if participants[i].StreamOnline && isMinecraftCategory(participants[i].StreamGameName) {
			current = append(current, participants[i])
		}
	}
	return current, nil
}

func cabinetSplitRows(run *store.Run) []map[string]any {
	rows := []map[string]any{
		{"Label": "РћРІРµСЂ -> РќРµР·РµСЂ:", "Value": (*int)(nil)},
		{"Label": "РќРµР·РµСЂ -> Р­РЅРґ:", "Value": (*int)(nil)},
		{"Label": "РЈР±РёР№СЃС‚РІРѕ РґСЂР°РєРѕРЅР°:", "Value": (*int)(nil)},
	}
	if run == nil {
		return rows
	}
	rows[0]["Value"] = run.NetherSplitMS
	rows[1]["Value"] = run.EndSplitMS
	rows[2]["Value"] = &run.TimeMS
	return rows
}

func cabinetAchievementRows(achievements []store.AchievementProgress) []map[string]any {
	rows := make([]map[string]any, 0, len(achievements))
	for _, achievement := range achievements {
		rows = append(rows, map[string]any{
			"Name":        achievement.Name,
			"Description": achievement.Description,
			"Progress":    achievement.Progress,
			"Goal":        achievement.Goal,
			"Percent":     achievementPercent(achievement),
			"Unlocked":    achievement.Unlocked,
			"UnlockedAt":  achievement.UnlockedAt,
			"Slug":        achievement.Slug,
		})
	}
	return rows
}

func cabinetAchievementSummary(achievements []store.AchievementProgress) map[string]any {
	rows := cabinetAchievementRows(achievements)
	unlocked := make([]map[string]any, 0, len(rows))
	locked := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if row["Unlocked"] == true {
			unlocked = append(unlocked, row)
			continue
		}
		locked = append(locked, row)
	}

	total := len(rows)
	unlockedCount := len(unlocked)
	percent := 0
	if total > 0 {
		percent = unlockedCount * 100 / total
	}

	return map[string]any{
		"Rows":              rows,
		"Unlocked":          unlocked,
		"Locked":            locked,
		"UnlockedPreview":   limitAchievementRows(unlocked, 5),
		"LockedPreview":     limitAchievementRows(locked, 5),
		"UnlockedOverflow":  maxInt(unlockedCount-5, 0),
		"LockedOverflow":    maxInt(total-unlockedCount-5, 0),
		"UnlockedCount":     unlockedCount,
		"LockedCount":       total - unlockedCount,
		"TotalCount":        total,
		"Percent":           percent,
		"FeaturedUnlocked":  firstAchievementRow(unlocked),
		"FeaturedAvailable": unlockedCount > 0,
	}
}

func achievementPercent(achievement store.AchievementProgress) int {
	if achievement.Goal <= 0 {
		return 0
	}
	percent := achievement.Progress * 100 / achievement.Goal
	if percent > 100 {
		return 100
	}
	if percent < 0 {
		return 0
	}
	return percent
}

func limitAchievementRows(rows []map[string]any, limit int) []map[string]any {
	if len(rows) <= limit {
		return rows
	}
	return rows[:limit]
}

func firstAchievementRow(rows []map[string]any) map[string]any {
	if len(rows) == 0 {
		return nil
	}
	return rows[0]
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (s *Server) profilePayload(participant *store.Participant, claims session.Claims, streamOnline bool) map[string]any {
	avatarURL := participant.AvatarURL
	if avatarURL == "" {
		if participant.TwitchProfileImageURL != "" {
			avatarURL = participant.TwitchProfileImageURL
		} else {
			avatarURL = "/static/avatar-placeholder.svg"
		}
	}

	return map[string]any{
		"role":          "streamer",
		"twitchUserId":  participant.TwitchUserID,
		"twitchLogin":   claims.TwitchLogin,
		"streamerName":  firstNonEmpty(participant.TwitchDisplayName, participant.TwitchLogin),
		"minecraftNick": participant.MinecraftNick,
		"avatarUrl":     avatarURL,
		"bestTimeMs":    derefInt(participant.BestTimeMS),
		"streamOnline":  streamOnline,
	}
}

func (s *Server) homeAccountCard(ctx context.Context, r *http.Request) map[string]any {
	claims, ok := s.currentClaims(r)
	if !ok {
		return map[string]any{
			"Authenticated": false,
			"Label":         "Р’РѕР№С‚Рё РІ РєР°Р±РёРЅРµС‚",
			"Href":          "/login",
		}
	}

	avatarURL := "/static/avatar-placeholder.svg"
	displayName := claims.TwitchLogin
	participant, err := s.store.FindParticipantByTwitchUserID(ctx, claims.TwitchUserID)
	if err == nil && participant != nil {
		displayName = firstNonEmpty(participant.TwitchDisplayName, participant.TwitchLogin, claims.TwitchLogin)
		avatarURL = firstNonEmpty(participant.AvatarURL, participant.TwitchProfileImageURL, avatarURL)
	}

	return map[string]any{
		"Authenticated": true,
		"Label":         "РџРµСЂРµР№С‚Рё РІ РєР°Р±РёРЅРµС‚",
		"Href":          "/cabinet",
		"AvatarURL":     avatarURL,
		"DisplayName":   displayName,
	}
}

func (s *Server) modTokenPayload(token *store.ModAuthToken, rawToken string) map[string]any {
	payload := map[string]any{
		"configured": false,
	}
	if token != nil {
		payload["configured"] = true
		payload["tokenPreview"] = token.TokenPreview
		payload["createdAt"] = token.CreatedAt
		payload["updatedAt"] = token.UpdatedAt
		payload["expiresAt"] = token.UpdatedAt.Add(modTokenTTL)
		payload["lastUsedAt"] = token.LastUsedAt
	}
	if rawToken != "" {
		payload["modToken"] = rawToken
	}
	return payload
}

func (s *Server) rotateModToken(ctx context.Context, twitchUserID string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	participant, err := s.store.FindParticipantByTwitchUserID(ctx, twitchUserID)
	if err != nil {
		return "", err
	}
	if participant == nil {
		return "", errors.New("participant not found")
	}

	rawToken, err := randomUserToken("msrmod", 24)
	if err != nil {
		return "", err
	}
	if _, err := s.store.RotateModAuthToken(ctx, participant.ID, rawToken); err != nil {
		return "", err
	}
	return rawToken, nil
}

func (s *Server) authorizeModParticipant(ctx context.Context, rawToken string, requireRunReady bool) (*store.Participant, *store.ModAuthToken, bool, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return nil, nil, false, "invalid_mod_token", nil
	}

	participant, token, err := s.store.FindParticipantByModToken(ctx, rawToken)
	if err != nil {
		return nil, nil, false, "", err
	}
	if !participantCanAccessCabinet(participant) || token == nil {
		return nil, nil, false, "not_whitelisted", nil
	}
	if time.Since(token.UpdatedAt) > modTokenTTL {
		return nil, nil, false, "mod_token_expired", nil
	}

	if !requireRunReady {
		return participant, token, participant.StreamOnline, "", nil
	}

	running, err := s.store.IsTournamentRunning(ctx)
	if err != nil {
		return nil, nil, false, "", err
	}
	if !running {
		return nil, nil, false, "tournament_not_running", nil
	}

	streamOnline, reason, err := s.validateParticipantStreamReady(ctx, participant)
	if err != nil {
		return nil, nil, false, "", err
	}
	if reason != "" {
		return nil, nil, streamOnline, reason, nil
	}

	return participant, token, streamOnline, "", nil
}

func (s *Server) validateParticipantStreamReady(ctx context.Context, participant *store.Participant) (bool, string, error) {
	streamOnline := participant.StreamOnline
	streamTitle := participant.StreamTitle
	streamGameName := participant.StreamGameName
	if s.cfg.EnableTwitchStreams && s.cfg.TwitchEnabled() {
		if streamInfo, liveErr := s.twitch.GetStreamInfo(ctx, participant.TwitchUserID); liveErr == nil {
			streamOnline = streamInfo != nil
			title := ""
			gameName := ""
			if streamInfo != nil {
				title = streamInfo.Title
				gameName = streamInfo.GameName
			}
			streamTitle = title
			streamGameName = gameName
			_ = s.store.SetStreamInfo(ctx, participant.TwitchUserID, streamOnline, title, gameName)
		} else {
			return false, "stream_check_unavailable", nil
		}
	}
	if s.cfg.EnableTwitchStreams && !streamOnline {
		return streamOnline, "stream_offline", nil
	}
	if s.cfg.EnableTwitchStreams && s.cfg.TwitchEnabled() && !isMinecraftCategory(streamGameName) && !isMinecraftCategory(streamTitle) {
		return streamOnline, "stream_not_minecraft", nil
	}

	return streamOnline, "", nil
}

func (s *Server) updateAvatar(ctx context.Context, twitchUserID string, r *http.Request) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if err := r.ParseMultipartForm(s.cfg.MaxAvatarBytes); err != nil {
		return "", badRequest("invalid multipart form")
	}
	file, header, err := r.FormFile("avatar")
	if err != nil {
		return "", badRequest("avatar file is required")
	}
	defer file.Close()

	participant, err := s.store.FindParticipantByTwitchUserID(ctx, twitchUserID)
	if err != nil {
		return "", internalError("failed to load participant")
	}
	if participant == nil {
		return "", badRequest("participant not found")
	}

	contents, ext, err := readValidatedImage(file, header, s.cfg.MaxAvatarBytes)
	if err != nil {
		return "", err
	}

	filename := fmt.Sprintf("avatar_%s_%d%s", participant.ID, time.Now().UnixNano(), ext)
	absolute := filepath.Join(s.uploadDir, filename)
	if err := os.WriteFile(absolute, contents, 0o644); err != nil {
		return "", internalError("failed to store avatar")
	}

	if participant.AvatarURL != "" && strings.HasPrefix(participant.AvatarURL, "/uploads/") {
		_ = os.Remove(filepath.Join(s.uploadDir, filepath.Base(participant.AvatarURL)))
	}

	avatarURL := "/uploads/" + filename
	if err := s.store.UpdateAvatar(ctx, participant.ID, avatarURL); err != nil {
		return "", internalError("failed to update avatar")
	}

	return avatarURL, nil
}

func (s *Server) decodeAndVerifyModJSON(r *http.Request, out any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 256*1024))
	if err != nil {
		return badRequest("invalid body")
	}

	var generic any
	if err := json.Unmarshal(body, &generic); err != nil {
		return badRequest("invalid payload")
	}
	if err := json.Unmarshal(body, out); err != nil {
		return badRequest("invalid payload")
	}

	timestampRaw := r.Header.Get("X-Mod-Timestamp")
	nonce := strings.TrimSpace(r.Header.Get("X-Mod-Nonce"))
	signature := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Mod-Signature")))
	modKey := r.Header.Get("X-Mod-Key")
	if modKey == "" && timestampRaw == "" && nonce == "" && signature == "" {
		return nil
	}
	if modKey != s.cfg.ModAPIKey {
		return unauthorized("invalid mod key")
	}
	if timestampRaw == "" || nonce == "" || signature == "" {
		return unauthorized("missing mod auth headers")
	}
	tsInt, parseErr := parseInt64(timestampRaw)
	if parseErr != nil {
		return badRequest("invalid timestamp header")
	}
	now := time.Now().Unix()
	if delta := now - tsInt; delta > int64(s.cfg.ModMaxSkew.Seconds()) || delta < -int64(s.cfg.ModMaxSkew.Seconds()) {
		return unauthorized("expired request timestamp")
	}
	if len(nonce) < 12 || len(nonce) > 128 {
		return badRequest("invalid nonce")
	}

	payload := timestampRaw + "." + nonce + "." + stableJSON(generic)
	expected := signHex(s.cfg.ModSigningSecret, payload)
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return unauthorized("invalid signature")
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := s.store.InsertModNonce(ctx, nonce); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "unique") {
			return unauthorized("replay request detected")
		}
		return internalError("failed to store nonce")
	}

	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeAPIError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message})
}

func decodeJSON(r *http.Request, out any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 256*1024))
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return errEmptyBody
	}
	return json.Unmarshal(body, out)
}

func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func randomState() string {
	buf := make([]byte, 24)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

func randomUserToken(prefix string, bytesLen int) (string, error) {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(buf), nil
}

func (s *Server) setFlashValue(w http.ResponseWriter, name, value string, maxAgeSec int) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.AuthCookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAgeSec,
	})
}

func (s *Server) consumeFlashValue(w http.ResponseWriter, r *http.Request, name string) string {
	cookie, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.AuthCookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
	return cookie.Value
}

func formatDuration(value any) string {
	var ms int
	switch current := value.(type) {
	case int:
		ms = current
	case int64:
		ms = int(current)
	case *int:
		if current == nil {
			return "-"
		}
		ms = *current
	default:
		return "-"
	}
	if ms <= 0 {
		return "-"
	}
	seconds := ms / 1000
	totalMinutes := seconds / 60
	hours := totalMinutes / 60
	minutes := totalMinutes % 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d.%03d", hours, minutes, seconds%60, ms%1000)
	}
	return fmt.Sprintf("%02d:%02d.%03d", minutes, seconds%60, ms%1000)
}

var mskLoc = time.FixedZone("MSK", 3*60*60)

func formatDateTime(value any) string {
	switch current := value.(type) {
	case time.Time:
		return current.In(mskLoc).Format("02.01.2006 15:04") + " РњРЎРљ"
	case *time.Time:
		if current == nil {
			return ""
		}
		return current.In(mskLoc).Format("02.01.2006 15:04") + " РњРЎРљ"
	default:
		return ""
	}
}

func formatShortDateTime(value any) string {
	switch current := value.(type) {
	case time.Time:
		return current.In(mskLoc).Format("02.01 15:04") + " РњРЎРљ"
	case *time.Time:
		if current == nil {
			return ""
		}
		return current.In(mskLoc).Format("02.01 15:04") + " РњРЎРљ"
	default:
		return ""
	}
}

func formatDate(value any) string {
	switch current := value.(type) {
	case time.Time:
		return current.In(mskLoc).Format("Jan 2, 2006")
	case *time.Time:
		if current == nil {
			return "-"
		}
		return current.In(mskLoc).Format("Jan 2, 2006")
	default:
		return "-"
	}
}

func validTwitchLogin(login string) bool {
	if len(login) < 2 || len(login) > 25 {
		return false
	}
	for _, r := range login {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}

func validMinecraftNick(nick string) bool {
	if len(nick) < 3 || len(nick) > 16 {
		return false
	}
	for _, r := range nick {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}

func validParticipantStatus(status string) bool {
	return status == "invited" || status == "active" || status == "blocked"
}

func validTournamentState(state string) bool {
	return state == "scheduled" || state == "running" || state == "finished"
}

func hasScope(scopes []string, required string) bool {
	for _, scope := range scopes {
		if strings.EqualFold(scope, required) {
			return true
		}
	}
	return false
}

func isMinecraftCategory(category string) bool {
	normalized := strings.ToLower(strings.TrimSpace(category))
	return strings.Contains(normalized, "minecraft") || strings.Contains(normalized, "РјР°Р№РЅРєСЂР°С„С‚")
}

func validRunWindow(timeMS int, startedAt, finishedAt time.Time) bool {
	if timeMS <= 0 || finishedAt.Before(startedAt) {
		return false
	}
	diff := finishedAt.Sub(startedAt)
	if diff < 0 {
		return false
	}
	// With IGT, total game time (timeMS) should be less than or equal to total real time (diff)
	// (plus a small tolerance of 10s for loading/saving latencies).
	if time.Duration(timeMS)*time.Millisecond > diff+10*time.Second {
		return false
	}
	return true
}

func validRunSplits(timeMS int, netherSplitMS, endSplitMS *int) bool {
	if timeMS <= 0 {
		return false
	}
	if netherSplitMS != nil && (*netherSplitMS <= 0 || *netherSplitMS >= timeMS) {
		return false
	}
	if endSplitMS != nil && (*endSplitMS <= 0 || *endSplitMS >= timeMS) {
		return false
	}
	if netherSplitMS != nil && endSplitMS != nil && *endSplitMS <= *netherSplitMS {
		return false
	}
	return true
}

func staleSync(last *time.Time, ttl time.Duration) bool {
	if last == nil {
		return true
	}
	return time.Since(*last) >= ttl
}

func flashMessage(code string) string {
	switch code {
	case "logged_out":
		return "РЎРµСЃСЃРёСЏ Р·Р°РІРµСЂС€РµРЅР°."
	case "profile_saved":
		return "Minecraft nick СЃРѕС…СЂР°РЅРµРЅ."
	case "mod_token_rotated":
		return "РўРѕРєРµРЅ РјРѕРґР° РѕР±РЅРѕРІР»РµРЅ. РЎРєРѕРїРёСЂСѓР№С‚Рµ РµРіРѕ СЃРµР№С‡Р°СЃ: РїРѕР·Р¶Рµ РїРѕР»РЅС‹Р№ С‚РѕРєРµРЅ Р±РѕР»СЊС€Рµ РЅРµ РїРѕРєР°Р·С‹РІР°РµС‚СЃСЏ."
	case "now_playing_saved":
		return "РќР°СЃС‚СЂРѕР№РєР° РЎРµР№С‡Р°СЃ РёРіСЂР°СЋС‚ СЃРѕС…СЂР°РЅРµРЅР°."
	case "now_playing_authorized":
		return "РџСЂР°РІР° Twitch РґР»СЏ РЎРµР№С‡Р°СЃ РёРіСЂР°СЋС‚ РїРѕРґС‚РІРµСЂР¶РґРµРЅС‹."
	case "avatar_saved":
		return "РђРІР°С‚Р°СЂ РѕР±РЅРѕРІР»РµРЅ."
	case "login_required":
		return "РќСѓР¶РЅРѕ Р°РІС‚РѕСЂРёР·РѕРІР°С‚СЊСЃСЏ."
	case "oauth_state":
		return "РЎРµСЃСЃРёСЏ Р°РІС‚РѕСЂРёР·Р°С†РёРё СѓСЃС‚Р°СЂРµР»Р°. РџРѕРїСЂРѕР±СѓР№С‚Рµ СЃРЅРѕРІР°."
	case "oauth_failed":
		return "РђРІС‚РѕСЂРёР·Р°С†РёСЏ С‡РµСЂРµР· Twitch Р·Р°РІРµСЂС€РёР»Р°СЃСЊ РѕС€РёР±РєРѕР№."
	case "not_whitelisted":
		return "Р’Р°С€ Р°РєРєР°СѓРЅС‚ РЅРµ РЅР°С…РѕРґРёС‚СЃСЏ РІ whitelist."
	case "mock_disabled":
		return "Dev mock Р°РІС‚РѕСЂРёР·Р°С†РёСЏ РѕС‚РєР»СЋС‡РµРЅР°."
	case "no_whitelisted_streamer":
		return "РќРµ РЅР°Р№РґРµРЅ РЅРё РѕРґРёРЅ whitelisted СЃС‚СЂРёРјРµСЂ."
	case "invalid_minecraft_nick":
		return "Minecraft nick РґРѕР»Р¶РµРЅ СЃРѕРґРµСЂР¶Р°С‚СЊ 3-16 Р»Р°С‚РёРЅСЃРєРёС… СЃРёРјРІРѕР»РѕРІ, С†РёС„СЂ РёР»Рё _. "
	case "profile_update_failed":
		return "РќРµ СѓРґР°Р»РѕСЃСЊ СЃРѕС…СЂР°РЅРёС‚СЊ РїСЂРѕС„РёР»СЊ."
	case "now_playing_permission_missing":
		return "РќСѓР¶РЅРѕ РґР°С‚СЊ РґРѕСЃС‚СѓРї Twitch РґР»СЏ РЎРµР№С‡Р°СЃ РёРіСЂР°СЋС‚."
	case "invalid_image":
		return "Р”РѕРїСѓСЃС‚РёРјС‹ С‚РѕР»СЊРєРѕ PNG, JPEG, GIF Рё WEBP."
	case "invalid_form":
		return "РќРµРІРµСЂРЅС‹Р№ С„РѕСЂРјР°С‚ С„РѕСЂРјС‹."
	case "invalid_application":
		return "РџСЂРѕРІРµСЂСЊ Discord, Twitch-РєР°РЅР°Р», С‡Р°СЃРѕРІРѕР№ РїРѕСЏСЃ Рё РїРѕРґС‚РІРµСЂР¶РґРµРЅРёРµ СЃС‚СЂРёРјР°."
	case "application_failed":
		return "РќРµ СѓРґР°Р»РѕСЃСЊ РѕС‚РїСЂР°РІРёС‚СЊ Р·Р°СЏРІРєСѓ. РџРѕРїСЂРѕР±СѓР№С‚Рµ РµС‰Рµ СЂР°Р·."
	case "applications_closed":
		return "РџСЂРёРµРј Р·Р°СЏРІРѕРє Р·Р°РєСЂС‹С‚. Р—Р°СЏРІРєРё Р±РѕР»СЊС€Рµ РїРѕРґР°РІР°С‚СЊ РЅРµР»СЊР·СЏ."
	case "file_too_large":
		return "Р¤Р°Р№Р» СЃР»РёС€РєРѕРј Р±РѕР»СЊС€РѕР№."
	case "too_many_requests":
		return "РЎР»РёС€РєРѕРј РјРЅРѕРіРѕ Р·Р°РїСЂРѕСЃРѕРІ. РџРѕРїСЂРѕР±СѓР№С‚Рµ РїРѕР·Р¶Рµ."
	default:
		return ""
	}
}

func flashCode(err error) string {
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "too large"):
		return "file_too_large"
	case strings.Contains(message, "image"):
		return "invalid_image"
	default:
		return "invalid_form"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func derefInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

type httpError struct {
	status  int
	message string
}

func (e httpError) Error() string { return e.message }

func badRequest(message string) error {
	return httpError{status: http.StatusBadRequest, message: message}
}
func unauthorized(message string) error {
	return httpError{status: http.StatusUnauthorized, message: message}
}
func internalError(message string) error {
	return httpError{status: http.StatusInternalServerError, message: message}
}

func errorStatus(err error) int {
	var httpErr httpError
	if errors.As(err, &httpErr) {
		return httpErr.status
	}
	return http.StatusInternalServerError
}

func parseInt64(value string) (int64, error) {
	var out int64
	_, err := fmt.Sscan(value, &out)
	return out, err
}

func signHex(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func stableJSON(value any) string {
	switch current := value.(type) {
	case nil:
		return "null"
	case map[string]any:
		keys := make([]string, 0, len(current))
		for key := range current {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, fmt.Sprintf("%s:%s", mustJSON(key), stableJSON(current[key])))
		}
		return "{" + strings.Join(parts, ",") + "}"
	case []any:
		parts := make([]string, 0, len(current))
		for _, item := range current {
			parts = append(parts, stableJSON(item))
		}
		return "[" + strings.Join(parts, ",") + "]"
	default:
		return mustJSON(current)
	}
}

func mustJSON(value any) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

func readValidatedImage(file multipart.File, header *multipart.FileHeader, limit int64) ([]byte, string, error) {
	if header.Size > limit {
		return nil, "", badRequest("file too large")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, "", badRequest("failed to read image")
	}
	if int64(len(data)) > limit {
		return nil, "", badRequest("file too large")
	}

	switch {
	case len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}):
		return data, ".png", nil
	case len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff:
		return data, ".jpg", nil
	case len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a"):
		return data, ".gif", nil
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return data, ".webp", nil
	default:
		return nil, "", badRequest("unsupported image type")
	}
}
