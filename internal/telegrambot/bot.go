package telegrambot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"html"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chai2010/webp"
	"github.com/dimension-science/tournament-kit/internal/discordbot"
	"github.com/dimension-science/tournament-kit/internal/store"
)

type Bot struct {
	token             string
	adminChatID       int64
	newsChat          string
	baseURL           string
	discordURL        string
	uploadDir         string
	store             *store.Store
	discord           *discordbot.Client
	client            *http.Client
	logger            *log.Logger
	pendingSupport    map[int64]bool
	pendingCheckCert  map[int64]bool
	pendingCooperate  map[int64]bool
	chaosApplications map[int64]*telegramChaosApplicationDraft
	menuState         map[int64]string
}

type telegramChaosApplicationDraft struct {
	Step       int
	GameNick   string
	Timezone   string
	Experience string
	DiscordID  string
	Motivation string
	Links      string
}

type updateResponse struct {
	OK          bool             `json:"ok"`
	Description string           `json:"description"`
	Result      []telegramUpdate `json:"result"`
}

type telegramUpdate struct {
	UpdateID    int              `json:"update_id"`
	Message     *telegramMessage `json:"message"`
	ChannelPost *telegramMessage `json:"channel_post"`
}

type telegramMessage struct {
	MessageID       int                     `json:"message_id"`
	Date            int64                   `json:"date"`
	Text            string                  `json:"text"`
	Caption         string                  `json:"caption"`
	Entities        []telegramMessageEntity `json:"entities"`
	CaptionEntities []telegramMessageEntity `json:"caption_entities"`
	Photo           []telegramPhotoSize     `json:"photo"`
	Video           *telegramVideo          `json:"video"`
	Animation       *telegramAnimation      `json:"animation"`
	Document        *telegramDocument       `json:"document"`
	ReplyMarkup     *telegramReplyMarkup    `json:"reply_markup"`
	Chat            telegramChat            `json:"chat"`
	From            *telegramUser           `json:"from"`
}

type telegramMessageEntity struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type telegramPhotoSize struct {
	FileID   string `json:"file_id"`
	FileSize int    `json:"file_size"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
}

type telegramVideo struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
	FileSize int    `json:"file_size"`
}

type telegramAnimation struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
	FileSize int    `json:"file_size"`
}

type telegramDocument struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
	FileSize int    `json:"file_size"`
}

type telegramReplyMarkup struct {
	InlineKeyboard [][]telegramInlineButton `json:"inline_keyboard"`
}

type telegramInlineButton struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

type telegramFileResponse struct {
	OK          bool         `json:"ok"`
	Description string       `json:"description"`
	Result      telegramFile `json:"result"`
}

type telegramFile struct {
	FileID   string `json:"file_id"`
	FilePath string `json:"file_path"`
	FileSize int    `json:"file_size"`
}

const maxDiscordMediaBytes = 8 * 1024 * 1024
const maxNewsImageBytes = 12 * 1024 * 1024

var (
	publicPostStartPattern = regexp.MustCompile(`(?s)<div class="tgme_widget_message[^"]*"[^>]*data-post="([^"]+)"[^>]*>`)
	publicPostTimePattern  = regexp.MustCompile(`(?s)<time[^>]*datetime="([^"]+)"`)
	publicPostTextPattern  = regexp.MustCompile(`(?s)<div class="tgme_widget_message_text[^"]*"[^>]*>(.*?)</div>`)
	publicPostPhotoPattern = regexp.MustCompile(`(?s)<a[^>]+class="[^"]*tgme_widget_message_photo_wrap[^"]*"[^>]+style="[^"]*background-image:url\(([^)]+)\)`)
	htmlTagPattern         = regexp.MustCompile(`(?s)<[^>]+>`)
)

type PublicNewsSyncResult struct {
	Imported int `json:"imported"`
	Seen     int `json:"seen"`
}

type publicPostBlock struct {
	Ref  string
	HTML string
}

type telegramChat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Username string `json:"username"`
}

type telegramUser struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

const (
	buttonProjects    = "Проекты"
	buttonDTR         = "DTR"
	buttonChaos       = "Chaos"
	buttonApply       = "Подать заявку"
	buttonSupport     = "Написать в поддержку"
	buttonCooperation = "Сотрудничество"
	buttonBack        = "Назад"
	buttonCheckCert   = "Проверить сертификат"
	buttonOpenSupport = "Открытые обращения"
	buttonStatus      = "Статус заявки DTR"
	buttonSite        = "Сайт Dimension Science"
	buttonMyID        = "Мой Telegram ID"
)

func Start(ctx context.Context, token string, adminChatID int64, newsChat string, baseURL string, discordURL string, uploadDir string, st *store.Store, discord *discordbot.Client, logger *log.Logger) {
	token = strings.TrimSpace(token)
	if token == "" {
		if logger != nil {
			logger.Print("telegram bot disabled: TELEGRAM_BOT_TOKEN is empty")
		}
		return
	}
	if logger == nil {
		logger = log.Default()
	}

	bot := &Bot{
		token:             token,
		adminChatID:       adminChatID,
		newsChat:          normalizeTelegramChat(newsChat),
		baseURL:           strings.TrimRight(baseURL, "/"),
		discordURL:        strings.TrimSpace(discordURL),
		uploadDir:         uploadDir,
		store:             st,
		discord:           discord,
		client:            &http.Client{Timeout: 35 * time.Second},
		logger:            logger,
		pendingSupport:    make(map[int64]bool),
		pendingCheckCert:  make(map[int64]bool),
		pendingCooperate:  make(map[int64]bool),
		chaosApplications: make(map[int64]*telegramChaosApplicationDraft),
		menuState:         make(map[int64]string),
	}
	go bot.loop(ctx)
}

func (b *Bot) loop(ctx context.Context) {
	b.logger.Print("telegram bot started")
	if err := b.deleteWebhook(ctx); err != nil && ctx.Err() == nil {
		b.logger.Printf("telegram deleteWebhook: %v", err)
	}
	offset := 0
	for {
		select {
		case <-ctx.Done():
			b.logger.Print("telegram bot stopped")
			return
		default:
		}

		updates, err := b.getUpdates(ctx, offset)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			b.logger.Printf("telegram getUpdates: %v", err)
			wait(ctx, 3*time.Second)
			continue
		}

		for _, update := range updates {
			if update.UpdateID >= offset {
				offset = update.UpdateID + 1
			}
			if update.ChannelPost != nil {
				b.handleChannelPost(ctx, update.ChannelPost)
				continue
			}
			if update.Message == nil || strings.TrimSpace(update.Message.Text) == "" {
				continue
			}
			reply := b.buildReply(ctx, update.Message)
			if reply == "" {
				continue
			}
			if err := b.sendMessage(ctx, update.Message.Chat.ID, reply); err != nil && ctx.Err() == nil {
				b.logger.Printf("telegram sendMessage: %v", err)
			}
		}
	}
}

