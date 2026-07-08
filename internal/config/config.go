package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv                     string
	Port                       string
	BaseURL                    string
	BaseOrigin                 string
	DatabaseURL                string
	SessionSecret              string
	SessionTTL                 time.Duration
	AuthCookieName             string
	AuthCookieSecure           bool
	ModAPIKey                  string
	ModSigningSecret           string
	ModMaxSkew                 time.Duration
	AllowDevMockAuth           bool
	TwitchClientID             string
	TwitchClientSecret         string
	TwitchRedirectURI          string
	TwitchForceVerify          bool
	EnableTwitchStreams        bool
	StreamStatusCacheTTL       time.Duration
	TournamentTitle            string
	SiteShortName              string
	OrganizerName              string
	OrganizerURL               string
	SupportURL                 string
	TelegramURL                string
	DiscordURL                 string
	TwitchURL                  string
	GameName                   string
	GameVersion                string
	PrizeText                  string
	CertificateTemplateDir     string
	CertificatePrefix          string
	BadgeAssetDir              string
	EnableAchievements         bool
	TelegramBotToken           string
	TelegramAdminChatID        int64
	TelegramNewsChat           string
	DiscordBotToken            string
	DiscordGuildID             string
	DiscordNewsChannelID       string
	DiscordModUpdatesChannelID string
	DiscordSupportID           string
	DiscordModeratorRoleID     string
	DiscordRunnerRoleID        string
	AllowedOrigins             map[string]struct{}
	UploadDir                  string
	MaxAvatarBytes             int64
}

func Load() (Config, error) {
	cfg := Config{
		AppEnv:                     env("APP_ENV", "development"),
		Port:                       env("PORT", "3000"),
		BaseURL:                    env("APP_BASE_URL", "http://localhost:3000"),
		DatabaseURL:                env("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/speedrun?sslmode=disable"),
		AuthCookieName:             env("AUTH_COOKIE_NAME", "speedrun_session"),
		UploadDir:                  env("UPLOAD_DIR", filepath.FromSlash("data/uploads")),
		TournamentTitle:            env("TOURNAMENT_TITLE", "Example Speedrun Tournament"),
		SiteShortName:              env("SITE_SHORT_NAME", "EST"),
		OrganizerName:              env("ORGANIZER_NAME", "Dimension Science"),
		OrganizerURL:               strings.TrimSpace(os.Getenv("ORGANIZER_URL")),
		SupportURL:                 strings.TrimSpace(os.Getenv("SUPPORT_URL")),
		TelegramURL:                strings.TrimSpace(os.Getenv("TELEGRAM_URL")),
		DiscordURL:                 strings.TrimSpace(os.Getenv("DISCORD_URL")),
		TwitchURL:                  strings.TrimSpace(os.Getenv("TWITCH_URL")),
		GameName:                   env("GAME_NAME", "Minecraft Java Edition"),
		GameVersion:                env("GAME_VERSION", "1.21.11"),
		PrizeText:                  strings.TrimSpace(os.Getenv("PRIZE_TEXT")),
		CertificateTemplateDir:     env("CERTIFICATE_TEMPLATE_DIR", filepath.FromSlash("assets/certificates")),
		CertificatePrefix:          env("CERTIFICATE_PREFIX", "CERT-2026"),
		BadgeAssetDir:              env("BADGE_ASSET_DIR", filepath.FromSlash("internal/web/static")),
		EnableAchievements:         envBool("ENABLE_ACHIEVEMENTS", true),
		TelegramBotToken:           strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		TelegramAdminChatID:        envInt64("TELEGRAM_ADMIN_CHAT_ID", 0),
		TelegramNewsChat:           strings.TrimSpace(os.Getenv("TELEGRAM_NEWS_CHAT")),
		DiscordBotToken:            strings.TrimSpace(os.Getenv("DISCORD_BOT_TOKEN")),
		DiscordGuildID:             strings.TrimSpace(os.Getenv("DISCORD_GUILD_ID")),
		DiscordNewsChannelID:       strings.TrimSpace(os.Getenv("DISCORD_NEWS_CHANNEL_ID")),
		DiscordModUpdatesChannelID: strings.TrimSpace(os.Getenv("DISCORD_MOD_UPDATES_CHANNEL_ID")),
		DiscordSupportID:           strings.TrimSpace(os.Getenv("DISCORD_SUPPORT_CHANNEL_ID")),
		DiscordModeratorRoleID:     strings.TrimSpace(os.Getenv("DISCORD_MODERATOR_ROLE_ID")),
		DiscordRunnerRoleID:        strings.TrimSpace(os.Getenv("DISCORD_RUNNER_ROLE_ID")),
		MaxAvatarBytes:             int64(envInt("MAX_AVATAR_BYTES", 2*1024*1024)),
		AllowDevMockAuth:           envBool("ALLOW_DEV_MOCK_AUTH", true),
		TwitchClientID:             strings.TrimSpace(os.Getenv("TWITCH_CLIENT_ID")),
		TwitchClientSecret:         strings.TrimSpace(os.Getenv("TWITCH_CLIENT_SECRET")),
		TwitchRedirectURI:          strings.TrimSpace(os.Getenv("TWITCH_REDIRECT_URI")),
		TwitchForceVerify:          envBool("TWITCH_FORCE_VERIFY", false),
		EnableTwitchStreams:        envBool("ENABLE_TWITCH_STREAM_CHECKS", true),
		StreamStatusCacheTTL:       time.Duration(envInt("STREAM_STATUS_CACHE_SEC", 60)) * time.Second,
		AuthCookieSecure:           envBool("AUTH_COOKIE_SECURE", false),
		SessionTTL:                 time.Duration(envInt("AUTH_SESSION_HOURS", 12)) * time.Hour,
		ModMaxSkew:                 time.Duration(envInt("MOD_MAX_SKEW_SEC", 120)) * time.Second,
	}

	if cfg.AppEnv != "development" && cfg.AppEnv != "test" && cfg.AppEnv != "production" {
		return Config{}, errors.New("APP_ENV must be one of: development, test, production")
	}

	var err error
	cfg.BaseURL, err = normalizeURL(cfg.BaseURL)
	if err != nil {
		return Config{}, fmt.Errorf("APP_BASE_URL: %w", err)
	}
	baseParsed, _ := url.Parse(cfg.BaseURL)
	cfg.BaseOrigin = baseParsed.Scheme + "://" + baseParsed.Host

	cfg.SessionSecret = strings.TrimSpace(os.Getenv("SESSION_SECRET"))
	if cfg.SessionSecret == "" {
		if cfg.AppEnv == "production" {
			return Config{}, errors.New("SESSION_SECRET is required in production")
		}
		cfg.SessionSecret = "dev_only_replace_me_session_secret_1234567890"
	}

	cfg.ModAPIKey = strings.TrimSpace(os.Getenv("MOD_API_KEY"))
	if isMissingOrPlaceholderSecret(cfg.ModAPIKey) {
		return Config{}, errors.New("MOD_API_KEY is required and must be a real secret")
	}

	cfg.ModSigningSecret = strings.TrimSpace(os.Getenv("MOD_SIGNING_SECRET"))
	if isMissingOrPlaceholderSecret(cfg.ModSigningSecret) {
		return Config{}, errors.New("MOD_SIGNING_SECRET is required and must be a real secret")
	}

	if cfg.AppEnv == "production" && cfg.AllowDevMockAuth {
		return Config{}, errors.New("ALLOW_DEV_MOCK_AUTH must be false in production")
	}

	if cfg.TwitchEnabled() {
		cfg.TwitchRedirectURI, err = normalizeURL(cfg.TwitchRedirectURI)
		if err != nil {
			return Config{}, fmt.Errorf("TWITCH_REDIRECT_URI: %w", err)
		}
	} else if cfg.AppEnv == "production" {
		return Config{}, errors.New("TWITCH_CLIENT_ID, TWITCH_CLIENT_SECRET and TWITCH_REDIRECT_URI are required in production")
	}

	cfg.AllowedOrigins = map[string]struct{}{cfg.BaseOrigin: {}}
	for _, origin := range strings.Split(env("ALLOWED_ORIGINS", cfg.BaseOrigin), ",") {
		trimmed := strings.TrimSpace(origin)
		if trimmed == "" {
			continue
		}
		normalized, normalizeErr := normalizeURL(trimmed)
		if normalizeErr != nil {
			return Config{}, fmt.Errorf("ALLOWED_ORIGINS: %w", normalizeErr)
		}
		parsed, _ := url.Parse(normalized)
		cfg.AllowedOrigins[parsed.Scheme+"://"+parsed.Host] = struct{}{}
	}

	return cfg, nil
}

