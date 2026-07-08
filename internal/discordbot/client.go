package discordbot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/dimension-science/tournament-kit/internal/store"
)

const discordAPIBase = "https://discord.com/api/v10"

const (
	commandSyncRoles               = "sync_roles"
	commandSetupTournamentChannels = "setup_tournament_channels"
)

type Client struct {
	token               string
	guildID             string
	newsChannelID       string
	modUpdatesChannelID string
	supportChannelID    string
	moderatorRoleID     string
	runnerRoleID        string
	sessionMu           sync.RWMutex
	session             *discordgo.Session
	httpClient          *http.Client
	logger              *log.Logger
}

type roleSyncResult struct {
	ApprovedApplications int
	ScannedMembers       int
	Assigned             int
	AlreadyHadRole       int
	UnmatchedApplicants  []string
	Errors               []string
}

type channelSetupResult struct {
	CategoryName string
	Created      []string
	Updated      []string
}

type tournamentChannelSpec struct {
	Name        string
	Type        discordgo.ChannelType
	Topic       string
	StarterText string
	Permissions func(everyoneID, runnerRoleID, moderatorRoleID, ownerID, botID string) []*discordgo.PermissionOverwrite
	NeedsRunner bool
}

type Config struct {
	Token               string
	GuildID             string
	NewsChannelID       string
	ModUpdatesChannelID string
	SupportChannelID    string
	ModeratorRoleID     string
	RunnerRoleID        string
}

type NewsPost struct {
	Title   string
	Text    string
	Links   []string
	Media   *MediaAttachment
	Mention bool
}

type MediaAttachment struct {
	FileName    string
	ContentType string
	Data        []byte
}

type messagePayload struct {
	Content         string          `json:"content,omitempty"`
	AllowedMentions allowedMentions `json:"allowed_mentions"`
}

type allowedMentions struct {
	Parse []string `json:"parse"`
	Roles []string `json:"roles,omitempty"`
}

func New(cfg Config, logger *log.Logger) *Client {
	cfg.Token = strings.TrimSpace(cfg.Token)
	cfg.GuildID = strings.TrimSpace(cfg.GuildID)
	if cfg.Token == "" || cfg.GuildID == "" {
		if logger != nil {
			logger.Print("discord bot disabled: DISCORD_BOT_TOKEN or DISCORD_GUILD_ID is empty")
		}
		return nil
	}
	if logger == nil {
		logger = log.Default()
	}
	newsChannel := strings.TrimSpace(cfg.NewsChannelID)
	modUpdatesChannel := strings.TrimSpace(cfg.ModUpdatesChannelID)
	if modUpdatesChannel == "" {
		modUpdatesChannel = newsChannel
	}
	return &Client{
		token:               cfg.Token,
		guildID:             cfg.GuildID,
		newsChannelID:       newsChannel,
		modUpdatesChannelID: modUpdatesChannel,
		supportChannelID:    strings.TrimSpace(cfg.SupportChannelID),
		moderatorRoleID:     strings.TrimSpace(cfg.ModeratorRoleID),
		runnerRoleID:        strings.TrimSpace(cfg.RunnerRoleID),
		httpClient:          &http.Client{Timeout: 15 * time.Second},
		logger:              logger,
	}
}

func (c *Client) NewsEnabled() bool {
	return c != nil && c.newsChannelID != ""
}

func (c *Client) ModUpdatesEnabled() bool {
	return c != nil && (c.modUpdatesChannelID != "" || c.newsChannelID != "")
}

func (c *Client) SupportEnabled() bool {
	return c != nil && c.supportChannelID != ""
}

func (c *Client) NotifyApplicationSubmitted(ctx context.Context, app *store.TournamentApplication) {
	if !c.SupportEnabled() || app == nil {
		return
	}
	lines := []string{
		fmt.Sprintf("РќРѕРІР°СЏ Р·Р°СЏРІРєР° #%d", app.ApplicationNumber),
		"",
		"Twitch: " + emptyDash(app.TwitchLogin),
		"Discord: " + emptyDash(app.DiscordUsername),
		"РљР°РЅР°Р»: " + emptyDash(app.TwitchChannelURL),
		"Р§Р°СЃРѕРІРѕР№ РїРѕСЏСЃ: " + emptyDash(app.Timezone),
		"ref: " + emptyDash(app.Referral),
	}
	c.sendBestEffort(ctx, c.supportChannelID, strings.Join(lines, "\n"), nil)
}

func (c *Client) NotifyApplicationReviewed(ctx context.Context, app *store.TournamentApplication) {
	if !c.SupportEnabled() || app == nil {
		return
	}
	status := "РѕР±РЅРѕРІР»РµРЅР°"
	switch app.Status {
	case "approved":
		status = "РѕРґРѕР±СЂРµРЅР°"
	case "rejected":
		status = "РѕС‚РєР»РѕРЅРµРЅР°"
	}
	lines := []string{
		fmt.Sprintf("Р—Р°СЏРІРєР° #%d %s", app.ApplicationNumber, status),
		"",
		"Twitch: " + emptyDash(app.TwitchLogin),
		"Discord: " + emptyDash(app.DiscordUsername),
	}
	c.sendBestEffort(ctx, c.supportChannelID, strings.Join(lines, "\n"), nil)
}

func (c *Client) NotifySupportTicket(ctx context.Context, ticket *store.TelegramSupportTicket) {
	if !c.SupportEnabled() || ticket == nil {
		return
	}
	lines := []string{
		fmt.Sprintf("РќРѕРІРѕРµ РѕР±СЂР°С‰РµРЅРёРµ #%d", ticket.TicketNumber),
		"",
		"РћС‚: " + supportUserLabel(ticket),
		fmt.Sprintf("Telegram chat: %d", ticket.UserChatID),
		"",
		ticket.Question,
		"",
		fmt.Sprintf("РћС‚РІРµС‚ РІ Telegram: РћС‚РІРµС‚ #%d С‚РµРєСЃС‚ РѕС‚РІРµС‚Р°", ticket.TicketNumber),
	}
	c.sendBestEffort(ctx, c.supportChannelID, strings.Join(lines, "\n"), nil)
}

func (c *Client) NotifyNews(ctx context.Context, text string, mentionRunners bool) {
	text = strings.TrimSpace(text)
	if !c.NewsEnabled() || text == "" {
		return
	}
	c.NotifyNewsPost(ctx, NewsPost{Text: text, Mention: mentionRunners})
}

