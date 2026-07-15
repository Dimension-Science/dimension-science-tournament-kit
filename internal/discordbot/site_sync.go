package discordbot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dimension-science/tournament-kit/internal/store"
)

type siteApplicationPayload struct {
	ExternalID      string `json:"externalId"`
	CampaignSlug    string `json:"campaignSlug"`
	Status          string `json:"status"`
	DiscordUserID   string `json:"discordUserId"`
	DiscordUsername string `json:"discordUsername"`
	GameNick        string `json:"gameNick"`
	Timezone        string `json:"timezone"`
	Experience      string `json:"experience"`
	Motivation      string `json:"motivation"`
	Links           string `json:"links"`
	ReviewReason    string `json:"reviewReason,omitempty"`
}

func (c *Client) syncSiteApplication(ctx context.Context, app *store.DiscordApplication) error {
	if c == nil || app == nil || c.siteBaseURL == "" || c.siteInternalToken == "" {
		return nil
	}
	status := app.Status
	switch status {
	case "pending", "needs_info":
		status = "new"
	case "approved":
		status = "accepted"
	case "rejected":
		status = "rejected"
	default:
		status = "reviewing"
	}
	payload, err := json.Marshal(siteApplicationPayload{
		ExternalID: app.ID, CampaignSlug: c.siteCampaignSlug, Status: status,
		DiscordUserID: app.DiscordUserID, DiscordUsername: app.DiscordUsername,
		GameNick: app.GameNick, Timezone: app.Timezone, Experience: app.Experience,
		Motivation: app.Motivation, Links: app.Links, ReviewReason: app.ReviewReason,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.siteBaseURL+"/api/internal/bot/applications", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.siteInternalToken)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("site sync status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (c *Client) syncSiteApplicationAsync(app *store.DiscordApplication) {
	if c == nil || app == nil || c.siteBaseURL == "" || c.siteInternalToken == "" {
		return
	}
	go func() {
		if err := c.syncSiteApplication(context.Background(), app); err != nil && c.logger != nil {
			c.logger.Printf("Dimension Science site application sync: %v", err)
		}
	}()
}
