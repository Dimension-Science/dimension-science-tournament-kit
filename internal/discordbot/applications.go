package discordbot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/dimension-science/tournament-kit/internal/store"
)

const (
	componentApplyChaos  = "ds:chaos:apply"
	componentApproveBase = "ds:chaos:approve:"
	componentRejectBase  = "ds:chaos:reject:"
	modalApplyChaos      = "ds:chaos:apply-modal"
	modalRejectBase      = "ds:chaos:reject-modal:"
)

func (c *Client) handleSetupDSApplicationsInteraction(session *discordgo.Session, event *discordgo.InteractionCreate) {
	if !memberCanRunAdminCommand(event.Member) {
		c.respondEphemeral(session, event, "Команда доступна только администраторам сервера.")
		return
	}
	embed := &discordgo.MessageEmbed{
		Title:       "Заявка в Dimension Science: Chaos",
		Description: "Прочитайте требования и нажмите **Подать заявку**. Ответы увидит только команда Dimension Science. После проверки одобренным участникам автоматически выдаётся роль **Игрок DS | Chaos**.",
		Color:       0x7C3AED,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Перед отправкой", Value: "Подготовьте игровой ник, часовой пояс, краткое описание опыта и мотивацию."},
			{Name: "Обработка", Value: "Одновременно может быть активна только одна заявка. Решение придёт личным сообщением, если личные сообщения доступны."},
		},
	}
	button := discordgo.Button{Label: "Подать заявку", Style: discordgo.PrimaryButton, CustomID: componentApplyChaos, Emoji: &discordgo.ComponentEmoji{Name: "📝"}}
	if err := session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}, Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{button}}}},
	}); err != nil && c.logger != nil {
		c.logger.Printf("discord application panel: %v", err)
	}
}

func (c *Client) handleDSApplicationInteraction(ctx context.Context, st *store.Store, session *discordgo.Session, event *discordgo.InteractionCreate) {
	switch event.Type {
	case discordgo.InteractionMessageComponent:
		customID := event.MessageComponentData().CustomID
		switch {
		case customID == componentApplyChaos:
			c.openChaosApplicationModal(session, event)
		case strings.HasPrefix(customID, componentApproveBase):
			c.approveChaosApplication(ctx, st, session, event, strings.TrimPrefix(customID, componentApproveBase))
		case strings.HasPrefix(customID, componentRejectBase):
			c.openRejectModal(session, event, strings.TrimPrefix(customID, componentRejectBase))
		}
	case discordgo.InteractionModalSubmit:
		customID := event.ModalSubmitData().CustomID
		switch {
		case customID == modalApplyChaos:
			c.submitChaosApplication(ctx, st, session, event)
		case strings.HasPrefix(customID, modalRejectBase):
			c.rejectChaosApplication(ctx, st, session, event, strings.TrimPrefix(customID, modalRejectBase))
		}
	}
}

func (c *Client) openChaosApplicationModal(session *discordgo.Session, event *discordgo.InteractionCreate) {
	fields := []discordgo.MessageComponent{
		textInputRow("game_nick", "Игровой ник", "Ваш ник в Minecraft", discordgo.TextInputShort, 2, 32, true),
		textInputRow("timezone", "Часовой пояс", "Например: UTC+5", discordgo.TextInputShort, 2, 32, true),
		textInputRow("experience", "Опыт", "Расскажите кратко об опыте игры или участия", discordgo.TextInputParagraph, 10, 600, true),
		textInputRow("motivation", "Почему хотите присоединиться?", "Несколько предложений", discordgo.TextInputParagraph, 10, 700, true),
		textInputRow("links", "Ссылки (необязательно)", "Twitch, YouTube или профиль", discordgo.TextInputShort, 0, 300, false),
	}
	_ = session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseModal, Data: &discordgo.InteractionResponseData{CustomID: modalApplyChaos, Title: "Заявка DS | Chaos", Components: fields}})
}