func (b *Bot) buildReply(ctx context.Context, message *telegramMessage) string {
	if message == nil {
		return ""
	}
	text := strings.TrimSpace(message.Text)
	command := strings.Fields(text)
	if len(command) == 0 {
		return ""
	}

	chatID := message.Chat.ID
	first := strings.ToLower(strings.Split(command[0], "@")[0])
	if first == "/start" || first == "/help" {
		b.deleteChaosApplicationDraft(ctx, chatID)
		b.menuState[chatID] = "main"
		return strings.Join([]string{
			"Привет! Это официальный бот Dimension Science.",
			"",
			"Здесь можно узнать о проектах, подать заявку в Chaos, проверить сертификат DTR, предложить сотрудничество или написать в поддержку.",
			"",
			"Выберите раздел в меню ниже.",
		}, "\n")
	}

	if b.isAdmin(chatID) {
		if reply := b.handleAdminMessage(ctx, text); reply != "" {
			return reply
		}
	}

	draft := b.chaosApplications[chatID]
	if draft == nil {
		draft = b.restoreChaosApplicationDraft(ctx, chatID)
	}
	if draft != nil {
		if strings.EqualFold(text, buttonBack) || strings.EqualFold(text, "Отменить") {
			b.deleteChaosApplicationDraft(ctx, chatID)
			b.menuState[chatID] = "chaos"
			return "Заполнение заявки отменено."
		}
		return b.handleChaosApplicationStep(ctx, message, draft, text)
	}

	switch strings.ToLower(text) {
	case strings.ToLower(buttonProjects):
		b.menuState[chatID] = "projects"
		return "Проекты Dimension Science:\n\nDTR — завершённый турнирный проект.\nChaos — новое игровое направление."
	case strings.ToLower(buttonDTR):
		b.menuState[chatID] = "dtr"
		return "Dimension Tournament Run (DTR)\n\nЗдесь можно проверить подлинность сертификата участника."
	case strings.ToLower(buttonChaos):
		b.menuState[chatID] = "chaos"
		return "Dimension Science: Chaos\n\nНажмите «Подать заявку», чтобы перейти к форме участия в Discord."
	case strings.ToLower(buttonBack):
		switch b.menuState[chatID] {
		case "dtr", "chaos":
			b.menuState[chatID] = "projects"
			return "Выберите проект: DTR или Chaos."
		default:
			b.menuState[chatID] = "main"
			return "Главное меню Dimension Science."
		}
	case strings.ToLower(buttonCooperation):
		b.pendingCooperate[chatID] = true
		delete(b.pendingSupport, chatID)
		return strings.Join([]string{
			"Сотрудничество с Dimension Science",
			"",
			"Мы открыты к партнёрствам, совместным игровым событиям, медиа-проектам, спонсорству и другим предложениям.",
			"Опишите идею одним сообщением. Укажите, кто вы, что предлагаете и как с вами связаться.",
		}, "\n")
	}

	switch strings.ToLower(text) {
	case strings.ToLower(buttonStatus):
		return "РћС‚РїСЂР°РІСЊ РЅРѕРјРµСЂ Р·Р°СЏРІРєРё РѕРґРЅРёРј СЃРѕРѕР±С‰РµРЅРёРµРј. РќР°РїСЂРёРјРµСЂ: #12"
	case strings.ToLower(buttonApply):
		draft := &telegramChaosApplicationDraft{Step: 1}
		b.chaosApplications[chatID] = draft
		b.saveChaosApplicationDraft(ctx, chatID, draft)
		b.menuState[chatID] = "chaos"
		return "Заявка в Dimension Science: Chaos\n\nШаг 1 из 6. Напишите ваш игровой ник в Minecraft (от 2 до 32 символов).\n\nЧтобы прервать заполнение, нажмите «Назад» или напишите «Отменить»."
	case strings.ToLower(buttonSupport):
		if b.adminChatID == 0 && !b.discordSupportEnabled() {
			return "Support is not connected yet. Contact the tournament organizer through the site contacts."
		}
		if b.isAdmin(chatID) {
			return b.openSupportTickets(ctx)
		}
		b.pendingSupport[chatID] = true
		return "Поддержка Dimension Science\n\nОпишите вопрос или проблему одним сообщением. Бот создаст тикет и отправит его команде."
	case strings.ToLower(buttonSite):
		return "РЎР°Р№С‚ С‚СѓСЂРЅРёСЂР°: " + b.baseURL
	case strings.ToLower(buttonMyID):
		return fmt.Sprintf("Р’Р°С€ Telegram ID: %d", chatID)
	case strings.ToLower(buttonOpenSupport):
		if b.isAdmin(chatID) {
			return b.openSupportTickets(ctx)
		}
	case strings.ToLower(buttonCheckCert):
		b.pendingCheckCert[chatID] = true
		delete(b.pendingSupport, chatID)
		return "Отправьте номер сертификата DTR, например: CERT-2026-123456."
	}

	if b.pendingCheckCert[chatID] {
		delete(b.pendingCheckCert, chatID)
		return b.verifyCertificateNumber(ctx, text)
	}

	if b.pendingCooperate[chatID] {
		delete(b.pendingCooperate, chatID)
		return b.createSupportTicket(ctx, message, "[Сотрудничество]\n"+text)
	}

	if strings.HasPrefix(strings.ToUpper(text), "CERT-2026-") && len(text) == 15 {
		return b.verifyCertificateNumber(ctx, text)
	}

	if b.pendingSupport[chatID] {
		delete(b.pendingSupport, chatID)
		return b.createSupportTicket(ctx, message, text)
	}

	number, ok := applicationNumberFromText(text)
	if !ok {
		return "Не удалось определить действие. Выберите нужный раздел с помощью кнопок меню."
	}

	queryCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	application, err := b.store.FindTournamentApplicationByNumber(queryCtx, number)
	if err != nil {
		return "РќРµ РїРѕР»СѓС‡РёР»РѕСЃСЊ РїСЂРѕРІРµСЂРёС‚СЊ Р·Р°СЏРІРєСѓ. РџРѕРїСЂРѕР±СѓР№ РµС‰Рµ СЂР°Р· С‡СѓС‚СЊ РїРѕР·Р¶Рµ."
	}
	if application == nil {
		return fmt.Sprintf("Р—Р°СЏРІРєР° #%d РЅРµ РЅР°Р№РґРµРЅР°. РџСЂРѕРІРµСЂСЊ РЅРѕРјРµСЂ Рё РїРѕРїСЂРѕР±СѓР№ СЃРЅРѕРІР°.", number)
	}

	status := applicationStatusText(application.Status)
	if application.Status == "approved" && application.ParticipantID != "" {
		participant, err := b.store.FindParticipantByID(queryCtx, application.ParticipantID)
		if err == nil && participant != nil && participant.Status == "blocked" {
			status = "СѓРґР°Р»РµРЅР° РёР· whitelist"
		}
	}

	lines := []string{
		fmt.Sprintf("Р—Р°СЏРІРєР° #%d", application.ApplicationNumber),
		fmt.Sprintf("Twitch: %s", application.TwitchLogin),
		fmt.Sprintf("РЎС‚Р°С‚СѓСЃ: %s", status),
	}
	switch application.Status {
	case "pending":
		lines = append(lines, "Р—Р°СЏРІРєР° РЅР° РїСЂРѕРІРµСЂРєРµ. РљРѕРіРґР° Р°РґРјРёРЅ РѕРґРѕР±СЂРёС‚ РµРµ, РѕС‚РєСЂРѕРµС‚СЃСЏ Р»РёС‡РЅС‹Р№ РєР°Р±РёРЅРµС‚. РўРѕРєРµРЅ РґР»СЏ РјРѕРґР° РїРѕРЅР°РґРѕР±РёС‚СЃСЏ 5 РёСЋРЅСЏ, РєРѕРіРґР° РЅР° СЃР°Р№С‚Рµ РїРѕСЏРІРёС‚СЃСЏ РѕС„РёС†РёР°Р»СЊРЅС‹Р№ РјРѕРґ.")
	case "approved":
		if status == "СѓРґР°Р»РµРЅР° РёР· whitelist" {
			lines = append(lines, "Р”РѕСЃС‚СѓРї РІ РєР°Р±РёРЅРµС‚ СЃРµР№С‡Р°СЃ Р·Р°РєСЂС‹С‚ Р°РґРјРёРЅРёСЃС‚СЂР°С‚РѕСЂРѕРј.")
		} else {
			lines = append(lines, "РљР°Р±РёРЅРµС‚ РѕС‚РєСЂС‹С‚: "+b.baseURL+"/login")
			lines = append(lines, "РўРѕРєРµРЅ РёР· РєР°Р±РёРЅРµС‚Р° РЅСѓР¶РµРЅ С‚РѕР»СЊРєРѕ РґР»СЏ РѕС„РёС†РёР°Р»СЊРЅРѕРіРѕ РјРѕРґР° Tournament. Р”Рѕ 5 РёСЋРЅСЏ РµРіРѕ РЅРёРєСѓРґР° РІСЃС‚Р°РІР»СЏС‚СЊ РЅРµ РЅСѓР¶РЅРѕ.")
		}
	case "rejected":
		lines = append(lines, "РњРѕР¶РЅРѕ РёСЃРїСЂР°РІРёС‚СЊ РґР°РЅРЅС‹Рµ Рё РѕС‚РїСЂР°РІРёС‚СЊ Р·Р°СЏРІРєСѓ Р·Р°РЅРѕРІРѕ: "+b.baseURL+"/apply")
	}
	return strings.Join(lines, "\n")
}

