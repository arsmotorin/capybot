package bot

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"capybot/internal/i18n"

	"github.com/sirupsen/logrus"
	tb "gopkg.in/telebot.v4"
)

// RatingStep represents the current step in the rating flow
type RatingStep int

const (
	StepNone RatingStep = iota
	StepChooseType
	StepEnterName
	StepChooseScore
	StepEnterReview
	StepConfirm
)

// Review represents a single professor review
type Review struct {
	ID          int    `json:"id"`
	UserID      int64  `json:"user_id"`
	Username    string `json:"username"`
	IsAnonymous bool   `json:"is_anonymous"`
	Professor   string `json:"professor"`
	Score       int    `json:"score"`
	Text        string `json:"text"`
	Status      string `json:"status"` // Pending, approved, rejected
	CreatedAt   int64  `json:"created_at"`
}

// RatingSession holds a user's current rating session
type RatingSession struct {
	Step        RatingStep
	IsAnonymous bool
	Professor   string
	Score       int
	Text        string
	MessageID   int
}

// RatingStore manages reviews persistence
type RatingStore struct {
	mu           sync.RWMutex
	Reviews      []Review `json:"reviews"`
	BlockedUsers []int64  `json:"blocked_users"`
	NextID       int      `json:"next_id"`
	file         string
}

// RatingHandler manages rating feature
type RatingHandler struct {
	bot          *tb.Bot
	store        *RatingStore
	sessions     map[int64]*RatingSession
	sessionsMu   sync.RWMutex
	adminChatID  int64
	adminHandler *AdminHandler
}

// NewRatingStore creates a new rating store
func NewRatingStore(file string) *RatingStore {
	_ = os.MkdirAll("data", 0755)
	rs := &RatingStore{
		Reviews:      make([]Review, 0),
		BlockedUsers: make([]int64, 0),
		NextID:       1,
		file:         file,
	}
	rs.load()
	return rs
}

func (rs *RatingStore) load() {
	data, err := os.ReadFile(rs.file)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, rs)
	if rs.Reviews == nil {
		rs.Reviews = make([]Review, 0)
	}
	if rs.BlockedUsers == nil {
		rs.BlockedUsers = make([]int64, 0)
	}
}

func (rs *RatingStore) save() {
	data, err := json.MarshalIndent(rs, "", "  ")
	if err != nil {
		logrus.WithError(err).Error("rating store marshal")
		return
	}
	if err := os.WriteFile(rs.file, data, 0644); err != nil {
		logrus.WithError(err).Error("rating store write")
	}
}

// AddReview adds a new review
func (rs *RatingStore) AddReview(r Review) int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	r.ID = rs.NextID
	rs.NextID++
	r.CreatedAt = time.Now().Unix()
	rs.Reviews = append(rs.Reviews, r)
	rs.save()
	return r.ID
}

// GetReview returns review by ID
func (rs *RatingStore) GetReview(id int) *Review {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	for i := range rs.Reviews {
		if rs.Reviews[i].ID == id {
			return &rs.Reviews[i]
		}
	}
	return nil
}

// UpdateReviewStatus updates review status
func (rs *RatingStore) UpdateReviewStatus(id int, status string) bool {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for i := range rs.Reviews {
		if rs.Reviews[i].ID == id {
			rs.Reviews[i].Status = status
			rs.save()
			return true
		}
	}
	return false
}

// GetApprovedReviews returns all approved reviews
func (rs *RatingStore) GetApprovedReviews() []Review {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	result := make([]Review, 0)
	for _, r := range rs.Reviews {
		if r.Status == "approved" {
			result = append(result, r)
		}
	}
	return result
}

// SearchReviews searches reviews by professor name
func (rs *RatingStore) SearchReviews(query string) []Review {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	query = strings.ToLower(query)
	result := make([]Review, 0)
	for _, r := range rs.Reviews {
		if r.Status == "approved" && strings.Contains(strings.ToLower(r.Professor), query) {
			result = append(result, r)
		}
	}
	return result
}

// IsBlocked checks if user is blocked
func (rs *RatingStore) IsBlocked(userID int64) bool {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	for _, id := range rs.BlockedUsers {
		if id == userID {
			return true
		}
	}
	return false
}

// BlockUser blocks a user
func (rs *RatingStore) BlockUser(userID int64) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for _, id := range rs.BlockedUsers {
		if id == userID {
			return
		}
	}
	rs.BlockedUsers = append(rs.BlockedUsers, userID)
	rs.save()
}

