package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"capybot/internal/bot"
	"capybot/internal/core"
	"capybot/internal/i18n"

	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
	tb "gopkg.in/telebot.v4"
)

// Version is the current bot version
const Version = "1.2.0"

// GitHubRepo is the repository URL
const GitHubRepo = "https://github.com/arsmotorin/capybot"

// Handler aggregates bot dependencies
type Handler struct {
	bot            *tb.Bot
	state          core.UserState
	quiz           core.QuizInterface
	blacklist      core.BlacklistInterface
	adminChatID    int64
	violations     map[int64]int
	adminHandler   core.AdminHandlerInterface
	featureHandler core.FeatureHandlerInterface
	ratingHandler  *bot.RatingHandler
}

func main() {
	logrus.WithField("version", Version).Info("Bot is starting...")
	_ = godotenv.Load()

	// Initialize localization
	langMap := map[string]i18n.Lang{
		"pl": i18n.PL, "en": i18n.EN, "ru": i18n.RU, "uk": i18n.UK, "be": i18n.BE,
	}
	defaultLang := i18n.PL
	if lang, ok := langMap[os.Getenv("DEFAULT_LANG")]; ok {
		defaultLang = lang
	}
	if err := i18n.Init(defaultLang); err != nil {
		logrus.WithError(err).Fatal("Failed to initialize i18n")
	}

	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		logrus.Fatal("BOT_TOKEN missing")
	}
	adminChatID, err := strconv.ParseInt(os.Getenv("ADMIN_CHAT_ID"), 10, 64)
	if err != nil {
		logrus.Fatal("ADMIN_CHAT_ID invalid or missing")
	}
	b, err := tb.NewBot(tb.Settings{
		Token:  token,
		Poller: &tb.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		logrus.WithError(err).Fatal("bot create failed")
	}
	h := NewHandler(b, adminChatID)
	h.Register()
	logrus.WithField("admin_chat_id", adminChatID).Info("Bot started")
	b.Start()
}

// NewHandler wires dependencies
func NewHandler(b *tb.Bot, adminChatID int64) *Handler {
	violations := make(map[int64]int)
	state := core.NewState()
	quiz := bot.DefaultQuiz()
	black := bot.NewBlacklist("blacklist.json")

	h := &Handler{bot: b, state: state, quiz: quiz, blacklist: black, adminChatID: adminChatID, violations: violations}

	// Buttons
	btns := struct{ Student, Guest, Ads tb.InlineButton }{
		Student: bot.StudentButton(),
		Guest:   bot.GuestButton(),
		Ads:     bot.AdsButton(),
	}

	// Admin
	adminHandler := bot.NewAdminHandler(b, black, adminChatID, violations)
	h.adminHandler = adminHandler

	// Feature
	featureHandler := bot.NewFeatureHandler(b, state, quiz, black, adminChatID, violations, adminHandler, btns)
	h.featureHandler = featureHandler

	// Rating
	ratingHandler := bot.NewRatingHandler(b, adminChatID, adminHandler)
	h.ratingHandler = ratingHandler

	return h
}

// Register sets handlers
func (h *Handler) Register() {
	h.bot.Handle(tb.OnUserJoined, h.featureHandler.HandleUserJoined)
	h.bot.Handle(tb.OnUserLeft, h.featureHandler.HandleUserLeft)
	h.bot.Handle("/rate", h.ratingHandler.HandleRate)
	h.bot.Handle("/ratings", h.ratingHandler.HandleRatings)
	h.ratingHandler.RegisterHandlers(h.bot)

	h.featureHandler.RegisterQuizHandlers(h.bot)
	h.bot.Handle("/banword", h.adminHandler.HandleBan)
	h.bot.Handle("/unbanword", h.adminHandler.HandleUnban)
	h.bot.Handle("/listbanword", h.adminHandler.HandleListBan)
	h.bot.Handle("/spamban", h.adminHandler.HandleSpamBan)
	h.bot.Handle("/ping", h.featureHandler.RateLimit(h.featureHandler.HandlePing))
	h.bot.Handle("/start", h.featureHandler.HandleStart)
	h.bot.Handle("/version", h.handleVersion)
	h.bot.Handle(tb.OnText, h.handleTextMessage)
	h.setBotCommands()
}

// handleVersion returns bot version
func (h *Handler) handleVersion(c tb.Context) error {
	return c.Send(fmt.Sprintf("🤖 Bot version: %s\n🔗 GitHub: %s", Version, GitHubRepo))
}

// handleTextMessage handles text messages
func (h *Handler) handleTextMessage(c tb.Context) error {
	if c.Chat().Type == tb.ChatPrivate {
		// Check rating input first
		if h.ratingHandler.HandleRateText(c) {
			return nil
		}
		if h.ratingHandler.HandleSearchText(c) {
			return nil
		}
		if err := h.featureHandler.HandlePrivateMessage(c); err != nil {
			return err
		}
	}
	return h.featureHandler.FilterMessage(c)
}

// setBotCommands sets bot commands
func (h *Handler) setBotCommands() {
	languages := []i18n.Lang{i18n.PL, i18n.EN, i18n.RU, i18n.UK, i18n.BE}

	for _, lang := range languages {
		msgs := i18n.Get().T(lang)
		commands := []tb.Command{
			{Text: "start", Description: msgs.Commands.StartDesc},
			{Text: "ping", Description: msgs.Commands.PingDesc},
			{Text: "version", Description: msgs.Commands.VersionDesc},
			{Text: "rate", Description: msgs.Commands.RateDesc},
			{Text: "ratings", Description: msgs.Commands.RatingsDesc},
		}

		// Set commands with language code
		_ = h.bot.SetCommands(commands, tb.CommandScope{Type: tb.CommandScopeDefault}, string(lang))
	}

	// Set default commands (no language specified) for backward compatibility
	msgs := i18n.Get().T(i18n.PL)
	_ = h.bot.SetCommands([]tb.Command{
		{Text: "ping", Description: msgs.Commands.PingDesc},
		{Text: "version", Description: msgs.Commands.VersionDesc},
		{Text: "rate", Description: msgs.Commands.RateDesc},
		{Text: "ratings", Description: msgs.Commands.RatingsDesc},
		{Text: "banword", Description: msgs.Commands.BanwordDesc},
		{Text: "unbanword", Description: msgs.Commands.UnbanwordDesc},
		{Text: "listbanword", Description: msgs.Commands.ListbanwordDesc},
		{Text: "spamban", Description: msgs.Commands.SpambanDesc},
	})
}