func (b *Bot) restoreChaosApplicationDraft(ctx context.Context, chatID int64) *telegramChaosApplicationDraft {
	if b.store == nil {
		return nil
	}
	stored, err := b.store.FindTelegramChaosApplicationDraft(ctx, chatID)
	if err != nil {
		b.logger.Printf("telegram restore Chaos application draft: %v", err)
		return nil
	}
	if stored == nil {
		return nil
	}
	draft := &telegramChaosApplicationDraft{
		Step: stored.Step, GameNick: stored.GameNick, Timezone: stored.Timezone,
		Experience: stored.Experience, DiscordID: stored.DiscordID,
		Motivation: stored.Motivation, Links: stored.Links,
	}
	b.chaosApplications[chatID] = draft
	b.menuState[chatID] = "chaos"
	return draft
}

func (b *Bot) saveChaosApplicationDraft(ctx context.Context, chatID int64, draft *telegramChaosApplicationDraft) {
	if b.store == nil || draft == nil {
		return
	}
	err := b.store.SaveTelegramChaosApplicationDraft(ctx, store.TelegramChaosApplicationDraft{
		ChatID: chatID, Step: draft.Step, GameNick: draft.GameNick, Timezone: draft.Timezone,
		Experience: draft.Experience, DiscordID: draft.DiscordID,
		Motivation: draft.Motivation, Links: draft.Links,
	})
	if err != nil {
		b.logger.Printf("telegram save Chaos application draft: %v", err)
	}
}

func (b *Bot) deleteChaosApplicationDraft(ctx context.Context, chatID int64) {
	delete(b.chaosApplications, chatID)
	if b.store != nil {
		if err := b.store.DeleteTelegramChaosApplicationDraft(ctx, chatID); err != nil {
			b.logger.Printf("telegram delete Chaos application draft: %v", err)
		}
	}
}

func (b *Bot) handleChaosApplicationStep(ctx context.Context, message *telegramMessage, draft *telegramChaosApplicationDraft, text string) string {
	chatID := message.Chat.ID
	length := len([]rune(strings.TrimSpace(text)))
	switch draft.Step {
	case 1:
		if length < 2 || length > 32 {
			return "Игровой ник должен содержать от 2 до 32 символов. Попробуйте ещё раз."
		}
		draft.GameNick = strings.TrimSpace(text)
		draft.Step = 2
		b.saveChaosApplicationDraft(ctx, chatID, draft)
		return "Шаг 2 из 6. Укажите ваш часовой пояс, например UTC+5."
	case 2:
		if length < 2 || length > 32 {
			return "Часовой пояс должен содержать от 2 до 32 символов. Попробуйте ещё раз."
		}
		draft.Timezone = strings.TrimSpace(text)
		draft.Step = 3
		b.saveChaosApplicationDraft(ctx, chatID, draft)
		return "Шаг 3 из 6. Сколько лет вы играете в Minecraft? Можно ответить кратко, например: 7 лет."
	case 3:
		if length < 1 || length > 50 {
			return "Ответ должен содержать от 1 до 50 символов. Например: 7 лет."
		}
		draft.Experience = strings.TrimSpace(text)
		draft.Step = 4
		b.saveChaosApplicationDraft(ctx, chatID, draft)
		return "Шаг 4 из 6. Отправьте ваш числовой Discord ID (17–20 цифр). Он нужен, чтобы после одобрения выдать роль «Игрок DS | Chaos»."
	case 4:
		value := strings.TrimSpace(text)
		if !regexp.MustCompile(`^\d{17,20}$`).MatchString(value) {
			return "Нужен именно числовой Discord ID длиной 17–20 цифр. В Discord включите режим разработчика, нажмите на свой профиль и выберите «Копировать ID»."
		}
		draft.DiscordID = value
		draft.Step = 5
		b.saveChaosApplicationDraft(ctx, chatID, draft)
		return "Шаг 5 из 6. Почему вы хотите присоединиться? Поле необязательное — напишите ответ или отправьте «-», чтобы пропустить."
	case 5:
		if text != "-" {
			if length > 700 {
				return "Ответ слишком длинный — максимум 700 символов."
			}
			draft.Motivation = strings.TrimSpace(text)
		}
		draft.Step = 6
		b.saveChaosApplicationDraft(ctx, chatID, draft)
		return "Шаг 6 из 6. Пришлите ссылки на Twitch, YouTube или профиль. Поле необязательное — отправьте «-», чтобы пропустить."
	case 6:
		if text != "-" {
			if length > 500 {
				return "Ссылки и описание слишком длинные — максимум 500 символов."
			}
			draft.Links = strings.TrimSpace(text)
		}
		draft.Step = 7
		b.saveChaosApplicationDraft(ctx, chatID, draft)
		motivation := draft.Motivation
		if motivation == "" {
			motivation = "не указано"
		}
		links := draft.Links
		if links == "" {
			links = "не указаны"
		}
		return fmt.Sprintf("Проверьте заявку:\n\nИгровой ник: %s\nЧасовой пояс: %s\nСколько играете: %s\nDiscord ID: %s\nПочему хотите присоединиться: %s\nСсылки: %s\n\nНапишите «Отправить», чтобы передать заявку команде, или «Отменить».", draft.GameNick, draft.Timezone, draft.Experience, draft.DiscordID, motivation, links)
	case 7:
		if !strings.EqualFold(strings.TrimSpace(text), "Отправить") {
			return "Для подтверждения напишите «Отправить». Чтобы начать заново, напишите «Отменить» и снова нажмите «Подать заявку»."
		}
		if b.discord == nil || b.store == nil {
			return "Сейчас отправка заявок временно недоступна. Ваши ответы сохранены — попробуйте отправить последнее сообщение ещё раз позже."
		}
		username := "Telegram"
		if message.From != nil {
			username = "Telegram: " + telegramDisplayName(*message.From)
		}
		app, err := b.discord.SubmitExternalApplication(ctx, b.store, store.DiscordApplicationInput{
			DiscordUserID: draft.DiscordID, DiscordUsername: username, ProjectKey: "chaos",
			GameNick: draft.GameNick, Timezone: draft.Timezone, Experience: draft.Experience,
			Motivation: draft.Motivation, Links: draft.Links,
		})
		if errors.Is(err, store.ErrActiveDiscordApplication) {
			b.deleteChaosApplicationDraft(ctx, chatID)
			return "У вас уже есть активная заявка Chaos. Дождитесь решения команды по ней."
		}
		if err != nil {
			b.logger.Printf("telegram Chaos application: %v", err)
			return "Не удалось отправить заявку. Ваши ответы сохранены — отправьте последнее сообщение ещё раз через несколько минут."
		}
		b.deleteChaosApplicationDraft(ctx, chatID)
		return fmt.Sprintf("Заявка DSC-%05d отправлена! Команда Dimension Science рассмотрит её в Discord. После одобрения роль «Игрок DS | Chaos» будет выдана автоматически.", app.ApplicationNumber)
	}
	b.deleteChaosApplicationDraft(ctx, chatID)
	return "Заполнение заявки сброшено. Нажмите «Подать заявку», чтобы начать заново."
}

