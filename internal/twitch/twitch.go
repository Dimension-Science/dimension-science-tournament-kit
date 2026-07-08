package twitch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/dimension-science/tournament-kit/internal/config"
)

type Client struct {
	cfg        config.Config
	httpClient *http.Client

	mu          sync.Mutex
	appToken    string
	appTokenExp time.Time
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

type ValidateResponse struct {
	ClientID  string   `json:"client_id"`
	Login     string   `json:"login"`
	UserID    string   `json:"user_id"`
	Scopes    []string `json:"scopes"`
	ExpiresIn int      `json:"expires_in"`
}

type User struct {
	ID              string `json:"id"`
	Login           string `json:"login"`
	DisplayName     string `json:"display_name"`
	ProfileImageURL string `json:"profile_image_url"`
}

type StreamInfo struct {
	ID       string `json:"id"`
	UserID   string `json:"user_id"`
	UserName string `json:"user_name"`
	GameName string `json:"game_name"`
	Title    string `json:"title"`
}

func NewClient(cfg config.Config) *Client {
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *Client) Enabled() bool {
	return c.cfg.TwitchEnabled()
}

func (c *Client) AuthorizeURL(state string, scopes ...string) string {
	return c.AuthorizeURLWithRedirectURI(c.cfg.TwitchRedirectURI, state, scopes...)
}

func (c *Client) AuthorizeURLWithRedirectURI(redirectURI, state string, scopes ...string) string {
	redirectURI = strings.TrimSpace(redirectURI)
	if redirectURI == "" {
		redirectURI = c.cfg.TwitchRedirectURI
	}
	query := url.Values{}
	query.Set("client_id", c.cfg.TwitchClientID)
	query.Set("redirect_uri", redirectURI)
	query.Set("response_type", "code")
	query.Set("scope", strings.Join(scopes, " "))
	query.Set("state", state)
	if c.cfg.TwitchForceVerify {
		query.Set("force_verify", "true")
	}

	return "https://id.twitch.tv/oauth2/authorize?" + query.Encode()
}

func (c *Client) ExchangeCode(ctx context.Context, code string) (*TokenResponse, error) {
	return c.ExchangeCodeWithRedirectURI(ctx, code, c.cfg.TwitchRedirectURI)
}

func (c *Client) ExchangeCodeWithRedirectURI(ctx context.Context, code, redirectURI string) (*TokenResponse, error) {
	if !c.Enabled() {
		return nil, errors.New("twitch oauth is not configured")
	}
	redirectURI = strings.TrimSpace(redirectURI)
	if redirectURI == "" {
		redirectURI = c.cfg.TwitchRedirectURI
	}

	form := url.Values{}
	form.Set("client_id", c.cfg.TwitchClientID)
	form.Set("client_secret", c.cfg.TwitchClientSecret)
	form.Set("code", code)
	form.Set("grant_type", "authorization_code")
	form.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://id.twitch.tv/oauth2/token", bytes.NewBufferString(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var token TokenResponse
	if err := c.doJSON(req, &token); err != nil {
		return nil, err
	}
	return &token, nil
}

func (c *Client) ValidateUserToken(ctx context.Context, accessToken string) (*ValidateResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://id.twitch.tv/oauth2/validate", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "OAuth "+accessToken)

	var response ValidateResponse
	if err := c.doJSON(req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

func (c *Client) GetUser(ctx context.Context, accessToken, userID string) (*User, error) {
	u, _ := url.Parse("https://api.twitch.tv/helix/users")
	values := u.Query()
	values.Set("id", userID)
	u.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Client-Id", c.cfg.TwitchClientID)

	var payload struct {
		Data []User `json:"data"`
	}
	if err := c.doJSON(req, &payload); err != nil {
		return nil, err
	}
	if len(payload.Data) == 0 {
		return nil, errors.New("twitch user not found")
	}
	return &payload.Data[0], nil
}

func (c *Client) IsStreamerLive(ctx context.Context, userID string) (bool, error) {
	info, err := c.GetStreamInfo(ctx, userID)
	return info != nil, err
}

func (c *Client) GetStreamInfo(ctx context.Context, userID string) (*StreamInfo, error) {
	if !c.cfg.EnableTwitchStreams || !c.Enabled() {
		return nil, nil
	}

	appToken, err := c.appAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	u, _ := url.Parse("https://api.twitch.tv/helix/streams")
	values := u.Query()
	values.Set("user_id", userID)
	u.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+appToken)
	req.Header.Set("Client-Id", c.cfg.TwitchClientID)

	var payload struct {
		Data []StreamInfo `json:"data"`
	}
	if err := c.doJSON(req, &payload); err != nil {
		return nil, err
	}
	if len(payload.Data) == 0 {
		return nil, nil
	}

	return &payload.Data[0], nil
}

func (c *Client) appAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.appToken != "" && time.Until(c.appTokenExp) > 30*time.Second {
		token := c.appToken
		c.mu.Unlock()
		return token, nil
	}
	c.mu.Unlock()

	form := url.Values{}
	form.Set("client_id", c.cfg.TwitchClientID)
	form.Set("client_secret", c.cfg.TwitchClientSecret)
	form.Set("grant_type", "client_credentials")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://id.twitch.tv/oauth2/token", bytes.NewBufferString(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var token TokenResponse
	if err := c.doJSON(req, &token); err != nil {
		return "", err
	}

	c.mu.Lock()
	c.appToken = token.AccessToken
	c.appTokenExp = time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	c.mu.Unlock()

	return token.AccessToken, nil
}

func (c *Client) doJSON(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var payload struct {
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&payload)
		message := payload.Message
		if message == "" {
			message = payload.Error
		}
		if message == "" {
			message = "twitch request failed"
		}
		return fmt.Errorf("%s: status %d", message, resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}
