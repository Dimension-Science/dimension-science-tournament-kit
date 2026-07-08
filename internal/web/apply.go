package web

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dimension-science/tournament-kit/internal/store"
)

func (s *Server) handleApplyPage(w http.ResponseWriter, r *http.Request) {
	referral := cleanReferral(r.URL.Query().Get("ref"))
	twitchLogin := cleanTwitchLogin(r.URL.Query().Get("twitch"))

	var participant *store.Participant
	if claims, authenticated := s.currentClaims(r); authenticated {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		participant, _ = s.store.FindParticipantByTwitchUserID(ctx, claims.TwitchUserID)
		if twitchLogin == "" {
			twitchLogin = cleanTwitchLogin(claims.TwitchLogin)
		}
	}

	status := strings.TrimSpace(r.URL.Query().Get("status"))
	applicationNumber, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("application")), 10, 64)
	statusTitle := ""
	statusText := ""
	switch status {
	case "pending":
		statusTitle = "Р—Р°СЏРІРєР° РЅР° РїСЂРѕРІРµСЂРєРµ"
		statusText = "РЇ СѓРІРёР¶Сѓ РµРµ РІ Р°РґРјРёРЅРєРµ Рё РѕРґРѕР±СЂСЋ, РµСЃР»Рё РІСЃРµ РїРѕРґС…РѕРґРёС‚ РїРѕРґ РїСЂР°РІРёР»Р° С‚СѓСЂРЅРёСЂР°."
	case "approved":
		statusTitle = "Р—Р°СЏРІРєР° СѓР¶Рµ РѕРґРѕР±СЂРµРЅР°"
		statusText = "РўРµРїРµСЂСЊ РјРѕР¶РЅРѕ РІРѕР№С‚Рё РІ Р»РёС‡РЅС‹Р№ РєР°Р±РёРЅРµС‚ С‡РµСЂРµР· Twitch. РўРѕРєРµРЅ РґР»СЏ РјРѕРґР° Р±СѓРґРµС‚ РЅСѓР¶РµРЅ 5 РёСЋРЅСЏ, РєРѕРіРґР° РЅР° СЃР°Р№С‚Рµ РїРѕСЏРІРёС‚СЃСЏ РѕС„РёС†РёР°Р»СЊРЅС‹Р№ РјРѕРґ."
	case "rejected":
		statusTitle = "Р—Р°СЏРІРєР° РѕС‚РєР»РѕРЅРµРЅР°"
		statusText = "РњРѕР¶РЅРѕ РёСЃРїСЂР°РІРёС‚СЊ РґР°РЅРЅС‹Рµ Рё РѕС‚РїСЂР°РІРёС‚СЊ Р·Р°СЏРІРєСѓ Р·Р°РЅРѕРІРѕ."
	}

	canOpenCabinet := participantCanAccessCabinet(participant)
	if canOpenCabinet && statusTitle == "" {
		status = "approved"
		statusTitle = "Р’С‹ СѓР¶Рµ РІ whitelist"
		statusText = "Р›РёС‡РЅС‹Р№ РєР°Р±РёРЅРµС‚ РѕС‚РєСЂС‹С‚: С‚Р°Рј РїСЂРѕС„РёР»СЊ, РґРѕСЃС‚РёР¶РµРЅРёСЏ Рё С‚РѕРєРµРЅ РґР»СЏ РѕС„РёС†РёР°Р»СЊРЅРѕРіРѕ РјРѕРґР°. Р”Рѕ 5 РёСЋРЅСЏ С‚РѕРєРµРЅ РЅРёРєСѓРґР° РІСЃС‚Р°РІР»СЏС‚СЊ РЅРµ РЅСѓР¶РЅРѕ."
	}

	_ = s.renderTemplate(w, "apply.html", map[string]any{
		"TournamentTitle":   s.cfg.TournamentTitle,
		"Referral":          referral,
		"TwitchLogin":       twitchLogin,
		"TwitchChannelURL":  twitchChannelForLogin(twitchLogin),
		"ApplicationStatus": status,
		"ApplicationNumber": applicationNumber,
		"StatusTitle":       statusTitle,
		"StatusText":        statusText,
		"CanOpenCabinet":    canOpenCabinet,
		"Error":             flashMessage(r.URL.Query().Get("error")),
	})
}