func (b *Bot) handleAdminMessage(ctx context.Context, text string) string {
	if strings.EqualFold(text, buttonOpenSupport) {
		return b.openSupportTickets(ctx)
	}

	number, answer, ok := supportAnswerFromText(text)
	if !ok {
		return ""
	}
	if answer == "" {
		return "РћС‚РІРµС‚ РїСѓСЃС‚РѕР№. Р¤РѕСЂРјР°С‚: РћС‚РІРµС‚ #12 С‚РµРєСЃС‚ РѕС‚РІРµС‚Р°"
	}

	queryCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	ticket, err := b.store.AnswerTelegramSupportTicket(queryCtx, number, answer)
	if err != nil {
		return fmt.Sprintf("РќРµ РїРѕР»СѓС‡РёР»РѕСЃСЊ РЅР°Р№С‚Рё РёР»Рё РѕР±РЅРѕРІРёС‚СЊ РѕР±СЂР°С‰РµРЅРёРµ #%d.", number)
	}
	if ticket == nil {
		return fmt.Sprintf("РћР±СЂР°С‰РµРЅРёРµ #%d РЅРµ РЅР°Р№РґРµРЅРѕ.", number)
	}

	userReply := strings.Join([]string{
		fmt.Sprintf("РћС‚РІРµС‚ РїРѕ РѕР±СЂР°С‰РµРЅРёСЋ #%d", ticket.TicketNumber),
		"",
		ticket.Answer,
	}, "\n")
	if err := b.sendMessage(ctx, ticket.UserChatID, userReply); err != nil {
		return fmt.Sprintf("РћС‚РІРµС‚ СЃРѕС…СЂР°РЅРµРЅ, РЅРѕ РЅРµ РїРѕР»СѓС‡РёР»РѕСЃСЊ РѕС‚РїСЂР°РІРёС‚СЊ РёРіСЂРѕРєСѓ: %v", err)
	}
	return fmt.Sprintf("РћС‚РІРµС‚ РїРѕ РѕР±СЂР°С‰РµРЅРёСЋ #%d РѕС‚РїСЂР°РІР»РµРЅ РёРіСЂРѕРєСѓ.", ticket.TicketNumber)
}

func (b *Bot) createSupportTicket(ctx context.Context, message *telegramMessage, question string) string {
	question = strings.TrimSpace(question)
	if question == "" {
		b.pendingSupport[message.Chat.ID] = true
		return "Сообщение пустое. Опишите вопрос одним сообщением."
	}

	userID := int64(0)
	username := ""
	if message.From != nil {
		userID = message.From.ID
		username = telegramDisplayName(*message.From)
	}

	queryCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	ticket, err := b.store.CreateTelegramSupportTicket(queryCtx, store.TelegramSupportTicketInput{
		UserChatID:     message.Chat.ID,
		UserTelegramID: userID,
		UserUsername:   username,
		Question:       question,
	})
	if err != nil {
		return "Не удалось создать тикет. Попробуйте ещё раз немного позже."
	}

	adminMessage := strings.Join([]string{
		fmt.Sprintf("Новый тикет #%d", ticket.TicketNumber),
		"",
		"От: " + supportUserLabel(ticket),
		fmt.Sprintf("Chat ID: %d", ticket.UserChatID),
		"",
		ticket.Question,
		"",
		fmt.Sprintf("Ответить: Ответ #%d текст ответа", ticket.TicketNumber),
	}, "\n")
	if b.discordSupportEnabled() {
		b.discord.NotifySupportTicket(ctx, ticket)
	}
	if b.adminChatID == 0 {
		return strings.Join([]string{
			fmt.Sprintf("Тикет #%d отправлен команде.", ticket.TicketNumber),
			"Когда сотрудник ответит, бот пришлёт сообщение сюда.",
		}, "\n")
	}
	if err := b.sendMessage(ctx, b.adminChatID, adminMessage); err != nil {
		b.logger.Printf("telegram support forward: %v", err)
		return fmt.Sprintf("Тикет #%d создан, но уведомление сотруднику не отправилось. Тикет сохранён в системе.", ticket.TicketNumber)
	}

	return strings.Join([]string{
		fmt.Sprintf("Тикет #%d отправлен команде.", ticket.TicketNumber),
		"Когда сотрудник ответит, бот пришлёт сообщение сюда.",
	}, "\n")
}

func (b *Bot) openSupportTickets(ctx context.Context) string {
	if !b.isAdmin(b.adminChatID) {
		return "РђРґРјРёРЅСЃРєРёР№ С‡Р°С‚ РЅРµ РЅР°СЃС‚СЂРѕРµРЅ."
	}
	queryCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	tickets, err := b.store.ListOpenTelegramSupportTickets(queryCtx, 10)
	if err != nil {
		return "РќРµ РїРѕР»СѓС‡РёР»РѕСЃСЊ Р·Р°РіСЂСѓР·РёС‚СЊ РѕС‚РєСЂС‹С‚С‹Рµ РѕР±СЂР°С‰РµРЅРёСЏ."
	}
	if len(tickets) == 0 {
		return "РћС‚РєСЂС‹С‚С‹С… РѕР±СЂР°С‰РµРЅРёР№ РЅРµС‚."
	}

	lines := []string{"РћС‚РєСЂС‹С‚С‹Рµ РѕР±СЂР°С‰РµРЅРёСЏ:"}
	for _, ticket := range tickets {
		lines = append(lines,
			"",
			fmt.Sprintf("#%d РѕС‚ %s", ticket.TicketNumber, supportUserLabel(&ticket)),
			trimMultiline(ticket.Question, 220),
			fmt.Sprintf("РћС‚РІРµС‚РёС‚СЊ: РћС‚РІРµС‚ #%d С‚РµРєСЃС‚ РѕС‚РІРµС‚Р°", ticket.TicketNumber),
		)
	}
	return strings.Join(lines, "\n")
}