func (c *Client) NotifyNewsPost(ctx context.Context, post NewsPost) {
	post.Text = strings.TrimSpace(post.Text)
	if !c.NewsEnabled() || post.Text == "" {
		return
	}
	roles := []string(nil)
	content := ""
	if post.Mention && c.runnerRoleID != "" {
		content = "<@&" + c.runnerRoleID + ">"
		roles = []string{c.runnerRoleID}
	}
	if err := c.sendNewsPost(ctx, c.newsChannelID, content, post, roles); err != nil {
		if c.logger != nil {
			c.logger.Printf("discord news post: %v", err)
		}
		fallback := post.Text
		if len(post.Links) > 0 {
			fallback += "\n\nРЎСЃС‹Р»РєРё:\n- " + strings.Join(uniqueStrings(post.Links), "\n- ")
		}
		if content != "" {
			fallback = content + "\n" + fallback
		}
		c.sendBestEffort(ctx, c.newsChannelID, fallback, roles)
	}
}

func (c *Client) NotifyModUpdate(ctx context.Context, post NewsPost) {
	if c == nil {
		return
	}
	targetChannel := c.modUpdatesChannelID
	if targetChannel == "" {
		targetChannel = c.newsChannelID
	}
	post.Text = strings.TrimSpace(post.Text)
	if targetChannel == "" || post.Text == "" {
		return
	}
	roles := []string(nil)
	content := ""
	if post.Mention && c.runnerRoleID != "" {
		content = "<@&" + c.runnerRoleID + ">"
		roles = []string{c.runnerRoleID}
	}
	if err := c.sendNewsPost(ctx, targetChannel, content, post, roles); err != nil {
		if c.logger != nil {
			c.logger.Printf("discord mod update post: %v", err)
		}
		fallback := post.Text
		if len(post.Links) > 0 {
			fallback += "\n\nРЎСЃС‹Р»РєРё:\n- " + strings.Join(uniqueStrings(post.Links), "\n- ")
		}
		if content != "" {
			fallback = content + "\n" + fallback
		}
		c.sendBestEffort(ctx, targetChannel, fallback, roles)
	}
}

func (c *Client) StartRoleSync(ctx context.Context, st *store.Store) {
	if c == nil || st == nil {
		return
	}
	if c.runnerRoleID == "" {
		if c.logger != nil {
			c.logger.Print("discord role sync disabled: DISCORD_RUNNER_ROLE_ID is empty")
		}
	}

	session, err := discordgo.New("Bot " + c.token)
	if err != nil {
		if c.logger != nil {
			c.logger.Printf("discord gateway init: %v", err)
		}
		return
	}
	session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMembers
	var registerOnce sync.Once
	session.AddHandler(func(_ *discordgo.Session, ready *discordgo.Ready) {
		if c.logger != nil && ready != nil && ready.User != nil {
			c.logger.Printf("discord bot connected as %s", ready.User.String())
		}
		registerOnce.Do(func() {
			if err := c.registerCommands(session); err != nil && c.logger != nil {
				c.logger.Printf("discord register commands: %v", err)
			}
			go c.ensureSupportChannelPrivate(context.Background(), session)
		})
	})
	session.AddHandler(func(s *discordgo.Session, event *discordgo.GuildMemberAdd) {
		c.handleGuildMemberAdd(ctx, st, s, event)
	})
	session.AddHandler(func(s *discordgo.Session, event *discordgo.InteractionCreate) {
		c.handleInteraction(ctx, st, s, event)
	})

	if err := session.Open(); err != nil {
		if c.logger != nil {
			c.logger.Printf("discord gateway open: %v", err)
		}
		return
	}
	c.sessionMu.Lock()
	c.session = session
	c.sessionMu.Unlock()

	go func() {
		<-ctx.Done()
		if err := session.Close(); err != nil && c.logger != nil {
			c.logger.Printf("discord gateway close: %v", err)
		}
		c.sessionMu.Lock()
		if c.session == session {
			c.session = nil
		}
		c.sessionMu.Unlock()
	}()
}

func (c *Client) SyncRunnerRoles(ctx context.Context, st *store.Store) (string, error) {
	if c == nil {
		return "", fmt.Errorf("discord bot is disabled")
	}
	c.sessionMu.RLock()
	session := c.session
	c.sessionMu.RUnlock()
	if session == nil {
		return "", fmt.Errorf("discord bot is not connected")
	}
	result, err := c.syncRunnerRoles(ctx, st, session)
	if err != nil {
		return "", err
	}
	message := result.roleSyncMessage()
	if c.SupportEnabled() {
		c.sendBestEffort(context.Background(), c.supportChannelID, message, nil)
	}
	return message, nil
}

func (c *Client) registerCommands(session *discordgo.Session) error {
	if session == nil || session.State == nil || session.State.User == nil {
		return fmt.Errorf("discord session user is not ready")
	}
	adminPermission := int64(discordgo.PermissionAdministrator)
	dmPermission := false
	commandsToRegister := []*discordgo.ApplicationCommand{
		{
			Name:                     commandSyncRoles,
			Description:              "Р’С‹РґР°С‚СЊ Tournament Runner СѓС‡Р°СЃС‚РЅРёРєР°Рј РёР· РѕРґРѕР±СЂРµРЅРЅС‹С… Р·Р°СЏРІРѕРє",
			DefaultMemberPermissions: &adminPermission,
			DMPermission:             &dmPermission,
		},
		{
			Name:                     commandSetupTournamentChannels,
			Description:              "РЎРѕР·РґР°С‚СЊ Рё РЅР°СЃС‚СЂРѕРёС‚СЊ РєР°РЅР°Р»С‹ Tournament РІ РІС‹Р±СЂР°РЅРЅРѕР№ РєР°С‚РµРіРѕСЂРёРё",
			DefaultMemberPermissions: &adminPermission,
			DMPermission:             &dmPermission,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionChannel,
					Name:        "category",
					Description: "РљР°С‚РµРіРѕСЂРёСЏ, РєСѓРґР° СЃРѕР·РґР°С‚СЊ РєР°РЅР°Р»С‹. Р•СЃР»Рё РїСѓСЃС‚Рѕ, РёСЃРїРѕР»СЊР·СѓРµС‚СЃСЏ С‚РµРєСѓС‰Р°СЏ РєР°С‚РµРіРѕСЂРёСЏ.",
					Required:    false,
					ChannelTypes: []discordgo.ChannelType{
						discordgo.ChannelTypeGuildCategory,
					},
				},
			},
		},
	}

	commands, err := session.ApplicationCommands(session.State.User.ID, c.guildID)
	if err != nil {
		return err
	}
	existingByName := map[string]*discordgo.ApplicationCommand{}
	for _, existingCommand := range commands {
		if existingCommand != nil {
			existingByName[existingCommand.Name] = existingCommand
		}
	}
	for _, command := range commandsToRegister {
		if existing := existingByName[command.Name]; existing != nil {
			if _, err := session.ApplicationCommandEdit(session.State.User.ID, c.guildID, existing.ID, command); err != nil {
				return err
			}
			continue
		}
		if _, err := session.ApplicationCommandCreate(session.State.User.ID, c.guildID, command); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) handleInteraction(ctx context.Context, st *store.Store, session *discordgo.Session, event *discordgo.InteractionCreate) {
	if event == nil || event.GuildID != c.guildID || event.Type != discordgo.InteractionApplicationCommand {
		return
	}
	switch event.ApplicationCommandData().Name {
	case commandSyncRoles:
		c.handleSyncRolesInteraction(ctx, st, session, event)
	case commandSetupTournamentChannels:
		c.handleSetupChannelsInteraction(ctx, session, event)
	}
}

