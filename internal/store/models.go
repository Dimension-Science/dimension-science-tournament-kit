package store

import "time"

type Participant struct {
	ID                     string
	TwitchUserID           string
	TwitchLogin            string
	TwitchDisplayName      string
	TwitchProfileImageURL  string
	MinecraftNick          string
	AvatarURL              string
	BestTimeMS             *int
	Status                 string
	StreamOnline           bool
	ShowInNowPlaying       bool
	NowPlayingAuthorized   bool
	StreamTitle            string
	StreamGameName         string
	LastStreamStatusSyncAt *time.Time
	LastLoginAt            *time.Time
}

type Tournament struct {
	ID                  string     `json:"id"`
	StartsAt            time.Time  `json:"startsAt"`
	QualificationEndsAt time.Time  `json:"qualificationEndsAt"`
	PlayoffEndsAt       time.Time  `json:"playoffEndsAt"`
	EndsAt              time.Time  `json:"endsAt"`
	State               string     `json:"state"`
	Phase               string     `json:"phase"`
	PlayoffSlots        int        `json:"playoffSlots"`
	BracketGeneratedAt  *time.Time `json:"bracketGeneratedAt,omitempty"`
}

type TestModeStatus struct {
	Enabled bool       `json:"enabled"`
	EndsAt  *time.Time `json:"endsAt,omitempty"`
}

type TournamentMatch struct {
	ID                  string       `json:"id"`
	TournamentID        string       `json:"tournamentId"`
	Round               string       `json:"round"`
	Position            int          `json:"position"`
	Player1             *Participant `json:"player1,omitempty"`
	Player2             *Participant `json:"player2,omitempty"`
	Player1Seed         *int         `json:"player1Seed,omitempty"`
	Player2Seed         *int         `json:"player2Seed,omitempty"`
	StartsAt            time.Time    `json:"startsAt"`
	EndsAt              time.Time    `json:"endsAt"`
	Status              string       `json:"status"`
	WinnerParticipantID string       `json:"winnerParticipantId,omitempty"`
	BestOf              int          `json:"bestOf"`
	WorldSeed           string       `json:"worldSeed,omitempty"`
	Player1ConnectedAt  *time.Time   `json:"player1ConnectedAt,omitempty"`
	Player2ConnectedAt  *time.Time   `json:"player2ConnectedAt,omitempty"`
	OfficialStartedAt   *time.Time   `json:"officialStartedAt,omitempty"`
	Player1BestTimeMS   *int         `json:"player1BestTimeMs,omitempty"`
	Player2BestTimeMS   *int         `json:"player2BestTimeMs,omitempty"`
	WinnerDisplayName   string       `json:"winnerDisplayName,omitempty"`
}

type TournamentMatchScheduleUpdate struct {
	ID        string
	StartsAt  time.Time
	EndsAt    time.Time
	WorldSeed string
}

type LeaderboardEntry struct {
	ParticipantID     string `json:"participantId,omitempty"`
	Rank              int    `json:"rank"`
	TwitchLogin       string `json:"twitchLogin"`
	TwitchDisplayName string `json:"twitchDisplayName"`
	MinecraftNick     string `json:"minecraftNick"`
	AvatarURL         string `json:"avatarUrl,omitempty"`
	BestTimeMS        int    `json:"bestTimeMs"`
	BestRunID         string `json:"bestRunId,omitempty"`
	BestRunFinishedAt *time.Time
	RankDelta         int  `json:"rankDelta,omitempty"`
	NetherSplitMS     *int `json:"netherSplitMs,omitempty"`
	NetherExitSplitMS *int `json:"-"`
	EndSplitMS        *int `json:"endSplitMs,omitempty"`
}

type Run struct {
	ID                string                `json:"id"`
	ParticipantID     string                `json:"participantId"`
	TimeMS            int                   `json:"timeMs"`
	NetherSplitMS     *int                  `json:"netherSplitMs,omitempty"`
	NetherExitSplitMS *int                  `json:"-"`
	EndSplitMS        *int                  `json:"endSplitMs,omitempty"`
	StartedAt         time.Time             `json:"startedAt"`
	FinishedAt        time.Time             `json:"finishedAt"`
	Source            string                `json:"source"`
	Status            string                `json:"status"`
	TournamentID      string                `json:"tournamentId,omitempty"`
	Phase             string                `json:"phase,omitempty"`
	MatchID           string                `json:"matchId,omitempty"`
	CreatedAt         time.Time             `json:"createdAt"`
	Achievements      []AchievementProgress `json:"achievements,omitempty"`
}

type RunSplitInput struct {
	NetherSplitMS *int
	EndSplitMS    *int
}

type RunContext struct {
	TournamentID string `json:"tournamentId,omitempty"`
	Phase        string `json:"phase,omitempty"`
	MatchID      string `json:"matchId,omitempty"`
	WorldSeed    string `json:"worldSeed,omitempty"`
}

