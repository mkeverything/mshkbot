package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ErrorContext holds context information for error logging
type ErrorContext struct {
	UserID    int64
	UserName  string
	Command   string
	Operation string
	Timestamp time.Time
	TraceID   string
}

// TraceableError represents an error with trace information
type TraceableError struct {
	Context  ErrorContext
	Original error
	Message  string
}

func (e *TraceableError) Error() string {
	return e.Message
}

// GenerateTraceID creates a unique trace ID for error tracking
func GenerateTraceID() string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)[:8]
}

// LogError logs an error with full context information
func LogError(context ErrorContext, err error, message string) {
	if context.TraceID == "" {
		context.TraceID = GenerateTraceID()
	}
	if context.Timestamp.IsZero() {
		context.Timestamp = time.Now()
	}

	log.Printf("error: traceID=%s, userID=%d, userName=%s, command=%s, operation=%s, message=%s, error=%v",
		context.TraceID,
		context.UserID,
		context.UserName,
		context.Command,
		context.Operation,
		message,
		err)
}

// LogTournamentError logs tournament-specific errors with context
func LogTournamentError(operation string, userID int64, err error) {
	traceID := GenerateTraceID()
	log.Printf("tournament error: traceID=%s, operation=%s, userID=%d, error=%v",
		traceID, operation, userID, err)
}

// CreateErrorContext creates error context from update
func CreateErrorContext(update *tgbotapi.Update, operation string) ErrorContext {
	var userID int64
	var userName string
	var command string

	if update.Message != nil {
		userID = update.Message.From.ID
		userName = update.Message.From.FirstName
		if update.Message.IsCommand() {
			command = update.Message.Command()
		}
	} else if update.CallbackQuery != nil {
		userID = update.CallbackQuery.From.ID
		userName = update.CallbackQuery.From.FirstName
		command = update.CallbackQuery.Data
	}

	return ErrorContext{
		UserID:    userID,
		UserName:  userName,
		Command:   command,
		Operation: operation,
		Timestamp: time.Now(),
		TraceID:   GenerateTraceID(),
	}
}

// FormatUserErrorMessage formats error message for user with trace ID
func FormatUserErrorMessage(traceID string, fallbackMessage string) string {
	if traceID != "" {
		return fmt.Sprintf("произошла ошибка (код: %s). обратитесь к администратору", traceID)
	}
	return fallbackMessage
}

// FormatCommandError formats error message for failed command
func FormatCommandError(command string, traceID string) string {
	if traceID != "" {
		return fmt.Sprintf("ошибка при выполнении команды /%s (код: %s)", command, traceID)
	}
	return fmt.Sprintf("ошибка при выполнении команды /%s", command)
}

// ExtractUserID safely extracts user ID from update as string
func ExtractUserID(update *tgbotapi.Update) string {
	if update.Message != nil {
		return strconv.FormatInt(update.Message.From.ID, 10)
	} else if update.CallbackQuery != nil {
		return strconv.FormatInt(update.CallbackQuery.From.ID, 10)
	}
	return "unknown"
}

// ExtractUsername safely extracts username from update
func ExtractUsername(update *tgbotapi.Update) string {
	if update.Message != nil {
		return update.Message.From.FirstName
	} else if update.CallbackQuery != nil {
		return update.CallbackQuery.From.FirstName
	}
	return "unknown"
}