func (b *Bot) isAdmin(chatID int64) bool {
	return b.adminChatID != 0 && chatID == b.adminChatID
}

func (b *Bot) discordSupportEnabled() bool {
	return b.discord != nil && b.discord.SupportEnabled()
}

func (b *Bot) handleChannelPost(ctx context.Context, post *telegramMessage) {
	if post == nil || !b.isNewsChannel(post.Chat) {
		return
	}
	text, links := postTextAndLinks(post)
	if text == "" {
		text = "РќРѕРІС‹Р№ РїРѕСЃС‚ Tournament"
	}
	imageURL, err := b.savePostImageAsWebP(ctx, post)
	if err != nil && b.logger != nil {
		b.logger.Printf("telegram news image save: %v", err)
	}
	if b.store != nil {
		queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if _, err := b.store.SaveTelegramNewsPost(queryCtx, store.TelegramNewsPostInput{
			TelegramChatID:    post.Chat.ID,
			TelegramMessageID: post.MessageID,
			SourceURL:         b.telegramPostURL(post),
			Text:              text,
			ImageURL:          imageURL,
			PublishedAt:       telegramPublishedAt(post),
		}); err != nil && b.logger != nil {
			b.logger.Printf("telegram news save: %v", err)
		}
	}
	if b.discord == nil || !b.discord.NewsEnabled() {
		return
	}
	media, err := b.downloadPostMedia(ctx, post)
	if err != nil && b.logger != nil {
		b.logger.Printf("telegram news media download: %v", err)
	}
	if text == "" && media == nil {
		return
	}
	if text == "" {
		text = "РќРѕРІС‹Р№ РїРѕСЃС‚ Tournament"
	}
	b.discord.NotifyNewsPost(ctx, discordbot.NewsPost{
		Text:    text,
		Links:   links,
		Media:   media,
		Mention: true,
	})
}

func telegramPublishedAt(post *telegramMessage) time.Time {
	if post != nil && post.Date > 0 {
		return time.Unix(post.Date, 0).UTC()
	}
	return time.Now().UTC()
}

func (b *Bot) telegramPostURL(post *telegramMessage) string {
	if post == nil || post.MessageID <= 0 {
		return ""
	}
	if username := normalizeTelegramChat(post.Chat.Username); username != "" {
		return fmt.Sprintf("https://t.me/%s/%d", username, post.MessageID)
	}
	chatID := strconv.FormatInt(post.Chat.ID, 10)
	if strings.HasPrefix(chatID, "-100") {
		return fmt.Sprintf("https://t.me/c/%s/%d", strings.TrimPrefix(chatID, "-100"), post.MessageID)
	}
	return ""
}

func (b *Bot) isNewsChannel(chat telegramChat) bool {
	if b.newsChat == "" {
		return false
	}
	candidates := []string{
		normalizeTelegramChat(chat.Username),
		normalizeTelegramChat(chat.Title),
		strconv.FormatInt(chat.ID, 10),
	}
	for _, candidate := range candidates {
		if candidate != "" && candidate == b.newsChat {
			return true
		}
	}
	return false
}