type ModRunSession struct {
	ID               string     `json:"id"`
	ParticipantID    string     `json:"participantId"`
	WorldFingerprint string     `json:"worldFingerprint"`
	SeedHash         string     `json:"seedHash"`
	TournamentID     string     `json:"tournamentId,omitempty"`
	Phase            string     `json:"phase,omitempty"`
	MatchID          string     `json:"matchId,omitempty"`
	Status           string     `json:"status"`
	StartedAt        time.Time  `json:"startedAt"`
	ExpiresAt        time.Time  `json:"expiresAt"`
	CompletedRunID   string     `json:"completedRunId,omitempty"`
	CompletedAt      *time.Time `json:"completedAt,omitempty"`
}

type ModAuthToken struct {
	ID            string
	ParticipantID string
	TokenPreview  string
	TokenHash     string
	LastUsedAt    *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type LoginIdentity struct {
	TwitchUserID          string
	TwitchLogin           string
	TwitchDisplayName     string
	TwitchProfileImageURL string
}

type TournamentApplication struct {
	ID                        string     `json:"id"`
	ApplicationNumber         int64      `json:"applicationNumber"`
	TwitchUserID              string     `json:"twitchUserId"`
	TwitchLogin               string     `json:"twitchLogin"`
	TwitchDisplayName         string     `json:"twitchDisplayName"`
	TwitchProfileImageURL     string     `json:"twitchProfileImageUrl"`
	TwitchChannelURL          string     `json:"twitchChannelUrl"`
	DiscordUsername           string     `json:"discordUsername"`
	Timezone                  string     `json:"timezone"`
	Referral                  string     `json:"referral"`
	UnderstandsStreamRequired bool       `json:"understandsStreamRequired"`
	Status                    string     `json:"status"`
	ParticipantID             string     `json:"participantId,omitempty"`
	ReviewedAt                *time.Time `json:"reviewedAt,omitempty"`
	CreatedAt                 time.Time  `json:"createdAt"`
	UpdatedAt                 time.Time  `json:"updatedAt"`
}

type TournamentApplicationInput struct {
	TwitchUserID              string
	TwitchLogin               string
	TwitchDisplayName         string
	TwitchProfileImageURL     string
	TwitchChannelURL          string
	DiscordUsername           string
	Timezone                  string
	Referral                  string
	UnderstandsStreamRequired bool
}

type TelegramSupportTicket struct {
	ID             string
	TicketNumber   int64
	UserChatID     int64
	UserTelegramID int64
	UserUsername   string
	Question       string
	Answer         string
	Status         string
	AnsweredAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type TelegramSupportTicketInput struct {
	UserChatID     int64
	UserTelegramID int64
	UserUsername   string
	Question       string
}

type TelegramNewsPost struct {
	ID                string
	TelegramChatID    int64
	TelegramMessageID int
	SourceURL         string
	Text              string
	ImageURL          string
	PublishedAt       time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type TelegramNewsPostInput struct {
	TelegramChatID    int64
	TelegramMessageID int
	SourceURL         string
	Text              string
	ImageURL          string
	PublishedAt       time.Time
}

type AchievementProgress struct {
	Slug         string     `json:"slug"`
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	Progress     int        `json:"progress"`
	Goal         int        `json:"goal"`
	Unlocked     bool       `json:"unlocked"`
	UnlockedAt   *time.Time `json:"unlockedAt,omitempty"`
	JustUnlocked bool       `json:"justUnlocked,omitempty"`
}

type AchievementFeedItem struct {
	ID                     string    `json:"id"`
	ParticipantID          string    `json:"participantId,omitempty"`
	TwitchLogin            string    `json:"twitchLogin"`
	DisplayName            string    `json:"displayName"`
	AvatarURL              string    `json:"avatarUrl,omitempty"`
	AchievementSlug        string    `json:"achievementSlug"`
	AchievementName        string    `json:"achievementName"`
	AchievementDescription string    `json:"achievementDescription,omitempty"`
	UnlockedAt             time.Time `json:"unlockedAt"`
}

type NotificationItem struct {
	EventTime       time.Time `json:"eventTime"`
	ParticipantID   string    `json:"participantId"`
	TwitchLogin     string    `json:"twitchLogin"`
	DisplayName     string    `json:"displayName"`
	AvatarURL       string    `json:"avatarUrl"`
	EventType       string    `json:"eventType"`  // "achievement", "run", "whitelist"
	DetailData      string    `json:"detailData"` // achievement_slug or run_id
	TimeMS          *int      `json:"timeMs,omitempty"`
	RunStatus       string    `json:"runStatus,omitempty"`
	AchievementName string    `json:"achievementName,omitempty"`
}