func (c *Client) submitChaosApplication(ctx context.Context, st *store.Store, session *discordgo.Session, event *discordgo.InteractionCreate) {
	if st == nil || event.Member == nil || event.Member.User == nil {
		c.respondEphemeral(session, event, "Не удалось определить пользователя. Попробуйте ещё раз.")
		return
	}
	values := modalValues(event.ModalSubmitData().Components)
	app, err := st.CreateDiscordApplication(ctx, store.DiscordApplicationInput{
		GuildID: c.guildID, DiscordUserID: event.Member.User.ID, DiscordUsername: event.Member.User.String(), ProjectKey: "chaos",
		GameNick: values["game_nick"], Timezone: values["timezone"], Experience: values["experience"], Motivation: values["motivation"], Links: values["links"],
	})
	if errors.Is(err, store.ErrActiveDiscordApplication) {
		c.respondEphemeral(session, event, "У вас уже есть активная или одобренная заявка в DS | Chaos.")
		return
	}
	if err != nil {
		if c.logger != nil {
			c.logger.Printf("create Discord application: %v", err)
		}
		c.respondEphemeral(session, event, "Не удалось сохранить заявку. Попробуйте позже.")
		return
	}
	if strings.TrimSpace(c.applicationLogID) == "" {
		c.respondEphemeral(session, event, fmt.Sprintf("Заявка DSC-%05d сохранена, но канал проверки ещё не настроен. Сообщите администратору.", app.ApplicationNumber))
		return
	}
	message, err := session.ChannelMessageSendComplex(c.applicationLogID, applicationLogMessage(app, true))
	if err != nil {
		if c.logger != nil {
			c.logger.Printf("send Discord application log: %v", err)
		}
		c.respondEphemeral(session, event, fmt.Sprintf("Заявка DSC-%05d сохранена. Команда увидит её в панели администратора.", app.ApplicationNumber))
		return
	}
	_ = st.SetDiscordApplicationLogMessage(ctx, app.ID, message.ChannelID, message.ID)
	c.respondEphemeral(session, event, fmt.Sprintf("Заявка **DSC-%05d** принята и отправлена на рассмотрение.", app.ApplicationNumber))
}

func (c *Client) approveChaosApplication(ctx context.Context, st *store.Store, session *discordgo.Session, event *discordgo.InteractionCreate, id string) {
	if !c.memberCanReview(event.Member) {
		c.respondEphemeral(session, event, "Недостаточно прав для проверки заявок.")
		return
	}
	app, err := st.FindDiscordApplicationByID(ctx, id)
	if err != nil || app == nil {
		c.respondEphemeral(session, event, "Заявка не найдена.")
		return
	}
	if app.Status != "pending" && app.Status != "needs_info" {
		c.respondEphemeral(session, event, "Эта заявка уже обработана.")
		return
	}
	if strings.TrimSpace(c.chaosPlayerRoleID) == "" {
		c.respondEphemeral(session, event, "Не настроена роль DISCORD_CHAOS_PLAYER_ROLE_ID.")
		return
	}
	if err := session.GuildMemberRoleAdd(c.guildID, app.DiscordUserID, c.chaosPlayerRoleID); err != nil {
		c.respondEphemeral(session, event, "Не удалось выдать роль: "+err.Error())
		return
	}
	reviewerID := interactionUserID(event)
	updated, err := st.ReviewDiscordApplication(ctx, id, "approved", reviewerID, "", true)
	if err != nil {
		_ = session.GuildMemberRoleRemove(c.guildID, app.DiscordUserID, c.chaosPlayerRoleID)
		c.respondEphemeral(session, event, "Не удалось зафиксировать решение: "+err.Error())
		return
	}
	c.updateReviewMessage(session, event, updated)
	c.sendApplicantDM(session, app.DiscordUserID, fmt.Sprintf("Ваша заявка DSC-%05d одобрена. Вам выдана роль **Игрок DS | Chaos**.", app.ApplicationNumber))
}

func (c *Client) openRejectModal(session *discordgo.Session, event *discordgo.InteractionCreate, id string) {
	if !c.memberCanReview(event.Member) {
		c.respondEphemeral(session, event, "Недостаточно прав для проверки заявок.")
		return
	}
	_ = session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseModal, Data: &discordgo.InteractionResponseData{
		CustomID: modalRejectBase + id, Title: "Отклонение заявки", Components: []discordgo.MessageComponent{textInputRow("reason", "Причина", "Кратко объясните причину", discordgo.TextInputParagraph, 3, 500, true)},
	}})
}