// NewRatingHandler creates a new rating handler
func NewRatingHandler(bot *tb.Bot, adminChatID int64, adminHandler *AdminHandler) *RatingHandler {
	return &RatingHandler{
		bot:          bot,
		store:        NewRatingStore("data/ratings.json"),
		sessions:     make(map[int64]*RatingSession),
		adminChatID:  adminChatID,
		adminHandler: adminHandler,
	}
}

// getSession returns or creates session
func (rh *RatingHandler) getSession(userID int64) *RatingSession {
	rh.sessionsMu.Lock()
	defer rh.sessionsMu.Unlock()
	if s, ok := rh.sessions[userID]; ok {
		return s
	}
	s := &RatingSession{Step: StepNone}
	rh.sessions[userID] = s
	return s
}

// clearSession removes session
func (rh *RatingHandler) clearSession(userID int64) {
	rh.sessionsMu.Lock()
	defer rh.sessionsMu.Unlock()
	delete(rh.sessions, userID)
}

// hasActiveSession checks if user has active rating session
func (rh *RatingHandler) hasActiveSession(userID int64) bool {
	rh.sessionsMu.RLock()
	defer rh.sessionsMu.RUnlock()
	s, ok := rh.sessions[userID]
	return ok && s.Step != StepNone
}

// getLangForUser returns language for user
func (rh *RatingHandler) getLangForUser(user *tb.User) i18n.Lang {
	if user == nil {
		return i18n.Get().GetDefault()
	}
	langCode := strings.ToLower(strings.TrimSpace(user.LanguageCode))
	langMap := map[string]i18n.Lang{
		"pl": i18n.PL, "en": i18n.EN, "ru": i18n.RU, "uk": i18n.UK, "be": i18n.BE,
	}
	if lang, ok := langMap[langCode]; ok {
		return lang
	}
	return i18n.Get().GetDefault()
}

// HandleRate starts rating flow
func (rh *RatingHandler) HandleRate(c tb.Context) error {
	if c.Chat().Type != tb.ChatPrivate {
		return nil
	}
	lang := rh.getLangForUser(c.Sender())
	msgs := i18n.Get().T(lang)

	userID := c.Sender().ID
	if rh.store.IsBlocked(userID) {
		_, _ = rh.bot.Send(c.Chat(), msgs.Rating.Blocked)
		return nil
	}

	session := rh.getSession(userID)
	session.Step = StepChooseType

	kb := &tb.ReplyMarkup{
		InlineKeyboard: [][]tb.InlineButton{
			{{Unique: "rate_public", Text: msgs.Rating.BtnPublic}, {Unique: "rate_anonymous", Text: msgs.Rating.BtnAnonymous}},
			{{Unique: "rate_cancel", Text: msgs.Rating.BtnCancel}},
		},
	}
	msg, _ := rh.bot.Send(c.Chat(), msgs.Rating.ChooseType, kb)
	session.MessageID = msg.ID
	return nil
}