func (c *Client) handleSyncRolesInteraction(ctx context.Context, st *store.Store, session *discordgo.Session, event *discordgo.InteractionCreate) {
	if !memberCanRunAdminCommand(event.Member) {
		_ = session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "РљРѕРјР°РЅРґР° РґРѕСЃС‚СѓРїРЅР° С‚РѕР»СЊРєРѕ Р°РґРјРёРЅРёСЃС‚СЂР°С‚РѕСЂР°Рј СЃРµСЂРІРµСЂР°.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	if err := session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	}); err != nil {
		if c.logger != nil {
			c.logger.Printf("discord sync_roles defer: %v", err)
		}
		return
	}

	go func() {
		syncCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		result, err := c.syncRunnerRoles(syncCtx, st, session)
		content := ""
		if err != nil {
			content = "РЎРёРЅС…СЂРѕРЅРёР·Р°С†РёСЏ СЂРѕР»РµР№ РЅРµ СѓРґР°Р»Р°СЃСЊ: " + err.Error()
		} else {
			content = result.roleSyncMessage()
		}
		if _, editErr := session.InteractionResponseEdit(event.Interaction, &discordgo.WebhookEdit{Content: &content}); editErr != nil && c.logger != nil {
			c.logger.Printf("discord sync_roles response edit: %v", editErr)
		}
		if c.SupportEnabled() {
			c.sendBestEffort(context.Background(), c.supportChannelID, content, nil)
		}
	}()
}

func (c *Client) handleSetupChannelsInteraction(ctx context.Context, session *discordgo.Session, event *discordgo.InteractionCreate) {
	if !memberCanRunAdminCommand(event.Member) {
		_ = session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "РљРѕРјР°РЅРґР° РґРѕСЃС‚СѓРїРЅР° С‚РѕР»СЊРєРѕ Р°РґРјРёРЅРёСЃС‚СЂР°С‚РѕСЂР°Рј СЃРµСЂРІРµСЂР°.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	if err := session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	}); err != nil {
		if c.logger != nil {
			c.logger.Printf("discord setup_tournament_channels defer: %v", err)
		}
		return
	}

	go func() {
		setupCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		result, err := c.setupTournamentChannels(setupCtx, session, event)
		content := ""
		if err != nil {
			content = "РќР°СЃС‚СЂРѕР№РєР° РєР°РЅР°Р»РѕРІ Tournament РЅРµ СѓРґР°Р»Р°СЃСЊ: " + err.Error()
		} else {
			content = result.channelSetupMessage()
		}
		if _, editErr := session.InteractionResponseEdit(event.Interaction, &discordgo.WebhookEdit{Content: &content}); editErr != nil && c.logger != nil {
			c.logger.Printf("discord setup_tournament_channels response edit: %v", editErr)
		}
		if c.SupportEnabled() {
			c.sendBestEffort(context.Background(), c.supportChannelID, content, nil)
		}
	}()
}

func (c *Client) handleGuildMemberAdd(ctx context.Context, st *store.Store, session *discordgo.Session, event *discordgo.GuildMemberAdd) {
	if event == nil || event.GuildID != c.guildID || event.User == nil {
		return
	}

	matchCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	applications, err := st.ListTournamentApplications(matchCtx)
	if err != nil {
		if c.logger != nil {
			c.logger.Printf("discord role sync load applications: %v", err)
		}
		return
	}

	memberNames := normalizedDiscordNames(event.Member)
	var matched *store.TournamentApplication
	for i := range applications {
		application := &applications[i]
		if application.Status != "approved" {
			continue
		}
		if memberNames[normalizeDiscordName(application.DiscordUsername)] {
			matched = application
			break
		}
	}
	if matched == nil {
		c.sendBestEffort(matchCtx, c.supportChannelID, "РќРѕРІС‹Р№ СѓС‡Р°СЃС‚РЅРёРє Discord РІРѕС€РµР» РЅР° СЃРµСЂРІРµСЂ, РЅРѕ РЅРµ РЅР°Р№РґРµРЅ СЃСЂРµРґРё РѕРґРѕР±СЂРµРЅРЅС‹С… Р·Р°СЏРІРѕРє: "+discordMemberLabel(event.Member), nil)
		return
	}

	if err := session.GuildMemberRoleAdd(c.guildID, event.User.ID, c.runnerRoleID); err != nil {
		if c.logger != nil {
			c.logger.Printf("discord role add for %s: %v", event.User.String(), err)
		}
		c.sendBestEffort(matchCtx, c.supportChannelID, fmt.Sprintf("РќРµ СѓРґР°Р»РѕСЃСЊ РІС‹РґР°С‚СЊ Tournament Runner РґР»СЏ Р·Р°СЏРІРєРё #%d (%s): %v", matched.ApplicationNumber, discordMemberLabel(event.Member), err), nil)
		return
	}

	c.sendBestEffort(matchCtx, c.supportChannelID, fmt.Sprintf("Р’С‹РґР°РЅР° СЂРѕР»СЊ Tournament Runner: %s -> Р·Р°СЏРІРєР° #%d (%s)", discordMemberLabel(event.Member), matched.ApplicationNumber, matched.TwitchLogin), nil)
}

