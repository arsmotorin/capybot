package i18n

import (
	"fmt"
	"os"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/sirupsen/logrus"
)

// Lang represents available language
type Lang string

const (
	PL Lang = "pl"
	EN Lang = "en"
	RU Lang = "ru"
	UK Lang = "uk"
	BE Lang = "be"
)

// Messages holds all translations
type Messages struct {
	Welcome struct {
		Greeting             string `toml:"greeting"`
		GreetingWithUsername string `toml:"greeting_with_username"`
		ChooseOption         string `toml:"choose_option"`
	} `toml:"welcome"`
	Buttons struct {
		Student       string `toml:"student"`
		Guest         string `toml:"guest"`
		Ads           string `toml:"ads"`
		NotYourButton string `toml:"not_your_button"`
	} `toml:"buttons"`
	Quiz struct {
		VerificationPassed string `toml:"verification_passed"`
		VerificationFailed string `toml:"verification_failed"`
		Question1          string `toml:"question_1"`
		Question2          string `toml:"question_2"`
		Question3          string `toml:"question_3"`
	} `toml:"quiz"`
	Guest struct {
		CanWrite string `toml:"can_write"`
	} `toml:"guest"`
	Ads struct {
		Message string `toml:"message"`
	} `toml:"ads"`
	Ping struct {
		Pong        string `toml:"pong"`
		PongWithMs  string `toml:"pong_with_ms"`
		PrivateOnly string `toml:"private_only"`
	} `toml:"ping"`
	RateLimit struct {
		TooFast string `toml:"too_fast"`
	} `toml:"ratelimit"`
	Filter struct {
		Warning string `toml:"warning"`
	} `toml:"filter"`
	Admin struct {
		BanCommandAdminOnly     string `toml:"ban_command_admin_only"`
		BanUsage                string `toml:"ban_usage"`
		BanAdded                string `toml:"ban_added"`
		UnbanCommandAdminOnly   string `toml:"unban_command_admin_only"`
		UnbanUsage              string `toml:"unban_usage"`
		UnbanNotFound           string `toml:"unban_not_found"`
		UnbanRemoved            string `toml:"unban_removed"`
		ListCommandAdminOnly    string `toml:"list_command_admin_only"`
		ListEmpty               string `toml:"list_empty"`
		ListHeader              string `toml:"list_header"`
		SpambanCommandAdminOnly string `toml:"spamban_command_admin_only"`
		SpambanUserNotFound     string `toml:"spamban_user_not_found"`
		SpambanCannotBanAdmin   string `toml:"spamban_cannot_ban_admin"`
		SpambanSuccess          string `toml:"spamban_success"`
	} `toml:"admin"`
	Start struct {
		Greeting string `toml:"greeting"`
	} `toml:"start"`
	Commands struct {
		StartDesc       string `toml:"start_desc"`
		PingDesc        string `toml:"ping_desc"`
		VersionDesc     string `toml:"version_desc"`
		BanwordDesc     string `toml:"banword_desc"`
		UnbanwordDesc   string `toml:"unbanword_desc"`
		ListbanwordDesc string `toml:"listbanword_desc"`
		SpambanDesc     string `toml:"spamban_desc"`
		RateDesc        string `toml:"rate_desc"`
		RatingsDesc     string `toml:"ratings_desc"`
	} `toml:"commands"`
	Rating struct {
		ChooseType      string `toml:"choose_type"`
		EnterName       string `toml:"enter_name"`
		InvalidName     string `toml:"invalid_name"`
		ChooseScore     string `toml:"choose_score"`
		EnterReview     string `toml:"enter_review"`
		ReviewTooShort  string `toml:"review_too_short"`
		ReviewTooLong   string `toml:"review_too_long"`
		ConfirmReview   string `toml:"confirm_review"`
		Submitted       string `toml:"submitted"`
		Cancelled       string `toml:"cancelled"`
		Blocked         string `toml:"blocked"`
		ReviewApproved  string `toml:"review_approved"`
		ReviewRejected  string `toml:"review_rejected"`
		NoReviews       string `toml:"no_reviews"`
		NoSearchResults string `toml:"no_search_results"`
		ListHeader      string `toml:"list_header"`
		SearchPrompt    string `toml:"search_prompt"`
		BtnPublic       string `toml:"btn_public"`
		BtnAnonymous    string `toml:"btn_anonymous"`
		BtnCancel       string `toml:"btn_cancel"`
		BtnSubmit       string `toml:"btn_submit"`
		BtnApprove      string `toml:"btn_approve"`
		BtnReject       string `toml:"btn_reject"`
		BtnBlock        string `toml:"btn_block"`
		BtnPrev         string `toml:"btn_prev"`
		BtnNext         string `toml:"btn_next"`
		BtnSearch       string `toml:"btn_search"`
		Sender          string `toml:"sender"`
		Professor       string `toml:"professor"`
		Score           string `toml:"score"`
		ReviewLabel     string `toml:"review_label"`
		Anonymous       string `toml:"anonymous"`
		Public          string `toml:"public"`
		TypeLabel       string `toml:"type_label"`
		NewReviewAdmin  string `toml:"new_review_admin"`
		StatusApproved  string `toml:"status_approved"`
		StatusRejected  string `toml:"status_rejected"`
		StatusBlocked   string `toml:"status_blocked"`
	} `toml:"rating"`
}

// Localizer manages translations
type Localizer struct {
	mu          sync.RWMutex
	messages    map[Lang]*Messages
	defaultLang Lang
}

var globalLocalizer *Localizer

// Init initializes localization
func Init(defaultLang Lang) error {
	globalLocalizer = &Localizer{
		messages:    make(map[Lang]*Messages),
		defaultLang: defaultLang,
	}

	// Load all languages
	languages := []Lang{PL, EN, RU, UK, BE}
	for _, lang := range languages {
		if err := globalLocalizer.loadLanguage(lang); err != nil {
			logrus.WithError(err).WithField("lang", lang).Warn("Failed to load language")
		}
	}

	return nil
}

// loadLanguage loads a language file
func (l *Localizer) loadLanguage(lang Lang) error {
	path := fmt.Sprintf("locales/%s.toml", lang)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var msgs Messages
	if err := toml.Unmarshal(data, &msgs); err != nil {
		return err
	}

	l.mu.Lock()
	l.messages[lang] = &msgs
	l.mu.Unlock()

	logrus.WithField("lang", lang).Info("Language loaded")
	return nil
}

// Get returns localizer instance
func Get() *Localizer {
	return globalLocalizer
}

// T returns messages for language
func (l *Localizer) T(lang Lang) *Messages {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if msgs, ok := l.messages[lang]; ok {
		return msgs
	}
	return l.messages[l.defaultLang]
}

// SetDefault sets default language
func (l *Localizer) SetDefault(lang Lang) {
	l.mu.Lock()
	l.defaultLang = lang
	l.mu.Unlock()
}

// GetDefault returns default language
func (l *Localizer) GetDefault() Lang {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.defaultLang
}