// HandleRateCallback handles rate button callbacks
func (rh *RatingHandler) HandleRateCallback(c tb.Context) error {
	userID := c.Sender().ID
	session := rh.getSession(userID)
	lang := rh.getLangForUser(c.Sender())
	msgs := i18n.Get().T(lang)

	// Use Data for callbacks from sent messages, Unique for inline keyboard in same request
	data := c.Callback().Data
	if data == "" {
		data = c.Callback().Unique
	}

	logrus.WithFields(logrus.Fields{
		"user_id": userID,
		"data":    data,
	}).Info("Rating callback received")

	switch {
	case data == "rate_cancel":
		rh.clearSession(userID)
		_, _ = rh.bot.Edit(c.Message(), msgs.Rating.Cancelled)
		return rh.bot.Respond(c.Callback())

	case data == "rate_public":
		session.IsAnonymous = false
		session.Step = StepEnterName
		kb := &tb.ReplyMarkup{
			InlineKeyboard: [][]tb.InlineButton{
				{{Unique: "rate_cancel", Text: msgs.Rating.BtnCancel}},
			},
		}
		_, _ = rh.bot.Edit(c.Message(), msgs.Rating.EnterName, kb)
		return rh.bot.Respond(c.Callback())

	case data == "rate_anonymous":
		session.IsAnonymous = true
		session.Step = StepEnterName
		kb := &tb.ReplyMarkup{
			InlineKeyboard: [][]tb.InlineButton{
				{{Unique: "rate_cancel", Text: msgs.Rating.BtnCancel}},
			},
		}
		_, _ = rh.bot.Edit(c.Message(), msgs.Rating.EnterName, kb)
		return rh.bot.Respond(c.Callback())

	case strings.HasPrefix(data, "rate_score_"):
		scoreStr := strings.TrimPrefix(data, "rate_score_")
		score, _ := strconv.Atoi(scoreStr)
		session.Score = score
		session.Step = StepEnterReview
		kb := &tb.ReplyMarkup{
			InlineKeyboard: [][]tb.InlineButton{
				{{Unique: "rate_cancel", Text: msgs.Rating.BtnCancel}},
			},
		}
		_, _ = rh.bot.Edit(c.Message(), msgs.Rating.EnterReview, kb)
		return rh.bot.Respond(c.Callback())

	case data == "rate_submit":
		logrus.Info("Submitting review")
		return rh.submitReview(c, session)

	case strings.HasPrefix(data, "rate_approve_"):
		logrus.WithField("data", data).Info("Admin approve action")
		return rh.handleAdminAction(c, "approved")

	case strings.HasPrefix(data, "rate_reject_"):
		logrus.WithField("data", data).Info("Admin reject action")
		return rh.handleAdminAction(c, "rejected")

	case strings.HasPrefix(data, "rate_block_"):
		logrus.WithField("data", data).Info("Admin block action")
		return rh.handleAdminBlock(c)
	}

	logrus.WithField("data", data).Warn("Unhandled rating callback")
	return rh.bot.Respond(c.Callback())
}

// HandleRateText handles text input during rating
func (rh *RatingHandler) HandleRateText(c tb.Context) bool {
	userID := c.Sender().ID
	if !rh.hasActiveSession(userID) {
		return false
	}

	session := rh.getSession(userID)
	lang := rh.getLangForUser(c.Sender())
	msgs := i18n.Get().T(lang)
	text := strings.TrimSpace(c.Text())

	switch session.Step {
	case StepEnterName:
		// Validate name format (Name Surname)
		nameRegex := regexp.MustCompile(`^[A-Za-zĄĆĘŁŃÓŚŹŻąćęłńóśźż]+\s+[A-Za-zĄĆĘŁŃÓŚŹŻąćęłńóśźż]+$`)
		if !nameRegex.MatchString(text) {
			_, _ = rh.bot.Send(c.Chat(), msgs.Rating.InvalidName)
			return true
		}
		session.Professor = text
		session.Step = StepChooseScore

		kb := &tb.ReplyMarkup{
			InlineKeyboard: [][]tb.InlineButton{
				{
					{Unique: "rate_score_1", Text: "1 ⭐"},
					{Unique: "rate_score_2", Text: "2 ⭐"},
					{Unique: "rate_score_3", Text: "3 ⭐"},
					{Unique: "rate_score_4", Text: "4 ⭐"},
					{Unique: "rate_score_5", Text: "5 ⭐"},
				},
				{{Unique: "rate_cancel", Text: msgs.Rating.BtnCancel}},
			},
		}
		_, _ = rh.bot.Send(c.Chat(), msgs.Rating.ChooseScore, kb)
		return true

	case StepEnterReview:
		if len(text) < 10 {
			_, _ = rh.bot.Send(c.Chat(), msgs.Rating.ReviewTooShort)
			return true
		}
		if len(text) > 1000 {
			_, _ = rh.bot.Send(c.Chat(), msgs.Rating.ReviewTooLong)
			return true
		}
		session.Text = text
		session.Step = StepConfirm

		// Show preview
		preview := rh.formatReview(c.Sender(), session, 0, msgs)
		kb := &tb.ReplyMarkup{
			InlineKeyboard: [][]tb.InlineButton{
				{{Unique: "rate_submit", Text: msgs.Rating.BtnSubmit}},
				{{Unique: "rate_cancel", Text: msgs.Rating.BtnCancel}},
			},
		}
		_, _ = rh.bot.Send(c.Chat(), msgs.Rating.ConfirmReview+"\n\n"+preview, kb)
		return true
	default:
		panic("unhandled default case")
	}

	return false
}