func (c *Client) rejectChaosApplication(ctx context.Context, st *store.Store, session *discordgo.Session, event *discordgo.InteractionCreate, id string) {
	if !c.memberCanReview(event.Member) {
		c.respondEphemeral(session, event, "Недостаточно прав для проверки заявок.")
		return
	}
	reason := modalValues(event.ModalSubmitData().Components)["reason"]
	app, err := st.ReviewDiscordApplication(ctx, id, "rejected", interactionUserID(event), reason, false)
	if err != nil || app == nil {
		c.respondEphemeral(session, event, "Не удалось отклонить заявку: "+errorText(err))
		return
	}
	c.respondEphemeral(session, event, fmt.Sprintf("Заявка DSC-%05d отклонена.", app.ApplicationNumber))
	if app.LogChannelID != "" && app.LogMessageID != "" {
		_, _ = session.ChannelMessageEditComplex(&discordgo.MessageEdit{Channel: app.LogChannelID, ID: app.LogMessageID, Embeds: &[]*discordgo.MessageEmbed{applicationEmbed(app)}, Components: &[]discordgo.MessageComponent{}})
	}
	c.sendApplicantDM(session, app.DiscordUserID, fmt.Sprintf("Ваша заявка DSC-%05d отклонена. Причина: %s", app.ApplicationNumber, reason))
}

func applicationLogMessage(app *store.DiscordApplication, actions bool) *discordgo.MessageSend {
	message := &discordgo.MessageSend{Embeds: []*discordgo.MessageEmbed{applicationEmbed(app)}, AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}}
	if actions {
		message.Components = []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: "Одобрить", Style: discordgo.SuccessButton, CustomID: componentApproveBase + app.ID},
			discordgo.Button{Label: "Отклонить", Style: discordgo.DangerButton, CustomID: componentRejectBase + app.ID},
		}}}
	}
	return message
}

func applicationEmbed(app *store.DiscordApplication) *discordgo.MessageEmbed {
	status := map[string]string{"pending": "На рассмотрении", "approved": "Одобрена", "rejected": "Отклонена", "needs_info": "Нужно уточнение"}[app.Status]
	fields := []*discordgo.MessageEmbedField{
		{Name: "Discord", Value: fmt.Sprintf("<@%s> (`%s`)", app.DiscordUserID, app.DiscordUserID), Inline: false},
		{Name: "Игровой ник", Value: safeField(app.GameNick), Inline: true}, {Name: "Часовой пояс", Value: safeField(app.Timezone), Inline: true},
		{Name: "Опыт", Value: safeField(app.Experience)}, {Name: "Мотивация", Value: safeField(app.Motivation)}, {Name: "Ссылки", Value: safeField(app.Links)},
	}
	if app.ReviewReason != "" {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Причина решения", Value: safeField(app.ReviewReason)})
	}
	return &discordgo.MessageEmbed{Title: fmt.Sprintf("Заявка DSC-%05d", app.ApplicationNumber), Description: "Статус: **" + status + "**", Color: applicationStatusColor(app.Status), Fields: fields, Timestamp: app.CreatedAt.Format(time.RFC3339)}
}

func (c *Client) updateReviewMessage(session *discordgo.Session, event *discordgo.InteractionCreate, app *store.DiscordApplication) {
	embeds := []*discordgo.MessageEmbed{applicationEmbed(app)}
	components := []discordgo.MessageComponent{}
	_ = session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseUpdateMessage, Data: &discordgo.InteractionResponseData{Embeds: embeds, Components: components}})
}

func (c *Client) memberCanReview(member *discordgo.Member) bool {
	if memberCanRunAdminCommand(member) {
		return true
	}
	if member == nil || c.moderatorRoleID == "" {
		return false
	}
	for _, roleID := range member.Roles {
		if roleID == c.moderatorRoleID {
			return true
		}
	}
	return false
}

func (c *Client) respondEphemeral(session *discordgo.Session, event *discordgo.InteractionCreate, content string) {
	_ = session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionResponseChannelMessageWithSource, Data: &discordgo.InteractionResponseData{Content: content, Flags: discordgo.MessageFlagsEphemeral}})
}