func (c *Client) syncRunnerRoles(ctx context.Context, st *store.Store, session *discordgo.Session) (roleSyncResult, error) {
	result := roleSyncResult{}
	if c == nil || st == nil || session == nil {
		return result, fmt.Errorf("discord role sync is not configured")
	}
	if c.runnerRoleID == "" {
		return result, fmt.Errorf("DISCORD_RUNNER_ROLE_ID is empty")
	}

	applications, err := st.ListTournamentApplications(ctx)
	if err != nil {
		return result, err
	}
	appByName := map[string]*store.TournamentApplication{}
	for i := range applications {
		application := &applications[i]
		if application.Status != "approved" {
			continue
		}
		result.ApprovedApplications++
		normalized := normalizeDiscordName(application.DiscordUsername)
		if normalized == "" {
			continue
		}
		if _, exists := appByName[normalized]; !exists {
			appByName[normalized] = application
		}
	}

	matchedApplicationIDs := map[string]bool{}
	after := ""
	for {
		members, err := session.GuildMembers(c.guildID, after, 1000)
		if err != nil {
			return result, err
		}
		if len(members) == 0 {
			break
		}
		for _, member := range members {
			if member == nil || member.User == nil {
				continue
			}
			after = member.User.ID
			result.ScannedMembers++
			if member.User.Bot {
				continue
			}
			application := matchApplicationForMember(appByName, member)
			if application == nil {
				continue
			}
			matchedApplicationIDs[application.ID] = true
			if memberHasRole(member, c.runnerRoleID) {
				result.AlreadyHadRole++
				continue
			}
			if err := session.GuildMemberRoleAdd(c.guildID, member.User.ID, c.runnerRoleID); err != nil {
				if c.logger != nil {
					c.logger.Printf("role add failed for %s: %v", discordMemberLabel(member), err)
				}
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", discordMemberLabel(member), err))
				continue
			}
			result.Assigned++
		}
		if len(members) < 1000 {
			break
		}
	}

	for i := range applications {
		application := &applications[i]
		if application.Status != "approved" || normalizeDiscordName(application.DiscordUsername) == "" {
			continue
		}
		if !matchedApplicationIDs[application.ID] {
			result.UnmatchedApplicants = append(result.UnmatchedApplicants, fmt.Sprintf("#%d %s / Discord: %s", application.ApplicationNumber, application.TwitchLogin, application.DiscordUsername))
		}
	}
	return result, nil
}

func (c *Client) setupTournamentChannels(ctx context.Context, session *discordgo.Session, event *discordgo.InteractionCreate) (channelSetupResult, error) {
	result := channelSetupResult{}
	if c == nil || session == nil || event == nil {
		return result, fmt.Errorf("discord channel setup is not configured")
	}
	if c.runnerRoleID == "" {
		return result, fmt.Errorf("DISCORD_RUNNER_ROLE_ID is empty")
	}
	if session.State == nil || session.State.User == nil {
		return result, fmt.Errorf("discord bot user is not ready")
	}

	category, err := selectedSetupCategory(session, event)
	if err != nil {
		return result, err
	}
	result.CategoryName = category.Name

	guild, err := session.Guild(c.guildID)
	if err != nil {
		return result, fmt.Errorf("load guild: %w", err)
	}
	ownerID := guild.OwnerID
	if ownerID == "" && event.Member != nil && event.Member.User != nil {
		ownerID = event.Member.User.ID
	}
	if ownerID == "" {
		return result, fmt.Errorf("server owner is unknown")
	}

	channels, err := session.GuildChannels(c.guildID)
	if err != nil {
		return result, fmt.Errorf("load channels: %w", err)
	}
	existingByKey := map[string]*discordgo.Channel{}
	for _, channel := range channels {
		if channel == nil || channel.ParentID != category.ID {
			continue
		}
		existingByKey[tournamentChannelKey(channel.Name, channel.Type)] = channel
	}

	configuredChannels := map[string]*discordgo.Channel{}
	for _, spec := range tournamentChannelSpecs() {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}
		if spec.NeedsRunner && c.runnerRoleID == "" {
			continue
		}
		key := tournamentChannelKey(spec.Name, spec.Type)
		if existing := existingByKey[key]; existing != nil {
			permissions := spec.Permissions(c.guildID, c.runnerRoleID, c.moderatorRoleID, ownerID, session.State.User.ID)
			if c.supportChannelID != "" && existing.ID == c.supportChannelID {
				permissions = ownerBotOnlyTextOverwrites(c.guildID, "", "", ownerID, session.State.User.ID)
			}
			if _, err := session.ChannelEditComplex(existing.ID, &discordgo.ChannelEdit{
				Name:                 spec.Name,
				Topic:                spec.Topic,
				ParentID:             category.ID,
				PermissionOverwrites: permissions,
			}); err != nil {
				return result, fmt.Errorf("update %s: %w", spec.Name, err)
			}
			result.Updated = append(result.Updated, spec.Name)
			configuredChannels[spec.Name] = existing
			continue
		}

		permissions := spec.Permissions(c.guildID, c.runnerRoleID, c.moderatorRoleID, ownerID, session.State.User.ID)
		created, err := session.GuildChannelCreateComplex(c.guildID, discordgo.GuildChannelCreateData{
			Name:                 spec.Name,
			Type:                 spec.Type,
			Topic:                spec.Topic,
			ParentID:             category.ID,
			PermissionOverwrites: permissions,
		})
		if err != nil {
			return result, fmt.Errorf("create %s: %w", spec.Name, err)
		}
		result.Created = append(result.Created, spec.Name)
		configuredChannels[spec.Name] = created
		if created != nil && strings.TrimSpace(spec.StarterText) != "" && spec.Type == discordgo.ChannelTypeGuildText {
			c.sendBestEffort(ctx, created.ID, spec.StarterText, nil)
		}
	}

	c.announceTournamentChannels(ctx, configuredChannels)
	return result, nil
}