func SyncPublicChannelNews(ctx context.Context, newsChat string, uploadDir string, st *store.Store, logger *log.Logger) (PublicNewsSyncResult, error) {
	channel := normalizeTelegramChat(newsChat)
	if channel == "" {
		return PublicNewsSyncResult{}, fmt.Errorf("TELEGRAM_NEWS_CHAT is empty")
	}
	if _, err := strconv.ParseInt(channel, 10, 64); err == nil {
		return PublicNewsSyncResult{}, fmt.Errorf("public Telegram channel username is required for history sync")
	}
	if st == nil {
		return PublicNewsSyncResult{}, fmt.Errorf("store is not configured")
	}

	client := &http.Client{Timeout: 35 * time.Second}
	pageURL := "https://t.me/s/" + url.PathEscape(channel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return PublicNewsSyncResult{}, err
	}
	req.Header.Set("User-Agent", "Tournament news sync")
	res, err := client.Do(req)
	if err != nil {
		return PublicNewsSyncResult{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		return PublicNewsSyncResult{}, fmt.Errorf("telegram public page status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return PublicNewsSyncResult{}, err
	}

	result := PublicNewsSyncResult{}
	chatID := syntheticTelegramChatID(channel)
	for _, postBlock := range publicPostBlocks(string(body)) {
		messageID := publicMessageID(postBlock.Ref)
		if messageID <= 0 {
			continue
		}
		if publicPostIsService(postBlock.HTML) {
			continue
		}
		result.Seen++

		publishedAt := publicPostPublishedAt(postBlock.HTML)
		text := publicPostText(postBlock.HTML)
		imageSource := publicPostImageSource(postBlock.HTML)
		imageURL := ""
		if imageSource != "" {
			if saved, err := savePublicImageAsWebP(ctx, client, uploadDir, imageSource, channel, messageID); err == nil {
				imageURL = saved
			} else if logger != nil {
				logger.Printf("telegram public news image save: %v", err)
			}
		}
		if text == "" && imageURL == "" {
			continue
		}
		if text == "" {
			text = "РќРѕРІС‹Р№ РїРѕСЃС‚ Tournament"
		}
		if _, err := st.SaveTelegramNewsPost(ctx, store.TelegramNewsPostInput{
			TelegramChatID:    chatID,
			TelegramMessageID: messageID,
			SourceURL:         fmt.Sprintf("https://t.me/%s/%d", channel, messageID),
			Text:              text,
			ImageURL:          imageURL,
			PublishedAt:       publishedAt,
		}); err != nil {
			return result, err
		}
		result.Imported++
	}
	return result, nil
}

func SyncPublicChannelNewsHistory(ctx context.Context, newsChat string, uploadDir string, st *store.Store, logger *log.Logger) (PublicNewsSyncResult, error) {
	channel := normalizeTelegramChat(newsChat)
	if channel == "" {
		return PublicNewsSyncResult{}, fmt.Errorf("TELEGRAM_NEWS_CHAT is empty")
	}
	if _, err := strconv.ParseInt(channel, 10, 64); err == nil {
		return PublicNewsSyncResult{}, fmt.Errorf("public Telegram channel username is required for history sync")
	}
	if st == nil {
		return PublicNewsSyncResult{}, fmt.Errorf("store is not configured")
	}

	client := &http.Client{Timeout: 35 * time.Second}
	result := PublicNewsSyncResult{}
	chatID := syntheticTelegramChatID(channel)
	seenPosts := map[int]bool{}
	before := 0
	for page := 0; page < 50; page++ {
		body, err := fetchPublicNewsPage(ctx, client, channel, before)
		if err != nil {
			return result, err
		}
		postBlocks := publicPostBlocks(body)
		if len(postBlocks) == 0 {
			break
		}
		minMessageID := 0
		newPosts := 0
		for _, postBlock := range postBlocks {
			messageID := publicMessageID(postBlock.Ref)
			if messageID <= 0 || seenPosts[messageID] {
				continue
			}
			seenPosts[messageID] = true
			if publicPostIsService(postBlock.HTML) {
				continue
			}
			newPosts++
			result.Seen++
			if minMessageID == 0 || messageID < minMessageID {
				minMessageID = messageID
			}

			publishedAt := publicPostPublishedAt(postBlock.HTML)
			text := publicPostText(postBlock.HTML)
			imageSource := publicPostImageSource(postBlock.HTML)
			imageURL := ""
			if imageSource != "" {
				if saved, err := savePublicImageAsWebP(ctx, client, uploadDir, imageSource, channel, messageID); err == nil {
					imageURL = saved
				} else if logger != nil {
					logger.Printf("telegram public news image save: %v", err)
				}
			}
			if text == "" && imageURL == "" {
				continue
			}
			if text == "" {
				text = "Р СњР С•Р Р†РЎвЂ№Р в„– Р С—Р С•РЎРѓРЎвЂљ Tournament"
			}
			if _, err := st.SaveTelegramNewsPost(ctx, store.TelegramNewsPostInput{
				TelegramChatID:    chatID,
				TelegramMessageID: messageID,
				SourceURL:         fmt.Sprintf("https://t.me/%s/%d", channel, messageID),
				Text:              text,
				ImageURL:          imageURL,
				PublishedAt:       publishedAt,
			}); err != nil {
				return result, err
			}
			result.Imported++
		}
		if newPosts == 0 || minMessageID <= 1 {
			break
		}
		before = minMessageID
	}
	return result, nil
}

func fetchPublicNewsPage(ctx context.Context, client *http.Client, channel string, before int) (string, error) {
	pageURL := "https://t.me/s/" + url.PathEscape(channel)
	if before > 0 {
		pageURL += "?before=" + strconv.Itoa(before)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Tournament news sync")
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		return "", fmt.Errorf("telegram public page status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func publicPostBlocks(body string) []publicPostBlock {
	matches := publicPostStartPattern.FindAllStringSubmatchIndex(body, -1)
	if len(matches) == 0 {
		return nil
	}
	blocks := make([]publicPostBlock, 0, len(matches))
	for index, match := range matches {
		if len(match) < 4 {
			continue
		}
		start := match[0]
		end := len(body)
		if index+1 < len(matches) {
			end = matches[index+1][0]
		}
		if start < 0 || end <= start || match[2] < 0 || match[3] < 0 {
			continue
		}
		blocks = append(blocks, publicPostBlock{
			Ref:  html.UnescapeString(body[match[2]:match[3]]),
			HTML: body[start:end],
		})
	}
	return blocks
}

func publicMessageID(postRef string) int {
	_, rawID, ok := strings.Cut(strings.TrimSpace(postRef), "/")
	if !ok {
		return 0
	}
	id, err := strconv.Atoi(rawID)
	if err != nil {
		return 0
	}
	return id
}

func publicPostIsService(block string) bool {
	if strings.Contains(block, " service_message ") || strings.Contains(block, `service_message`) {
		return true
	}
	text := strings.ToLower(strings.TrimSpace(publicPostText(block)))
	switch text {
	case "channel created",
		"channel photo updated",
		"channel name changed",
		"channel description changed",
		"channel username changed":
		return true
	}
	return false
}

func publicPostPublishedAt(block string) time.Time {
	match := publicPostTimePattern.FindStringSubmatch(block)
	if len(match) >= 2 {
		if parsed, err := time.Parse(time.RFC3339, html.UnescapeString(match[1])); err == nil {
			return parsed.UTC()
		}
	}
	return time.Now().UTC()
}

func publicPostText(block string) string {
	match := publicPostTextPattern.FindStringSubmatch(block)
	if len(match) < 2 {
		return ""
	}
	text := match[1]
	replacer := strings.NewReplacer(
		"<br/>", "\n",
		"<br />", "\n",
		"<br>", "\n",
		"</p>", "\n",
		"</div>", "\n",
	)
	text = replacer.Replace(text)
	text = htmlTagPattern.ReplaceAllString(text, "")
	text = html.UnescapeString(text)
	lines := strings.Split(text, "\n")
	clean := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			clean = append(clean, line)
		}
	}
	return strings.Join(clean, "\n")
}

func publicPostImageSource(block string) string {
	match := publicPostPhotoPattern.FindStringSubmatch(block)
	if len(match) < 2 {
		return ""
	}
	imageURL := strings.TrimSpace(html.UnescapeString(match[1]))
	imageURL = strings.Trim(imageURL, `'"`)
	if strings.HasPrefix(imageURL, "//") {
		imageURL = "https:" + imageURL
	}
	return imageURL
}

func savePublicImageAsWebP(ctx context.Context, client *http.Client, uploadDir string, imageURL string, channel string, messageID int) (string, error) {
	if strings.TrimSpace(imageURL) == "" {
		return "", nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Tournament news sync")
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		return "", fmt.Errorf("telegram public image status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, maxNewsImageBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxNewsImageBytes {
		return "", fmt.Errorf("telegram public image is too large")
	}
	webpData, err := encodeTelegramImageWebP(data, res.Header.Get("Content-Type"))
	if err != nil {
		return "", err
	}
	dir := filepath.Join(uploadDir, "news")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	fileName := fmt.Sprintf("telegram_public_%s_%d.webp", safeNewsFilePart(channel), messageID)
	if err := os.WriteFile(filepath.Join(dir, fileName), webpData, 0o644); err != nil {
		return "", err
	}
	return "/uploads/news/" + fileName, nil
}

func syntheticTelegramChatID(channel string) int64 {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(strings.ToLower(strings.TrimSpace(channel))))
	value := int64(hash.Sum64() & ((1 << 62) - 1))
	if value == 0 {
		value = 1
	}
	return -value
}

func safeNewsFilePart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer(" ", "-", "/", "-", "\\", "-", ".", "-").Replace(value)
	value = regexp.MustCompile(`[^a-z0-9_-]+`).ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "channel"
	}
	return value
}

func postTextAndLinks(post *telegramMessage) (string, []string) {
	if post == nil {
		return "", nil
	}
	text := strings.TrimSpace(post.Text)
	entities := post.Entities
	if text == "" {
		text = strings.TrimSpace(post.Caption)
		entities = post.CaptionEntities
	}
	links := entityLinks(entities)
	links = append(links, replyMarkupLinks(post.ReplyMarkup)...)
	return text, links
}

func entityLinks(entities []telegramMessageEntity) []string {
	links := make([]string, 0, len(entities))
	for _, entity := range entities {
		if strings.TrimSpace(entity.URL) != "" {
			links = append(links, entity.URL)
		}
	}
	return links
}

func replyMarkupLinks(markup *telegramReplyMarkup) []string {
	if markup == nil {
		return nil
	}
	links := []string{}
	for _, row := range markup.InlineKeyboard {
		for _, button := range row {
			if strings.TrimSpace(button.URL) != "" {
				if strings.TrimSpace(button.Text) != "" {
					links = append(links, button.Text+": "+button.URL)
				} else {
					links = append(links, button.URL)
				}
			}
		}
	}
	return links
}