// formatReview formats a review for display
func (rh *RatingHandler) formatReview(user *tb.User, session *RatingSession, reviewID int, msgs *i18n.Messages) string {
	sender := msgs.Rating.Anonymous
	if !session.IsAnonymous && user != nil {
		if user.Username != "" {
			sender = "@" + user.Username
		} else {
			sender = user.FirstName
		}
	}

	stars := strings.Repeat("⭐", session.Score) + strings.Repeat("☆", 5-session.Score)
	reviewNum := ""
	if reviewID > 0 {
		reviewNum = fmt.Sprintf(" #%d", reviewID)
	}

	return fmt.Sprintf("%s: %s\n%s: %s\n%s: [%d/5] %s\n\n%s%s: %s",
		msgs.Rating.Sender, sender,
		msgs.Rating.Professor, session.Professor,
		msgs.Rating.Score, session.Score, stars,
		msgs.Rating.ReviewLabel, reviewNum, session.Text,
	)
}

// formatReviewFromData formats review from stored data
func (rh *RatingHandler) formatReviewFromData(r Review, msgs *i18n.Messages) string {
	sender := msgs.Rating.Anonymous
	if !r.IsAnonymous {
		sender = "@" + r.Username
	}

	stars := strings.Repeat("⭐", r.Score) + strings.Repeat("☆", 5-r.Score)

	return fmt.Sprintf("%s: %s\n%s: %s\n%s: [%d/5] %s\n\n%s #%d: %s",
		msgs.Rating.Sender, sender,
		msgs.Rating.Professor, r.Professor,
		msgs.Rating.Score, r.Score, stars,
		msgs.Rating.ReviewLabel, r.ID, r.Text,
	)
}

// submitReview submits the review for moderation
func (rh *RatingHandler) submitReview(c tb.Context, session *RatingSession) error {
	lang := rh.getLangForUser(c.Sender())
	msgs := i18n.Get().T(lang)

	username := c.Sender().Username
	if username == "" {
		username = c.Sender().FirstName
	}

	review := Review{
		UserID:      c.Sender().ID,
		Username:    username,
		IsAnonymous: session.IsAnonymous,
		Professor:   session.Professor,
		Score:       session.Score,
		Text:        session.Text,
		Status:      "pending",
	}

	reviewID := rh.store.AddReview(review)
	rh.clearSession(c.Sender().ID)

	_, _ = rh.bot.Edit(c.Message(), msgs.Rating.Submitted)

	// Send it to the admin channel
	adminMsgs := i18n.Get().T(i18n.RU)
	adminText := fmt.Sprintf("📝 %s\n\n%s: @%s (ID: %d)\n%s: %s\n%s: %s\n%s: [%d/5] %s\n\n%s: %s",
		adminMsgs.Rating.NewReviewAdmin,
		adminMsgs.Rating.Sender, username, c.Sender().ID,
		adminMsgs.Rating.TypeLabel, func() string {
			if session.IsAnonymous {
				return adminMsgs.Rating.Anonymous
			}
			return adminMsgs.Rating.Public
		}(),
		adminMsgs.Rating.Professor, session.Professor,
		adminMsgs.Rating.Score, session.Score, strings.Repeat("⭐", session.Score),
		adminMsgs.Rating.ReviewLabel, session.Text,
	)

	kb := &tb.ReplyMarkup{
		InlineKeyboard: [][]tb.InlineButton{
			{
				{Data: fmt.Sprintf("rate_approve_%d", reviewID), Text: adminMsgs.Rating.BtnApprove},
				{Data: fmt.Sprintf("rate_reject_%d", reviewID), Text: adminMsgs.Rating.BtnReject},
			},
			{{Data: fmt.Sprintf("rate_block_%d", reviewID), Text: adminMsgs.Rating.BtnBlock}},
		},
	}
	_, _ = rh.bot.Send(&tb.Chat{ID: rh.adminChatID}, adminText, kb)

	return rh.bot.Respond(c.Callback())
}

