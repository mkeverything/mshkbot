package logger

import (
	"fmt"
	"log"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// LogStatus represents the status of a log entry
type LogStatus string

const (
	StatusError   LogStatus = "error"
	StatusSuccess LogStatus = "success"
	StatusInfo    LogStatus = "info"
)

// Status emojis
const (
	EmojiError   = "❌"
	EmojiSuccess = "✅"
	EmojiInfo    = "⚙️"
)

// Logger handles logging to a Telegram channel
type Logger struct {
	bot       *tgbotapi.BotAPI
	channelID int64
	enabled   bool
}

// LogEntry represents a single log entry
type LogEntry struct {
	Status    LogStatus
	Action    string
	UserID    int64
	Username  string
	FirstName string
	LastName  string
	ChatID    int64
	ChatType  string
	Details   string
	Error     error
	Timestamp time.Time
}

// New creates a new logger instance
func New(bot *tgbotapi.BotAPI, channelID int64) *Logger {
	return &Logger{
		bot:       bot,
		channelID: channelID,
		enabled:   channelID != 0,
	}
}

// IsEnabled returns whether logging is enabled
func (l *Logger) IsEnabled() bool {
	return l.enabled
}

// Log creates and sends a log entry
func (l *Logger) Log(entry LogEntry) {
	if !l.enabled {
		return
	}

	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}

	message := l.formatLogMessage(entry)

	msg := tgbotapi.NewMessage(l.channelID, message)
	msg.ParseMode = "Markdown"
	msg.DisableWebPagePreview = true

	if _, err := l.bot.Send(msg); err != nil {
		log.Printf("[logger] failed to send log: %v", err)
	}
}

// LogError logs an error event
func (l *Logger) LogError(action string, user *UserInfo, details string, err error) {
	l.Log(LogEntry{
		Status:    StatusError,
		Action:    action,
		UserID:    user.ID,
		Username:  user.Username,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		ChatID:    user.ChatID,
		ChatType:  user.ChatType,
		Details:   details,
		Error:     err,
	})
}

// LogSuccess logs a successful event
func (l *Logger) LogSuccess(action string, user *UserInfo, details string) {
	l.Log(LogEntry{
		Status:    StatusSuccess,
		Action:    action,
		UserID:    user.ID,
		Username:  user.Username,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		ChatID:    user.ChatID,
		ChatType:  user.ChatType,
		Details:   details,
	})
}

// LogInfo logs an informational event
func (l *Logger) LogInfo(action string, user *UserInfo, details string) {
	l.Log(LogEntry{
		Status:    StatusInfo,
		Action:    action,
		UserID:    user.ID,
		Username:  user.Username,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		ChatID:    user.ChatID,
		ChatType:  user.ChatType,
		Details:   details,
	})
}

// UserInfo holds user information for logging
type UserInfo struct {
	ID        int64
	Username  string
	FirstName string
	LastName  string
	ChatID    int64
	ChatType  string
}

// ExtractUserInfoFromUpdate extracts user info from a Telegram update
func ExtractUserInfoFromUpdate(update *tgbotapi.Update, chatType string) *UserInfo {
	if update.Message != nil {
		return &UserInfo{
			ID:        update.Message.From.ID,
			Username:  update.Message.From.UserName,
			FirstName: update.Message.From.FirstName,
			LastName:  update.Message.From.LastName,
			ChatID:    update.Message.Chat.ID,
			ChatType:  chatType,
		}
	} else if update.CallbackQuery != nil {
		chatID := int64(0)
		if update.CallbackQuery.Message != nil {
			chatID = update.CallbackQuery.Message.Chat.ID
		}
		return &UserInfo{
			ID:        update.CallbackQuery.From.ID,
			Username:  update.CallbackQuery.From.UserName,
			FirstName: update.CallbackQuery.From.FirstName,
			LastName:  update.CallbackQuery.From.LastName,
			ChatID:    chatID,
			ChatType:  chatType,
		}
	}
	return nil
}

// formatLogMessage formats a log entry into a message string
func (l *Logger) formatLogMessage(entry LogEntry) string {
	var emoji string
	switch entry.Status {
	case StatusError:
		emoji = EmojiError
	case StatusSuccess:
		emoji = EmojiSuccess
	case StatusInfo:
		emoji = EmojiInfo
	}

	// Format user link
	var userLink string
	if entry.Username != "" {
		userLink = fmt.Sprintf("[%s %s](tg://user?id=%d) (@%s)",
			entry.FirstName, entry.LastName, entry.UserID, entry.Username)
	} else {
		userLink = fmt.Sprintf("[%s %s](tg://user?id=%d)",
			entry.FirstName, entry.LastName, entry.UserID)
	}

	// Build message
	message := fmt.Sprintf("%s *%s*\n\n", emoji, entry.Action)
	message += fmt.Sprintf("👤 *User:* %s\n", userLink)
	message += fmt.Sprintf("🆔 *ID:* `%d`\n", entry.UserID)

	if entry.Username != "" {
		message += fmt.Sprintf("📱 *Username:* @%s\n", entry.Username)
	}

	message += fmt.Sprintf("💬 *Chat:* `%d` (%s)\n", entry.ChatID, entry.ChatType)
	message += fmt.Sprintf("🕐 *Time:* `%s`\n", entry.Timestamp.Format("2006-01-02 15:04:05 MST"))

	if entry.Details != "" {
		message += fmt.Sprintf("\n📝 *Details:* %s", entry.Details)
	}

	if entry.Error != nil {
		message += fmt.Sprintf("\n\n⚠️ *Error:* `%s`", entry.Error.Error())
	}

	return message
}