func (b *Bot) savePostImageAsWebP(ctx context.Context, post *telegramMessage) (string, error) {
	if post == nil {
		return "", nil
	}
	fileID, contentType, fileSize := postImageCandidate(post)
	if fileID == "" {
		return "", nil
	}
	if fileSize > maxNewsImageBytes {
		return "", fmt.Errorf("telegram image is too large for news: %d bytes", fileSize)
	}
	telegramFile, err := b.getTelegramFile(ctx, fileID)
	if err != nil {
		return "", err
	}
	if telegramFile.FileSize > maxNewsImageBytes {
		return "", fmt.Errorf("telegram image file is too large for news: %d bytes", telegramFile.FileSize)
	}
	data, err := b.downloadTelegramFileLimited(ctx, telegramFile.FilePath, maxNewsImageBytes)
	if err != nil {
		return "", err
	}
	webpData, err := encodeTelegramImageWebP(data, contentType)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(b.uploadDir, "news")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	fileName := fmt.Sprintf("telegram_%d_%d.webp", post.Chat.ID, post.MessageID)
	if err := os.WriteFile(filepath.Join(dir, fileName), webpData, 0o644); err != nil {
		return "", err
	}
	return "/uploads/news/" + fileName, nil
}

func postImageCandidate(post *telegramMessage) (fileID, contentType string, fileSize int) {
	if len(post.Photo) > 0 {
		best := post.Photo[0]
		for _, photo := range post.Photo[1:] {
			bestPixels := best.Width * best.Height
			photoPixels := photo.Width * photo.Height
			if photoPixels > bestPixels || (photoPixels == bestPixels && photo.FileSize > best.FileSize) {
				best = photo
			}
		}
		return best.FileID, "image/jpeg", best.FileSize
	}
	if post.Document != nil && strings.HasPrefix(strings.ToLower(post.Document.MimeType), "image/") {
		return post.Document.FileID, post.Document.MimeType, post.Document.FileSize
	}
	return "", "", 0
}

func encodeTelegramImageWebP(data []byte, contentType string) ([]byte, error) {
	var (
		img image.Image
		err error
	)
	if strings.EqualFold(contentType, "image/webp") || isWebP(data) {
		img, err = webp.Decode(bytes.NewReader(data))
	} else {
		img, _, err = image.Decode(bytes.NewReader(data))
	}
	if err != nil {
		return nil, err
	}
	return webp.EncodeRGBA(img, 82)
}

func isWebP(data []byte) bool {
	return len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP"
}

func (b *Bot) downloadPostMedia(ctx context.Context, post *telegramMessage) (*discordbot.MediaAttachment, error) {
	if post == nil {
		return nil, nil
	}

	fileID, fileName, contentType, fileSize := postMediaCandidate(post)
	if fileID == "" {
		return nil, nil
	}
	if fileSize > maxDiscordMediaBytes {
		return nil, fmt.Errorf("telegram media is too large for discord: %d bytes", fileSize)
	}

	telegramFile, err := b.getTelegramFile(ctx, fileID)
	if err != nil {
		return nil, err
	}
	if telegramFile.FileSize > maxDiscordMediaBytes {
		return nil, fmt.Errorf("telegram file is too large for discord: %d bytes", telegramFile.FileSize)
	}
	if fileName == "" {
		fileName = filepath.Base(telegramFile.FilePath)
	}
	fileName = safeDiscordFileName(fileName, contentType)
	data, err := b.downloadTelegramFile(ctx, telegramFile.FilePath)
	if err != nil {
		return nil, err
	}
	return &discordbot.MediaAttachment{
		FileName:    fileName,
		ContentType: contentType,
		Data:        data,
	}, nil
}

func postMediaCandidate(post *telegramMessage) (fileID, fileName, contentType string, fileSize int) {
	if len(post.Photo) > 0 {
		best := post.Photo[0]
		for _, photo := range post.Photo[1:] {
			bestPixels := best.Width * best.Height
			photoPixels := photo.Width * photo.Height
			if photoPixels > bestPixels || (photoPixels == bestPixels && photo.FileSize > best.FileSize) {
				best = photo
			}
		}
		return best.FileID, "telegram-photo.jpg", "image/jpeg", best.FileSize
	}
	if post.Animation != nil {
		return post.Animation.FileID, post.Animation.FileName, mimeOrDefault(post.Animation.MimeType, "image/gif"), post.Animation.FileSize
	}
	if post.Document != nil && strings.HasPrefix(strings.ToLower(post.Document.MimeType), "image/") {
		return post.Document.FileID, post.Document.FileName, post.Document.MimeType, post.Document.FileSize
	}
	if post.Video != nil {
		return post.Video.FileID, post.Video.FileName, mimeOrDefault(post.Video.MimeType, "video/mp4"), post.Video.FileSize
	}
	return "", "", "", 0
}

func (b *Bot) getTelegramFile(ctx context.Context, fileID string) (telegramFile, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getFile?file_id=%s", b.token, fileID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return telegramFile{}, err
	}
	res, err := b.client.Do(req)
	if err != nil {
		return telegramFile{}, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return telegramFile{}, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return telegramFile{}, fmt.Errorf("telegram getFile status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload telegramFileResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return telegramFile{}, err
	}
	if !payload.OK {
		return telegramFile{}, fmt.Errorf("telegram getFile: %s", payload.Description)
	}
	return payload.Result, nil
}

func (b *Bot) downloadTelegramFile(ctx context.Context, filePath string) ([]byte, error) {
	return b.downloadTelegramFileLimited(ctx, filePath, maxDiscordMediaBytes)
}

func (b *Bot) downloadTelegramFileLimited(ctx context.Context, filePath string, limit int) ([]byte, error) {
	filePath = strings.TrimLeft(filePath, "/")
	if filePath == "" {
		return nil, fmt.Errorf("telegram file path is empty")
	}
	if limit <= 0 {
		limit = maxDiscordMediaBytes
	}
	url := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", b.token, filePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		return nil, fmt.Errorf("telegram file status %d: %s", res.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("telegram file is too large")
	}
	return data, nil
}