func (c *Client) announceTournamentChannels(ctx context.Context, channels map[string]*discordgo.Channel) {
	if c == nil || c.runnerRoleID == "" {
		return
	}
	newsChannelID := c.newsChannelID
	if channel := channels["РЅРѕРІРѕСЃС‚Рё-С‚СѓСЂРЅРёСЂР°"]; channel != nil && channel.ID != "" {
		newsChannelID = channel.ID
	}
	if newsChannelID == "" {
		return
	}

	lines := []string{
		"<@&" + c.runnerRoleID + ">",
		"",
		"РљР°РЅР°Р»С‹ Tournament РіРѕС‚РѕРІС‹. Р—РґРµСЃСЊ Р±СѓРґСѓС‚ РЅРѕРІРѕСЃС‚Рё С‚СѓСЂРЅРёСЂР°, РѕР±РЅРѕРІР»РµРЅРёСЏ РјРѕРґР° Рё РєРѕРѕСЂРґРёРЅР°С†РёСЏ СЂР°РЅРЅРµСЂРѕРІ.",
		"",
	}
	for _, name := range []string{"РЅРѕРІРѕСЃС‚Рё-С‚СѓСЂРЅРёСЂР°", "РѕР±РЅРѕРІР»РµРЅРёСЏ-РјРѕРґР°", "С‡Р°С‚-СЂР°РЅРЅРµСЂРѕРІ", "РіРѕР»РѕСЃРѕРІРѕР№-РєР°РЅР°Р»"} {
		channel := channels[name]
		if channel == nil || channel.ID == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s: <#%s>", name, channel.ID))
	}
	c.sendBestEffort(ctx, newsChannelID, strings.Join(lines, "\n"), []string{c.runnerRoleID})
}

func (c *Client) sendBestEffort(ctx context.Context, channelID, content string, allowedRoleIDs []string) {
	if c == nil || strings.TrimSpace(channelID) == "" {
		return
	}
	content = trimDiscordMessage(content)
	if content == "" {
		return
	}
	if err := c.sendMessage(ctx, channelID, content, allowedRoleIDs); err != nil && c.logger != nil {
		c.logger.Printf("discord sendMessage: %v", err)
	}
}

func (c *Client) sendNewsPost(ctx context.Context, channelID, content string, post NewsPost, allowedRoleIDs []string) error {
	description := trimDiscordEmbedDescription(post.Text)
	links := uniqueStrings(post.Links)
	if len(links) > 0 {
		description += "\n\n**РЎСЃС‹Р»РєРё:**\n- " + strings.Join(links, "\n- ")
	}

	title := "РќРѕРІРѕСЃС‚СЊ Tournament"
	if post.Title != "" {
		title = post.Title
	}
	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: description,
		Color:       0x9A63FF,
		Timestamp:   time.Now().Format(time.RFC3339),
	}

	message := &discordgo.MessageSend{
		Content: content,
		Embeds:  []*discordgo.MessageEmbed{embed},
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Parse: []discordgo.AllowedMentionType{},
			Roles: allowedRoleIDs,
		},
	}
	if post.Media != nil && len(post.Media.Data) > 0 && post.Media.FileName != "" {
		message.Files = []*discordgo.File{{
			Name:        post.Media.FileName,
			ContentType: post.Media.ContentType,
			Reader:      bytes.NewReader(post.Media.Data),
		}}
		if strings.HasPrefix(strings.ToLower(post.Media.ContentType), "image/") {
			embed.Image = &discordgo.MessageEmbedImage{URL: "attachment://" + post.Media.FileName}
		}
	}

	c.sessionMu.RLock()
	session := c.session
	c.sessionMu.RUnlock()
	if session != nil {
		_, err := session.ChannelMessageSendComplex(channelID, message)
		return err
	}
	if post.Media != nil {
		return fmt.Errorf("discord gateway session is not ready for media upload")
	}
	return c.sendMessage(ctx, channelID, content+"\n"+description, allowedRoleIDs)
}

func (c *Client) ensureSupportChannelPrivate(_ context.Context, session *discordgo.Session) {
	if c == nil || session == nil || c.supportChannelID == "" {
		return
	}
	guild, err := session.Guild(c.guildID)
	if err != nil {
		if c.logger != nil {
			c.logger.Printf("discord support channel permissions: load guild: %v", err)
		}
		return
	}
	ownerID := guild.OwnerID
	botID := ""
	if session.State != nil && session.State.User != nil {
		botID = session.State.User.ID
	}
	_, err = session.ChannelEditComplex(c.supportChannelID, &discordgo.ChannelEdit{
		PermissionOverwrites: ownerBotOnlyTextOverwrites(c.guildID, "", "", ownerID, botID),
	})
	if err != nil && c.logger != nil {
		c.logger.Printf("discord support channel permissions: %v", err)
	}
}

func (c *Client) staffRoleIDs(ctx context.Context, session *discordgo.Session) []string {
	roleIDs := []string{}
	seen := map[string]bool{}
	add := func(roleID string) {
		roleID = strings.TrimSpace(roleID)
		if roleID == "" || seen[roleID] {
			return
		}
		seen[roleID] = true
		roleIDs = append(roleIDs, roleID)
	}
	add(c.moderatorRoleID)

	roles, err := session.GuildRoles(c.guildID)
	if err != nil {
		if c.logger != nil {
			c.logger.Printf("discord staff roles lookup: %v", err)
		}
		return roleIDs
	}
	select {
	case <-ctx.Done():
		return roleIDs
	default:
	}
	for _, role := range roles {
		if role == nil {
			continue
		}
		name := strings.ToLower(role.Name)
		if strings.Contains(name, "moder") || strings.Contains(name, "РјРѕРґРµСЂ") {
			add(role.ID)
		}
	}
	return roleIDs
}