func (s *Server) handleApplyForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, applyRedirectPath("", "", "invalid_form"), http.StatusSeeOther)
		return
	}

	referral := cleanReferral(r.FormValue("ref"))
	twitchLogin := cleanTwitchLogin(r.FormValue("twitch_login"))
	discordUsername := strings.TrimSpace(r.FormValue("discord_username"))
	timezone := strings.TrimSpace(r.FormValue("timezone"))
	twitchChannelURL := normalizeTwitchChannelURL(r.FormValue("twitch_channel_url"), twitchLogin)
	understandsStreamRequired := r.FormValue("understands_stream_required") == "1"

	if twitchLogin == "" || discordUsername == "" || len(discordUsername) > 80 || timezone == "" || len(timezone) > 80 || twitchChannelURL == "" || !understandsStreamRequired {
		http.Redirect(w, r, applyRedirectPath(referral, twitchLogin, "invalid_application"), http.StatusSeeOther)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	closed, err := s.store.AreApplicationsClosed(ctx)
	if err != nil {
		http.Redirect(w, r, applyRedirectPath(referral, twitchLogin, "application_failed"), http.StatusSeeOther)
		return
	}
	if closed {
		http.Redirect(w, r, applyRedirectPath(referral, twitchLogin, "applications_closed"), http.StatusSeeOther)
		return
	}

	participant, err := s.store.FindWhitelistedParticipant(ctx, "", twitchLogin)
	if err != nil {
		http.Redirect(w, r, applyRedirectPath(referral, twitchLogin, "application_failed"), http.StatusSeeOther)
		return
	}
	if participantCanAccessCabinet(participant) {
		http.Redirect(w, r, applyStatusPath(referral, twitchLogin, "approved"), http.StatusSeeOther)
		return
	}

	application, err := s.store.UpsertTournamentApplication(ctx, store.TournamentApplicationInput{
		TwitchUserID:              pendingTwitchUserID(twitchLogin),
		TwitchLogin:               twitchLogin,
		TwitchDisplayName:         twitchLogin,
		TwitchChannelURL:          twitchChannelURL,
		DiscordUsername:           discordUsername,
		Timezone:                  timezone,
		Referral:                  referral,
		UnderstandsStreamRequired: understandsStreamRequired,
	})
	if err != nil {
		http.Redirect(w, r, applyRedirectPath(referral, twitchLogin, "application_failed"), http.StatusSeeOther)
		return
	}
	applicationNumber := ""
	if application != nil && application.ApplicationNumber > 0 {
		applicationNumber = strconv.FormatInt(application.ApplicationNumber, 10)
	}
	s.notifyDiscordApplicationSubmitted(r.Context(), application)
	http.Redirect(w, r, applyStatusPath(referral, twitchLogin, "pending", applicationNumber), http.StatusSeeOther)
}

func (s *Server) handleAdminApplications(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	applications, err := s.store.ListTournamentApplications(ctx)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load applications")
		return
	}
	writeJSON(w, http.StatusOK, applications)
}

func (s *Server) handleAdminApplicationApprove(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	applicationID := strings.TrimSpace(r.PathValue("id"))
	if applicationID == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid application id")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	application, err := s.store.FindTournamentApplicationByID(ctx, applicationID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load application")
		return
	}
	if application == nil {
		writeAPIError(w, http.StatusNotFound, "application not found")
		return
	}

	participant, err := s.store.UpsertWhitelistLogin(ctx, strings.ToLower(application.TwitchLogin))
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to whitelist applicant")
		return
	}
	updated, err := s.store.UpdateTournamentApplicationStatus(ctx, application.ID, "approved", participant.ID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to approve application")
		return
	}
	s.notifyDiscordApplicationReviewed(r.Context(), updated)
	writeJSON(w, http.StatusOK, map[string]any{"application": updated, "participant": participant})
}