// handleAdminAction handles approve/reject
func (rh *RatingHandler) handleAdminAction(c tb.Context, status string) error {
	data := c.Callback().Data
	if data == "" {
		data = c.Callback().Unique
	}
	var reviewID int
	var n int

	if status == "approved" {
		n, _ = fmt.Sscanf(data, "rate_approve_%d", &reviewID)
	} else {
		n, _ = fmt.Sscanf(data, "rate_reject_%d", &reviewID)
	}

	logrus.WithFields(logrus.Fields{
		"data":     data,
		"status":   status,
		"reviewID": reviewID,
		"parsed":   n,
	}).Info("Handling admin action")

	if n != 1 {
		logrus.Warn("Failed to parse review ID from callback data")
		return rh.bot.Respond(c.Callback())
	}

	review := rh.store.GetReview(reviewID)
	if review == nil {
		logrus.WithField("reviewID", reviewID).Warn("Review not found")
		return rh.bot.Respond(c.Callback())
	}

	logrus.WithFields(logrus.Fields{
		"reviewID":  reviewID,
		"professor": review.Professor,
		"userID":    review.UserID,
	}).Info("Review found, updating status")

	rh.store.UpdateReviewStatus(reviewID, status)

	adminMsgs := i18n.Get().T(i18n.RU)
	statusText := adminMsgs.Rating.StatusApproved
	if status == "rejected" {
		statusText = adminMsgs.Rating.StatusRejected
	}
	_, err := rh.bot.Edit(c.Message(), c.Message().Text+"\n\n"+statusText)
	if err != nil {
		logrus.WithError(err).Error("Failed to edit admin message")
	}

	// Notify user
	userChat := &tb.Chat{ID: review.UserID}
	userMsgs := i18n.Get().T(i18n.RU)
	var notifMsg string
	if status == "approved" {
		notifMsg = fmt.Sprintf(userMsgs.Rating.ReviewApproved, review.Professor)
	} else {
		notifMsg = fmt.Sprintf(userMsgs.Rating.ReviewRejected, review.Professor)
	}

	_, err = rh.bot.Send(userChat, notifMsg)
	if err != nil {
		logrus.WithError(err).WithField("userID", review.UserID).Error("Failed to notify user")
	} else {
		logrus.WithField("userID", review.UserID).Info("User notified successfully")
	}

	return rh.bot.Respond(c.Callback())
}

// handleAdminBlock blocks user
func (rh *RatingHandler) handleAdminBlock(c tb.Context) error {
	data := c.Callback().Data
	if data == "" {
		data = c.Callback().Unique
	}
	var reviewID int
	n, _ := fmt.Sscanf(data, "rate_block_%d", &reviewID)

	if n != 1 {
		return rh.bot.Respond(c.Callback())
	}

	review := rh.store.GetReview(reviewID)
	if review == nil {
		return rh.bot.Respond(c.Callback())
	}

	rh.store.UpdateReviewStatus(reviewID, "rejected")
	rh.store.BlockUser(review.UserID)

	adminMsgs := i18n.Get().T(i18n.RU)
	_, _ = rh.bot.Edit(c.Message(), c.Message().Text+"\n\n"+adminMsgs.Rating.StatusBlocked)

	return rh.bot.Respond(c.Callback())
}

// HandleRatings shows the ratings list
func (rh *RatingHandler) HandleRatings(c tb.Context) error {
	if c.Chat().Type != tb.ChatPrivate {
		return nil
	}
	return rh.showRatingsPage(c, 0, "")
}

// showRatingsPage shows paginated ratings
func (rh *RatingHandler) showRatingsPage(c tb.Context, page int, search string) error {
	lang := rh.getLangForUser(c.Sender())
	msgs := i18n.Get().T(lang)

	var reviews []Review
	if search != "" {
		reviews = rh.store.SearchReviews(search)
	} else {
		reviews = rh.store.GetApprovedReviews()
	}

	if len(reviews) == 0 {
		text := msgs.Rating.NoReviews
		if search != "" {
			text = fmt.Sprintf(msgs.Rating.NoSearchResults, search)
		}
		_, _ = rh.bot.Send(c.Chat(), text)
		return nil
	}

	// Pagination
	perPage := 5
	totalPages := (len(reviews) + perPage - 1) / perPage
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}

	start := page * perPage
	end := start + perPage
	if end > len(reviews) {
		end = len(reviews)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 %s (%d/%d)\n\n", msgs.Rating.ListHeader, page+1, totalPages))

	for i, r := range reviews[start:end] {
		sb.WriteString(rh.formatReviewFromData(r, msgs))
		if i < len(reviews[start:end])-1 {
			sb.WriteString("\n\n-----\n\n")
		}
	}

	// Build keyboard
	var buttons [][]tb.InlineButton
	var navRow []tb.InlineButton

	if page > 0 {
		navRow = append(navRow, tb.InlineButton{
			Unique: fmt.Sprintf("ratings_page_%d_%s", page-1, search),
			Text:   msgs.Rating.BtnPrev,
		})
	}
	if page < totalPages-1 {
		navRow = append(navRow, tb.InlineButton{
			Unique: fmt.Sprintf("ratings_page_%d_%s", page+1, search),
			Text:   msgs.Rating.BtnNext,
		})
	}
	if len(navRow) > 0 {
		buttons = append(buttons, navRow)
	}

	buttons = append(buttons, []tb.InlineButton{{Unique: "ratings_search", Text: msgs.Rating.BtnSearch}})

	kb := &tb.ReplyMarkup{InlineKeyboard: buttons}
	_, _ = rh.bot.Send(c.Chat(), sb.String(), kb)
	return nil
}