func tournamentChannelSpecs() []tournamentChannelSpec {
	return []tournamentChannelSpec{
		{
			Name:  "Р·Р°СЏРІРєР°",
			Type:  discordgo.ChannelTypeGuildText,
			Topic: "Р—Р°РєСЂС‹С‚С‹Р№ РєР°РЅР°Р» РґР»СЏ РІРѕРїСЂРѕСЃРѕРІ РїРѕ СѓС‡Р°СЃС‚РёСЋ РІ Tournament. Р”РѕСЃС‚СѓРїРµРЅ С‚РѕР»СЊРєРѕ Tournament Runner, РІР»Р°РґРµР»СЊС†Сѓ Рё Р±РѕС‚Сѓ.",
			StarterText: strings.Join([]string{
				"Р—Р°СЏРІРєРё РЅР° С‚СѓСЂРЅРёСЂ РїРѕРґР°СЋС‚СЃСЏ С‡РµСЂРµР· СЃР°Р№С‚ Tournament.",
				"",
				"Р—РґРµСЃСЊ РјРѕР¶РЅРѕ СѓС‚РѕС‡РЅСЏС‚СЊ РІРѕРїСЂРѕСЃС‹ РїРѕ СѓС‡Р°СЃС‚РёСЋ РїРѕСЃР»Рµ РїРѕР»СѓС‡РµРЅРёСЏ РґРѕСЃС‚СѓРїР°.",
			}, "\n"),
			Permissions: runnerWritableOverwrites,
			NeedsRunner: true,
		},
		{
			Name:        "РЅРѕРІРѕСЃС‚Рё-С‚СѓСЂРЅРёСЂР°",
			Type:        discordgo.ChannelTypeGuildNews,
			Topic:       "РћС‚РєСЂС‹С‚С‹Рµ РЅРѕРІРѕСЃС‚Рё Tournament. Р§РёС‚Р°С‚СЊ РјРѕРіСѓС‚ РІСЃРµ, РїРёСЃР°С‚СЊ РјРѕР¶РµС‚ С‚РѕР»СЊРєРѕ РєРѕРјР°РЅРґР° С‚СѓСЂРЅРёСЂР° Рё Р±РѕС‚.",
			Permissions: publicReadOnlyOverwrites,
		},
		{
			Name:        "РїРѕРґРґРµСЂР¶РєР°-tournament",
			Type:        discordgo.ChannelTypeGuildText,
			Topic:       "Р—Р°РєСЂС‹С‚С‹Р№ С‡Р°С‚ РїРѕРґРґРµСЂР¶РєРё РґР»СЏ СѓС‡Р°СЃС‚РЅРёРєРѕРІ С‚СѓСЂРЅРёСЂР°.",
			Permissions: runnerWritableOverwrites,
			NeedsRunner: true,
		},
		{
			Name:        "РѕР±РЅРѕРІР»РµРЅРёСЏ-РјРѕРґР°",
			Type:        discordgo.ChannelTypeGuildText,
			Topic:       "Р¤РёРєСЃС‹ Рё РѕР±РЅРѕРІР»РµРЅРёСЏ РѕС„РёС†РёР°Р»СЊРЅС‹С… РјРѕРґРѕРІ Tournament. Р”РѕСЃС‚СѓРїРЅРѕ С‚РѕР»СЊРєРѕ Tournament Runner, РІР»Р°РґРµР»СЊС†Сѓ Рё Р±РѕС‚Сѓ.",
			Permissions: runnerWritableOverwrites,
			NeedsRunner: true,
		},
		{
			Name:        "С‡Р°С‚-СЂР°РЅРЅРµСЂРѕРІ",
			Type:        discordgo.ChannelTypeGuildText,
			Topic:       "Р—Р°РєСЂС‹С‚С‹Р№ С‡Р°С‚ СѓС‡Р°СЃС‚РЅРёРєРѕРІ С‚СѓСЂРЅРёСЂР°.",
			Permissions: runnerWritableOverwrites,
			NeedsRunner: true,
		},
		{
			Name:        "РіРѕР»РѕСЃРѕРІРѕР№-РєР°РЅР°Р»",
			Type:        discordgo.ChannelTypeGuildVoice,
			Permissions: runnerVoiceOverwrites,
			NeedsRunner: true,
		},
		{
			Name:        "С‚РµС…РїРѕРґРґРµСЂР¶РєР°",
			Type:        discordgo.ChannelTypeGuildText,
			Topic:       "Р—Р°РєСЂС‹С‚С‹Р№ РєР°РЅР°Р» С‚РµС…РїРѕРґРґРµСЂР¶РєРё РґР»СЏ СѓС‡Р°СЃС‚РЅРёРєРѕРІ С‚СѓСЂРЅРёСЂР°.",
			Permissions: runnerWritableOverwrites,
			NeedsRunner: true,
		},
		{
			Name:        "Р±РѕС‚-СЃР»СѓР¶РµР±РЅС‹Р№",
			Type:        discordgo.ChannelTypeGuildText,
			Topic:       "Р›РѕРіРё Рё СЃР»СѓР¶РµР±РЅС‹Рµ СЃРѕРѕР±С‰РµРЅРёСЏ Tournament-Р±РѕС‚Р°. Р”РѕСЃС‚СѓРїРµРЅ РІР»Р°РґРµР»СЊС†Сѓ СЃРµСЂРІРµСЂР° Рё Р±РѕС‚Сѓ.",
			Permissions: ownerBotOnlyTextOverwrites,
		},
	}
}

func selectedSetupCategory(session *discordgo.Session, event *discordgo.InteractionCreate) (*discordgo.Channel, error) {
	data := event.ApplicationCommandData()
	for _, option := range data.Options {
		if option != nil && option.Name == "category" {
			channel := option.ChannelValue(session)
			if channel == nil || channel.ID == "" {
				return nil, fmt.Errorf("category is empty")
			}
			if channel.Type != discordgo.ChannelTypeGuildCategory {
				return nil, fmt.Errorf("selected channel is not a category")
			}
			return channel, nil
		}
	}

	current, err := session.Channel(event.ChannelID)
	if err != nil {
		return nil, fmt.Errorf("load current channel: %w", err)
	}
	if current == nil || current.ParentID == "" {
		return nil, fmt.Errorf("СѓРєР°Р¶РёС‚Рµ category РёР»Рё Р·Р°РїСѓСЃС‚РёС‚Рµ РєРѕРјР°РЅРґСѓ РёР· РєР°РЅР°Р»Р° РІРЅСѓС‚СЂРё РЅСѓР¶РЅРѕР№ РєР°С‚РµРіРѕСЂРёРё")
	}
	category, err := session.Channel(current.ParentID)
	if err != nil {
		return nil, fmt.Errorf("load current category: %w", err)
	}
	if category == nil || category.Type != discordgo.ChannelTypeGuildCategory {
		return nil, fmt.Errorf("current parent is not a category")
	}
	return category, nil
}

func publicWritableOverwrites(everyoneID, _ string, _ string, ownerID, botID string) []*discordgo.PermissionOverwrite {
	return permissionOverwrites(
		roleOverwrite(everyoneID, textWritePermissions(), 0),
		memberOverwrite(ownerID, ownerTextManagePermissions(), 0),
		memberOverwrite(botID, ownerTextManagePermissions(), 0),
	)
}