func (s *Server) handleAdminApplicationReject(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	applicationID := strings.TrimSpace(r.PathValue("id"))
	if applicationID == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid application id")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	application, err := s.store.UpdateTournamentApplicationStatus(ctx, applicationID, "rejected", "")
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to reject application")
		return
	}
	s.notifyDiscordApplicationReviewed(r.Context(), application)
	writeJSON(w, http.StatusOK, application)
}

func (s *Server) notifyDiscordApplicationSubmitted(ctx context.Context, application *store.TournamentApplication) {
	if s.discord == nil || application == nil {
		return
	}
	notifyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	s.discord.NotifyApplicationSubmitted(notifyCtx, application)
}

func (s *Server) notifyDiscordApplicationReviewed(ctx context.Context, application *store.TournamentApplication) {
	if s.discord == nil || application == nil {
		return
	}
	notifyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	s.discord.NotifyApplicationReviewed(notifyCtx, application)
	log.Printf("discord application notification queued for #%d", application.ApplicationNumber)
}

func cleanReferral(value string) string {
	value = cleanTwitchLogin(value)
	if !validTwitchLogin(value) {
		return ""
	}
	return value
}

func cleanTwitchLogin(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimPrefix(value, "@")
	value = strings.TrimPrefix(value, "https://www.twitch.tv/")
	value = strings.TrimPrefix(value, "https://twitch.tv/")
	value = strings.Trim(value, "/")
	if !validTwitchLogin(value) {
		return ""
	}
	return value
}

func pendingTwitchUserID(login string) string {
	return "pending:" + cleanTwitchLogin(login)
}

func twitchChannelForLogin(login string) string {
	login = cleanTwitchLogin(login)
	if login == "" {
		return ""
	}
	return "https://www.twitch.tv/" + login
}

func normalizeTwitchChannelURL(rawValue, fallbackLogin string) string {
	rawValue = strings.TrimSpace(rawValue)
	if rawValue == "" {
		return twitchChannelForLogin(fallbackLogin)
	}
	if !strings.Contains(rawValue, "://") {
		rawValue = "https://" + rawValue
	}
	parsed, err := url.Parse(rawValue)
	if err != nil || parsed.Host == "" {
		return ""
	}
	host := strings.ToLower(strings.TrimPrefix(parsed.Host, "www."))
	if host != "twitch.tv" {
		return ""
	}
	login := cleanTwitchLogin(strings.Trim(strings.TrimPrefix(parsed.Path, "/"), "/"))
	if login == "" {
		return ""
	}
	return twitchChannelForLogin(login)
}

func applyRedirectPath(referral string, parts ...string) string {
	twitchLogin := ""
	code := ""
	if len(parts) == 1 {
		code = parts[0]
	} else if len(parts) >= 2 {
		twitchLogin = cleanTwitchLogin(parts[0])
		code = parts[1]
	}

	path := "/apply"
	values := url.Values{}
	if referral != "" {
		values.Set("ref", referral)
	}
	if twitchLogin != "" {
		values.Set("twitch", twitchLogin)
	}
	if code != "" {
		values.Set("error", code)
	}
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return path
}

func applyStatusPath(referral string, parts ...string) string {
	twitchLogin := ""
	status := "pending"
	applicationNumber := ""
	if len(parts) == 1 {
		status = parts[0]
	} else if len(parts) >= 2 {
		twitchLogin = cleanTwitchLogin(parts[0])
		status = parts[1]
		if len(parts) >= 3 {
			applicationNumber = strings.TrimSpace(parts[2])
		}
	}

	path := "/apply"
	values := url.Values{}
	if referral != "" {
		values.Set("ref", referral)
	}
	if twitchLogin != "" {
		values.Set("twitch", twitchLogin)
	}
	if applicationNumber != "" {
		values.Set("application", applicationNumber)
	}
	values.Set("status", status)
	return path + "?" + values.Encode()
}