// HandleRatingsCallback handles ratings pagination
func (rh *RatingHandler) HandleRatingsCallback(c tb.Context) error {
	data := c.Callback().Data
	if data == "" {
		data = c.Callback().Unique
	}
	lang := rh.getLangForUser(c.Sender())
	msgs := i18n.Get().T(lang)

	switch {
	case data == "ratings_search":
		rh.sessionsMu.Lock()
		rh.sessions[c.Sender().ID] = &RatingSession{Step: StepNone, MessageID: -1} // -1 = search mode
		rh.sessionsMu.Unlock()
		_, _ = rh.bot.Edit(c.Message(), msgs.Rating.SearchPrompt)
		return rh.bot.Respond(c.Callback())

	case strings.HasPrefix(data, "ratings_page_"):
		parts := strings.SplitN(strings.TrimPrefix(data, "ratings_page_"), "_", 2)
		page, _ := strconv.Atoi(parts[0])
		search := ""
		if len(parts) > 1 {
			search = parts[1]
		}
		_ = rh.bot.Delete(c.Message())
		return rh.showRatingsPage(c, page, search)
	}

	return rh.bot.Respond(c.Callback())
}

// HandleSearchText handles search text input
func (rh *RatingHandler) HandleSearchText(c tb.Context) bool {
	rh.sessionsMu.RLock()
	session, ok := rh.sessions[c.Sender().ID]
	rh.sessionsMu.RUnlock()

	if !ok || session.MessageID != -1 {
		return false
	}

	rh.clearSession(c.Sender().ID)
	query := strings.TrimSpace(c.Text())
	return rh.showRatingsPage(c, 0, query) == nil
}

// RegisterHandlers registers all rating handlers
func (rh *RatingHandler) RegisterHandlers(bot *tb.Bot) {
	// Rate flow buttons - register specific handlers
	rateButtons := []string{
		"rate_cancel", "rate_public", "rate_anonymous", "rate_submit",
		"rate_score_1", "rate_score_2", "rate_score_3", "rate_score_4", "rate_score_5",
	}
	for _, unique := range rateButtons {
		btn := tb.InlineButton{Unique: unique}
		bot.Handle(&btn, rh.HandleRateCallback)
	}

	// Ratings pagination and search
	bot.Handle(&tb.InlineButton{Unique: "ratings_search"}, rh.HandleRatingsCallback)

	bot.Handle(tb.OnCallback, func(c tb.Context) error {
		logrus.Info("OnCallback handler invoked")

		if c.Callback() == nil {
			logrus.Warn("Callback is nil")
			return nil
		}

		data := c.Callback().Data
		unique := c.Callback().Unique

		logrus.WithFields(logrus.Fields{
			"data":    data,
			"unique":  unique,
			"user_id": c.Sender().ID,
			"chat_id": c.Chat().ID,
		}).Info("Callback received in OnCallback handler")

		callbackID := data
		if callbackID == "" {
			callbackID = unique
		}

		if callbackID == "" {
			logrus.Warn("Both Data and Unique are empty")
			return nil
		}

		if strings.HasPrefix(callbackID, "rate_approve_") ||
			strings.HasPrefix(callbackID, "rate_reject_") ||
			strings.HasPrefix(callbackID, "rate_block_") {
			logrus.WithField("callbackID", callbackID).Info("Admin button callback detected")
			return rh.HandleRateCallback(c)
		}

		if strings.HasPrefix(callbackID, "ratings_page_") {
			logrus.WithField("callbackID", callbackID).Debug("Pagination callback detected")
			return rh.HandleRatingsCallback(c)
		}

		logrus.WithField("callbackID", callbackID).Info("Callback not handled by rating handler")
		return nil
	})
}