func publicReadOnlyOverwrites(everyoneID, _ string, _ string, ownerID, botID string) []*discordgo.PermissionOverwrite {
	return permissionOverwrites(
		roleOverwrite(everyoneID, textReadPermissions(), textPostDenyPermissions()),
		memberOverwrite(ownerID, ownerTextManagePermissions(), 0),
		memberOverwrite(botID, ownerTextManagePermissions(), 0),
	)
}

func runnerReadOnlyOverwrites(everyoneID, runnerRoleID, _ string, ownerID, botID string) []*discordgo.PermissionOverwrite {
	return permissionOverwrites(
		roleOverwrite(everyoneID, 0, discordgo.PermissionViewChannel),
		roleOverwrite(runnerRoleID, textReadPermissions(), textPostDenyPermissions()),
		memberOverwrite(ownerID, ownerTextManagePermissions(), 0),
		memberOverwrite(botID, ownerTextManagePermissions(), 0),
	)
}

func runnerWritableOverwrites(everyoneID, runnerRoleID, _ string, ownerID, botID string) []*discordgo.PermissionOverwrite {
	return permissionOverwrites(
		roleOverwrite(everyoneID, 0, discordgo.PermissionViewChannel),
		roleOverwrite(runnerRoleID, textWritePermissions(), 0),
		memberOverwrite(ownerID, ownerTextManagePermissions(), 0),
		memberOverwrite(botID, ownerTextManagePermissions(), 0),
	)
}

func ownerBotOnlyTextOverwrites(everyoneID, _ string, _ string, ownerID, botID string) []*discordgo.PermissionOverwrite {
	return permissionOverwrites(
		roleOverwrite(everyoneID, 0, discordgo.PermissionViewChannel),
		memberOverwrite(ownerID, ownerTextManagePermissions(), 0),
		memberOverwrite(botID, ownerTextManagePermissions(), 0),
	)
}

func staffPrivateTextOverwrites(everyoneID, _ string, moderatorRoleID string, ownerID, botID string) []*discordgo.PermissionOverwrite {
	return permissionOverwrites(
		roleOverwrite(everyoneID, 0, discordgo.PermissionViewChannel),
		roleOverwrite(moderatorRoleID, textWritePermissions(), 0),
		memberOverwrite(ownerID, ownerTextManagePermissions(), 0),
		memberOverwrite(botID, ownerTextManagePermissions(), 0),
	)
}

func runnerVoiceOverwrites(everyoneID, runnerRoleID, _ string, ownerID, botID string) []*discordgo.PermissionOverwrite {
	return permissionOverwrites(
		roleOverwrite(everyoneID, 0, discordgo.PermissionViewChannel|discordgo.PermissionVoiceConnect),
		roleOverwrite(runnerRoleID, voiceUsePermissions(), 0),
		memberOverwrite(ownerID, ownerVoiceManagePermissions(), 0),
		memberOverwrite(botID, ownerVoiceManagePermissions(), 0),
	)
}

func roleOverwrite(id string, allow, deny int64) *discordgo.PermissionOverwrite {
	return permissionOverwrite(id, discordgo.PermissionOverwriteTypeRole, allow, deny)
}

func memberOverwrite(id string, allow, deny int64) *discordgo.PermissionOverwrite {
	return permissionOverwrite(id, discordgo.PermissionOverwriteTypeMember, allow, deny)
}

func permissionOverwrite(id string, overwriteType discordgo.PermissionOverwriteType, allow, deny int64) *discordgo.PermissionOverwrite {
	if strings.TrimSpace(id) == "" {
		return nil
	}
	return &discordgo.PermissionOverwrite{
		ID:    id,
		Type:  overwriteType,
		Allow: allow,
		Deny:  deny,
	}
}

func permissionOverwrites(overwrites ...*discordgo.PermissionOverwrite) []*discordgo.PermissionOverwrite {
	seen := map[string]bool{}
	filtered := make([]*discordgo.PermissionOverwrite, 0, len(overwrites))
	for _, overwrite := range overwrites {
		if overwrite == nil {
			continue
		}
		key := fmt.Sprintf("%d:%s", overwrite.Type, overwrite.ID)
		if seen[key] {
			continue
		}
		seen[key] = true
		filtered = append(filtered, overwrite)
	}
	return filtered
}

func textReadPermissions() int64 {
	return discordgo.PermissionViewChannel | discordgo.PermissionReadMessageHistory
}

func textWritePermissions() int64 {
	return textReadPermissions() | discordgo.PermissionSendMessages
}

func textPostDenyPermissions() int64 {
	return discordgo.PermissionSendMessages |
		discordgo.PermissionCreatePublicThreads |
		discordgo.PermissionCreatePrivateThreads |
		discordgo.PermissionSendMessagesInThreads
}

func ownerTextManagePermissions() int64 {
	return textWritePermissions() |
		discordgo.PermissionManageChannels |
		discordgo.PermissionCreatePublicThreads |
		discordgo.PermissionCreatePrivateThreads |
		discordgo.PermissionSendMessagesInThreads
}

func voiceUsePermissions() int64 {
	return discordgo.PermissionViewChannel | discordgo.PermissionVoiceConnect | discordgo.PermissionVoiceSpeak
}

func ownerVoiceManagePermissions() int64 {
	return voiceUsePermissions() | discordgo.PermissionManageChannels
}

func memberCanRunAdminCommand(member *discordgo.Member) bool {
	if member == nil {
		return false
	}
	return member.Permissions&discordgo.PermissionAdministrator != 0 || member.Permissions&discordgo.PermissionManageChannels != 0
}

func tournamentChannelKey(name string, channelType discordgo.ChannelType) string {
	return fmt.Sprintf("%d:%s", channelType, normalizeChannelName(name))
}