func (c Config) IsProduction() bool {
	return c.AppEnv == "production"
}

func (c Config) TwitchEnabled() bool {
	return c.TwitchClientID != "" && c.TwitchClientSecret != "" && c.TwitchRedirectURI != ""
}

func (c Config) DiscordEnabled() bool {
	return c.DiscordBotToken != "" && c.DiscordGuildID != ""
}

func (c Config) OriginAllowed(origin string) bool {
	_, ok := c.AllowedOrigins[origin]
	return ok
}

type PublicProfile struct {
	SiteName               string `json:"siteName"`
	ShortName              string `json:"shortName"`
	OrganizerName          string `json:"organizerName"`
	OrganizerURL           string `json:"organizerUrl,omitempty"`
	SupportURL             string `json:"supportUrl,omitempty"`
	TelegramURL            string `json:"telegramUrl,omitempty"`
	DiscordURL             string `json:"discordUrl,omitempty"`
	TwitchURL              string `json:"twitchUrl,omitempty"`
	GameName               string `json:"gameName"`
	GameVersion            string `json:"gameVersion"`
	PrizeText              string `json:"prizeText,omitempty"`
	CertificateTemplateDir string `json:"certificateTemplateDir"`
	CertificatePrefix      string `json:"certificatePrefix"`
	BadgeAssetDir          string `json:"badgeAssetDir"`
	EnableAchievements     bool   `json:"enableAchievements"`
	BaseURL                string `json:"baseUrl"`
}

func (c Config) PublicProfile() PublicProfile {
	return PublicProfile{
		SiteName:               c.TournamentTitle,
		ShortName:              c.SiteShortName,
		OrganizerName:          c.OrganizerName,
		OrganizerURL:           c.OrganizerURL,
		SupportURL:             c.SupportURL,
		TelegramURL:            c.TelegramURL,
		DiscordURL:             c.DiscordURL,
		TwitchURL:              c.TwitchURL,
		GameName:               c.GameName,
		GameVersion:            c.GameVersion,
		PrizeText:              c.PrizeText,
		CertificateTemplateDir: c.CertificateTemplateDir,
		CertificatePrefix:      c.CertificatePrefix,
		BadgeAssetDir:          c.BadgeAssetDir,
		EnableAchievements:     c.EnableAchievements,
		BaseURL:                c.BaseURL,
	}
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func isMissingOrPlaceholderSecret(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return normalized == "" ||
		normalized == "change-me" ||
		strings.HasPrefix(normalized, "replace_with") ||
		strings.Contains(normalized, "dev_only_replace")
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt64(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes"
}

func normalizeURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("must be an absolute URL")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}
