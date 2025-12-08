package bot

import (
	"capybot/internal/core"

	tb "gopkg.in/telebot.v4"
)

// Type aliases for core interfaces
type (
	QuestionInterface     = core.QuestionInterface
	QuizInterface         = core.QuizInterface
	BlacklistInterface    = core.BlacklistInterface
	AdminHandlerInterface = core.AdminHandlerInterface
)

// FeatureHandlerInterface lists feature methods
type FeatureHandlerInterface interface {
	OnlyNewbies(handler func(tb.Context) error) func(tb.Context) error
	SendOrEdit(chat *tb.Chat, msg *tb.Message, text string, rm *tb.ReplyMarkup) *tb.Message
	SetUserRestriction(chat *tb.Chat, user *tb.User, allowAll bool)
	HandleUserJoined(c tb.Context) error
	HandleUserLeft(c tb.Context) error
	HandleStudent(c tb.Context) error
	HandleGuest(c tb.Context) error
	HandleAds(c tb.Context) error
	HandlePing(c tb.Context) error
	HandleStart(c tb.Context) error
	HandlePrivateMessage(c tb.Context) error
	RateLimit(handler func(tb.Context) error) func(tb.Context) error
	RegisterQuizHandlers(bot *tb.Bot)
	CreateQuizHandler(i int, q QuestionInterface, btn tb.InlineButton) func(tb.Context) error
	FilterMessage(c tb.Context) error
}