func normalizeChannelName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func (c *Client) sendMessage(ctx context.Context, channelID, content string, allowedRoleIDs []string) error {
	payload := messagePayload{
		Content: content,
		AllowedMentions: allowedMentions{
			Parse: []string{},
			Roles: allowedRoleIDs,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, discordAPIBase+"/channels/"+channelID+"/messages", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+c.token)
	req.Header.Set("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		return fmt.Errorf("discord status %d: %s", res.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	return nil
}

func emptyDash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

func supportUserLabel(ticket *store.TelegramSupportTicket) string {
	if ticket == nil {
		return "unknown"
	}
	if strings.TrimSpace(ticket.UserUsername) != "" {
		return ticket.UserUsername
	}
	if ticket.UserTelegramID != 0 {
		return fmt.Sprintf("%d", ticket.UserTelegramID)
	}
	return fmt.Sprintf("%d", ticket.UserChatID)
}

func normalizedDiscordNames(member *discordgo.Member) map[string]bool {
	names := map[string]bool{}
	if member == nil {
		return names
	}
	if member.Nick != "" {
		names[normalizeDiscordName(member.Nick)] = true
	}
	if member.User != nil {
		names[normalizeDiscordName(member.User.Username)] = true
		if member.User.GlobalName != "" {
			names[normalizeDiscordName(member.User.GlobalName)] = true
		}
		if member.User.Discriminator != "" && member.User.Discriminator != "0" {
			names[normalizeDiscordName(member.User.Username+"#"+member.User.Discriminator)] = true
		}
	}
	delete(names, "")
	return names
}

func matchApplicationForMember(applicationsByName map[string]*store.TournamentApplication, member *discordgo.Member) *store.TournamentApplication {
	for name := range normalizedDiscordNames(member) {
		if application := applicationsByName[name]; application != nil {
			return application
		}
	}
	return nil
}

func memberHasRole(member *discordgo.Member, roleID string) bool {
	if member == nil || roleID == "" {
		return false
	}
	for _, existingRoleID := range member.Roles {
		if existingRoleID == roleID {
			return true
		}
	}
	return false
}

func normalizeDiscordName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimPrefix(value, "@")
	value = strings.ReplaceAll(value, " ", "")
	if strings.HasSuffix(value, "#0000") {
		value = strings.TrimSuffix(value, "#0000")
	} else if strings.HasSuffix(value, "#0") {
		value = strings.TrimSuffix(value, "#0")
	}
	return value
}

func discordMemberLabel(member *discordgo.Member) string {
	if member == nil || member.User == nil {
		return "unknown"
	}
	label := member.User.Username
	if member.User.GlobalName != "" {
		label = member.User.GlobalName + " / " + label
	}
	if member.Nick != "" {
		label += " / nick: " + member.Nick
	}
	return label
}

func trimDiscordMessage(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= 1900 {
		return value
	}
	return string(runes[:1900]) + "\n..."
}

func trimDiscordEmbedDescription(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= 3500 {
		return value
	}
	return string(runes[:3500]) + "\n..."
}

func uniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func (r roleSyncResult) roleSyncMessage() string {
	lines := []string{
		"РЎРёРЅС…СЂРѕРЅРёР·Р°С†РёСЏ СЂРѕР»РµР№ Tournament Р·Р°РІРµСЂС€РµРЅР°.",
		"",
		fmt.Sprintf("РћРґРѕР±СЂРµРЅРЅС‹С… Р·Р°СЏРІРѕРє: %d", r.ApprovedApplications),
		fmt.Sprintf("РџСЂРѕРІРµСЂРµРЅРѕ СѓС‡Р°СЃС‚РЅРёРєРѕРІ Discord: %d", r.ScannedMembers),
		fmt.Sprintf("Р РѕР»СЊ РІС‹РґР°РЅР° СЃРµР№С‡Р°СЃ: %d", r.Assigned),
		fmt.Sprintf("Р РѕР»СЊ СѓР¶Рµ Р±С‹Р»Р°: %d", r.AlreadyHadRole),
	}
	if len(r.UnmatchedApplicants) > 0 {
		lines = append(lines, "", fmt.Sprintf("РќРµ РЅР°Р№РґРµРЅРѕ РЅР° СЃРµСЂРІРµСЂРµ РёР»Рё РЅРёРє РЅРµ СЃРѕРІРїР°Р»: %d", len(r.UnmatchedApplicants)))
		limit := len(r.UnmatchedApplicants)
		if limit > 8 {
			limit = 8
		}
		lines = append(lines, r.UnmatchedApplicants[:limit]...)
		if len(r.UnmatchedApplicants) > limit {
			lines = append(lines, fmt.Sprintf("...Рё РµС‰Рµ %d", len(r.UnmatchedApplicants)-limit))
		}
	}
	if len(r.Errors) > 0 {
		lines = append(lines, "", fmt.Sprintf("РћС€РёР±РєРё РїСЂРё РІС‹РґР°С‡Рµ СЂРѕР»РµР№: %d", len(r.Errors)))
		limit := len(r.Errors)
		if limit > 5 {
			limit = 5
		}
		lines = append(lines, r.Errors[:limit]...)
		if len(r.Errors) > limit {
			lines = append(lines, fmt.Sprintf("...Рё РµС‰Рµ %d", len(r.Errors)-limit))
		}
	}
	return strings.Join(lines, "\n")
}

func (r channelSetupResult) channelSetupMessage() string {
	lines := []string{
		"РљР°РЅР°Р»С‹ Tournament РЅР°СЃС‚СЂРѕРµРЅС‹.",
		"",
		"РљР°С‚РµРіРѕСЂРёСЏ: " + emptyDash(r.CategoryName),
		fmt.Sprintf("РЎРѕР·РґР°РЅРѕ: %d", len(r.Created)),
		fmt.Sprintf("РћР±РЅРѕРІР»РµРЅРѕ: %d", len(r.Updated)),
	}
	if len(r.Created) > 0 {
		lines = append(lines, "", "РЎРѕР·РґР°РЅРЅС‹Рµ РєР°РЅР°Р»С‹:", strings.Join(r.Created, ", "))
	}
	if len(r.Updated) > 0 {
		lines = append(lines, "", "РћР±РЅРѕРІР»РµРЅРЅС‹Рµ РєР°РЅР°Р»С‹:", strings.Join(r.Updated, ", "))
	}
	lines = append(lines, "", "Р’Р°Р¶РЅРѕ: РїРѕР»СЊР·РѕРІР°С‚РµР»Рё СЃ РїСЂР°РІРѕРј Administrator РІ Discord РјРѕРіСѓС‚ РІРёРґРµС‚СЊ Р·Р°РєСЂС‹С‚С‹Рµ РєР°РЅР°Р»С‹ РЅРµР·Р°РІРёСЃРёРјРѕ РѕС‚ РїРµСЂРµР·Р°РїРёСЃРµР№ РїСЂР°РІ.")
	return strings.Join(lines, "\n")
}