func (c *Client) sendApplicantDM(session *discordgo.Session, userID, content string) {
	channel, err := session.UserChannelCreate(userID)
	if err != nil {
		return
	}
	_, _ = session.ChannelMessageSend(channel.ID, content)
}

// ReviewApplicationFromAdmin lets the existing web admin panel use the same
// role assignment and Discord notification path as the staff message buttons.
func (c *Client) ReviewApplicationFromAdmin(ctx context.Context, st *store.Store, id, status, actor, reason string) (*store.DiscordApplication, error) {
	if c == nil || st == nil {
		return nil, errors.New("Discord bot is disabled")
	}
	c.sessionMu.RLock()
	session := c.session
	c.sessionMu.RUnlock()
	if session == nil {
		return nil, errors.New("Discord bot is not connected")
	}
	app, err := st.FindDiscordApplicationByID(ctx, id)
	if err != nil || app == nil {
		if err == nil {
			err = errors.New("application not found")
		}
		return nil, err
	}
	roleGranted := false
	if status == "approved" {
		if c.chaosPlayerRoleID == "" {
			return nil, errors.New("DISCORD_CHAOS_PLAYER_ROLE_ID is empty")
		}
		if err := session.GuildMemberRoleAdd(c.guildID, app.DiscordUserID, c.chaosPlayerRoleID); err != nil {
			return nil, err
		}
		roleGranted = true
	}
	updated, err := st.ReviewDiscordApplication(ctx, id, status, actor, reason, roleGranted)
	if err != nil {
		if roleGranted {
			_ = session.GuildMemberRoleRemove(c.guildID, app.DiscordUserID, c.chaosPlayerRoleID)
		}
		return nil, err
	}
	if updated.LogChannelID != "" && updated.LogMessageID != "" {
		embeds := []*discordgo.MessageEmbed{applicationEmbed(updated)}
		components := []discordgo.MessageComponent{}
		_, _ = session.ChannelMessageEditComplex(&discordgo.MessageEdit{Channel: updated.LogChannelID, ID: updated.LogMessageID, Embeds: &embeds, Components: &components})
	}
	if status == "approved" {
		c.sendApplicantDM(session, updated.DiscordUserID, fmt.Sprintf("Ваша заявка DSC-%05d одобрена. Вам выдана роль **Игрок DS | Chaos**.", updated.ApplicationNumber))
	} else {
		c.sendApplicantDM(session, updated.DiscordUserID, fmt.Sprintf("Ваша заявка DSC-%05d отклонена. Причина: %s", updated.ApplicationNumber, safeField(reason)))
	}
	return updated, nil
}

func textInputRow(id, label, placeholder string, style discordgo.TextInputStyle, min, max int, required bool) discordgo.ActionsRow {
	return discordgo.ActionsRow{Components: []discordgo.MessageComponent{discordgo.TextInput{CustomID: id, Label: label, Placeholder: placeholder, Style: style, MinLength: min, MaxLength: max, Required: required}}}
}

func modalValues(components []discordgo.MessageComponent) map[string]string {
	values := map[string]string{}
	for _, component := range components {
		row, ok := component.(*discordgo.ActionsRow)
		if !ok {
			if value, ok2 := component.(discordgo.ActionsRow); ok2 {
				row = &value
			} else {
				continue
			}
		}
		for _, child := range row.Components {
			if input, ok := child.(*discordgo.TextInput); ok {
				values[input.CustomID] = strings.TrimSpace(input.Value)
				continue
			}
			if input, ok := child.(discordgo.TextInput); ok {
				values[input.CustomID] = strings.TrimSpace(input.Value)
			}
		}
	}
	return values
}

func interactionUserID(event *discordgo.InteractionCreate) string {
	if event.Member != nil && event.Member.User != nil {
		return event.Member.User.ID
	}
	if event.User != nil {
		return event.User.ID
	}
	return "unknown"
}

func safeField(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "—"
	}
	if len(value) > 1000 {
		return value[:1000] + "…"
	}
	return value
}
func applicationStatusColor(status string) int {
	if status == "approved" {
		return 0x22C55E
	}
	if status == "rejected" {
		return 0xEF4444
	}
	return 0xF59E0B
}
func errorText(err error) string {
	if err == nil {
		return "неизвестная ошибка"
	}
	return err.Error()
}
