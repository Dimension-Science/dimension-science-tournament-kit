package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func (s *Server) handleAdminDiscordApplications(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireAdminAPI(w, r); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	applications, err := s.store.ListDiscordApplications(ctx)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load Discord applications")
		return
	}
	writeJSON(w, http.StatusOK, applications)
}

func (s *Server) handleAdminDiscordApplicationApprove(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.requireAdminAPI(w, r)
	if !ok {
		return
	}
	s.reviewDiscordApplicationAPI(w, r, "approved", "web:"+claims.TwitchLogin, "")
}

func (s *Server) handleAdminDiscordApplicationReject(w http.ResponseWriter, r *http.Request) {
	claims, ok := s.requireAdminAPI(w, r)
	if !ok {
		return
	}
	var payload struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || strings.TrimSpace(payload.Reason) == "" {
		writeAPIError(w, http.StatusBadRequest, "rejection reason is required")
		return
	}
	s.reviewDiscordApplicationAPI(w, r, "rejected", "web:"+claims.TwitchLogin, payload.Reason)
}

func (s *Server) reviewDiscordApplicationAPI(w http.ResponseWriter, r *http.Request, status, actor, reason string) {
	if s.discord == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "Discord bot is disabled")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid application id")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	application, err := s.discord.ReviewApplicationFromAdmin(ctx, s.store, id, status, actor, reason)
	if err != nil {
		writeAPIError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, application)
}