func mimeOrDefault(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func safeDiscordFileName(fileName, contentType string) string {
	fileName = filepath.Base(strings.TrimSpace(fileName))
	if fileName == "" || fileName == "." || fileName == string(filepath.Separator) {
		fileName = "telegram-media"
	}
	if filepath.Ext(fileName) == "" {
		switch strings.ToLower(contentType) {
		case "image/jpeg":
			fileName += ".jpg"
		case "image/png":
			fileName += ".png"
		case "image/gif":
			fileName += ".gif"
		case "image/webp":
			fileName += ".webp"
		case "video/mp4":
			fileName += ".mp4"
		default:
			fileName += ".bin"
		}
	}
	return strings.NewReplacer(" ", "-", "\n", "-", "\r", "-", "\t", "-").Replace(fileName)
}

func normalizeTelegramChat(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimPrefix(value, "https://t.me/")
	value = strings.TrimPrefix(value, "http://t.me/")
	value = strings.TrimPrefix(value, "t.me/")
	value = strings.TrimPrefix(value, "@")
	value = strings.Trim(value, "/")
	return value
}

func applicationNumberFromText(text string) (int64, bool) {
	parts := strings.Fields(strings.TrimSpace(text))
	if len(parts) == 0 {
		return 0, false
	}

	candidate := parts[0]
	command := strings.ToLower(strings.Split(candidate, "@")[0])
	if strings.HasPrefix(command, "/status") {
		if len(parts) < 2 {
			return 0, false
		}
		candidate = parts[1]
	}

	candidate = strings.TrimSpace(strings.TrimPrefix(candidate, "#"))
	number, err := strconv.ParseInt(candidate, 10, 64)
	if err != nil || number <= 0 {
		return 0, false
	}
	return number, true
}

func replyKeyboard() map[string]any {
	return map[string]any{
		"keyboard": [][]map[string]string{
			{{"text": buttonProjects}},
			{{"text": buttonSupport}, {"text": buttonCooperation}},
		},
		"resize_keyboard":   true,
		"one_time_keyboard": false,
		"selective":         false,
	}
}

func projectsKeyboard() map[string]any {
	return keyboardRows([][]string{{buttonDTR, buttonChaos}, {buttonBack}})
}

func dtrKeyboard() map[string]any {
	return keyboardRows([][]string{{buttonCheckCert}, {buttonBack}})
}

func chaosKeyboard() map[string]any {
	return keyboardRows([][]string{{buttonApply}, {buttonBack}})
}

func keyboardRows(rows [][]string) map[string]any {
	keyboard := make([][]map[string]string, 0, len(rows))
	for _, row := range rows {
		buttons := make([]map[string]string, 0, len(row))
		for _, label := range row {
			buttons = append(buttons, map[string]string{"text": label})
		}
		keyboard = append(keyboard, buttons)
	}
	return map[string]any{"keyboard": keyboard, "resize_keyboard": true, "one_time_keyboard": false, "selective": false}
}

func adminReplyKeyboard() map[string]any {
	return map[string]any{
		"keyboard": [][]map[string]string{
			{{"text": buttonOpenSupport}},
			{{"text": buttonProjects}},
			{{"text": buttonSupport}, {"text": buttonCooperation}},
		},
		"resize_keyboard":   true,
		"one_time_keyboard": false,
		"selective":         false,
	}
}

func supportAnswerFromText(text string) (int64, string, bool) {
	parts := strings.Fields(strings.TrimSpace(text))
	if len(parts) < 3 {
		return 0, "", false
	}
	if !strings.EqualFold(parts[0], "РѕС‚РІРµС‚") && !strings.EqualFold(parts[0], "answer") {
		return 0, "", false
	}
	numberText := strings.TrimPrefix(parts[1], "#")
	number, err := strconv.ParseInt(numberText, 10, 64)
	if err != nil || number <= 0 {
		return 0, "", false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(text, parts[0]))
	rest = strings.TrimSpace(strings.TrimPrefix(rest, parts[1]))
	return number, strings.TrimSpace(rest), true
}

func telegramDisplayName(user telegramUser) string {
	if user.Username != "" {
		return "@" + user.Username
	}
	name := strings.TrimSpace(strings.TrimSpace(user.FirstName + " " + user.LastName))
	if name != "" {
		return name
	}
	if user.ID != 0 {
		return strconv.FormatInt(user.ID, 10)
	}
	return "unknown"
}

func supportUserLabel(ticket *store.TelegramSupportTicket) string {
	if ticket == nil {
		return "unknown"
	}
	if ticket.UserUsername != "" {
		return ticket.UserUsername
	}
	if ticket.UserTelegramID != 0 {
		return strconv.FormatInt(ticket.UserTelegramID, 10)
	}
	return strconv.FormatInt(ticket.UserChatID, 10)
}

func trimMultiline(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len([]rune(value)) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit]) + "..."
}

func applicationStatusText(status string) string {
	switch status {
	case "pending":
		return "РЅР° РїСЂРѕРІРµСЂРєРµ"
	case "approved":
		return "РѕРґРѕР±СЂРµРЅР°"
	case "rejected":
		return "РѕС‚РєР»РѕРЅРµРЅР°"
	default:
		if status == "" {
			return "РЅРµРёР·РІРµСЃС‚РЅРѕ"
		}
		return status
	}
}

func (b *Bot) getUpdates(ctx context.Context, offset int) ([]telegramUpdate, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?timeout=25&offset=%d", b.token, offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	res, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("telegram status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload updateResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if !payload.OK {
		return nil, fmt.Errorf("telegram response: %s", payload.Description)
	}
	return payload.Result, nil
}

func (b *Bot) sendMessage(ctx context.Context, chatID int64, text string) error {
	payload := map[string]any{
		"chat_id":                  chatID,
		"text":                     text,
		"disable_web_page_preview": true,
		"reply_markup":             b.keyboardFor(chatID),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", b.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		return fmt.Errorf("telegram status %d: %s", res.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	return nil
}

func (b *Bot) keyboardFor(chatID int64) map[string]any {
	switch b.menuState[chatID] {
	case "projects":
		return projectsKeyboard()
	case "dtr":
		return dtrKeyboard()
	case "chaos":
		return chaosKeyboard()
	}
	if b.isAdmin(chatID) {
		return adminReplyKeyboard()
	}
	return replyKeyboard()
}

func (b *Bot) deleteWebhook(ctx context.Context) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/deleteWebhook", b.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	res, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		responseBody, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		return fmt.Errorf("telegram status %d: %s", res.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	return nil
}

func wait(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func (b *Bot) verifyCertificateNumber(ctx context.Context, text string) string {
	number := strings.TrimSpace(text)
	number = strings.ToUpper(number)

	matched, _ := regexp.MatchString(`^CERT-2026-\d{6}$`, number)
	if !matched {
		return "Неверный формат номера сертификата. Пример: CERT-2026-123456"
	}

	queryCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	p, certStatus, available, err := b.store.FindParticipantByCertificateNumber(queryCtx, number)
	if err != nil {
		return "Не удалось выполнить проверку. Попробуйте позже."
	}

	if !available || p == nil {
		return fmt.Sprintf("Сертификат %s не найден или ещё не опубликован.", number)
	}

	displayName := p.TwitchDisplayName
	if displayName == "" {
		displayName = p.TwitchLogin
	}

	lines := []string{
		"✅ Сертификат DTR подтверждён!",
		"",
		fmt.Sprintf("Номер: %s", number),
		fmt.Sprintf("Статус: %s", certStatus),
		fmt.Sprintf("Участник: %s", displayName),
	}

	if p.MinecraftNick != "" {
		lines = append(lines, fmt.Sprintf("Minecraft ник: %s", p.MinecraftNick))
	}

	if p.BestTimeMS != nil {
		ms := *p.BestTimeMS
		seconds := ms / 1000
		totalMinutes := seconds / 60
		hours := totalMinutes / 60
		minutes := totalMinutes % 60
		var formattedTime string
		if hours > 0 {
			formattedTime = fmt.Sprintf("%d:%02d:%02d.%03d", hours, minutes, seconds%60, ms%1000)
		} else {
			formattedTime = fmt.Sprintf("%02d:%02d.%03d", minutes, seconds%60, ms%1000)
		}
		lines = append(lines, fmt.Sprintf("Рекорд на турнире: %s", formattedTime))
	}

	return strings.Join(lines, "\n")
}
