// users.go
package db

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sukalov/mshkbot/internal/utils"
	"gorm.io/gorm"
)

func Register(update tgbotapi.Update) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	message := update.Message
	userName := message.From.UserName
	tgName := strings.TrimSpace(message.From.FirstName + " " + message.From.LastName)

	user := User{
		ChatID:   message.Chat.ID,
		Username: userName,
		TgName:   tgName,
	}

	// FirstOrCreate to avoid duplicates
	result := Database.WithContext(ctx).Where(User{ChatID: message.Chat.ID}).FirstOrCreate(&user)
	if result.Error != nil {
		return fmt.Errorf("failed to register user: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		log.Printf("new user registered: id: %d, username: %s", message.Chat.ID, userName)
	}

	return nil
}

func GetByChatID(chatID int64) (User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	var user User
	result := Database.WithContext(ctx).Where("chat_id = ?", chatID).First(&user)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return User{}, fmt.Errorf("user not found: %d", chatID)
		}
		return User{}, fmt.Errorf("failed to retrieve user: %w", result.Error)
	}

	return user, nil
}

func GetByUsername(username string) (User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	var user User
	result := Database.WithContext(ctx).Where("username = ?", username).First(&user)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return User{}, fmt.Errorf("user not found")
		}
		return User{}, fmt.Errorf("failed to retrieve user: %w", result.Error)
	}

	return user, nil
}

func UpdateSavedName(chatID int64, newName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	result := Database.WithContext(ctx).
		Model(&User{}).
		Where("chat_id = ?", chatID).
		Update("saved_name", newName)

	if result.Error != nil {
		return fmt.Errorf("failed to update saved name: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("no user found with chat id: %d", chatID)
	}

	return nil
}

func UpdateLichess(chatID int64, lichess string) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	var value *string
	if lichess == "" {
		return fmt.Errorf("update lichess with ''")
	}
	value = &lichess

	result := Database.WithContext(ctx).
		Model(&User{}).
		Where("chat_id = ?", chatID).
		Update("lichess", value)

	if result.Error != nil {
		return fmt.Errorf("failed to update lichess: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("no user found with chat id: %d", chatID)
	}

	return nil
}

func UpdateChessCom(chatID int64, chessCom string) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	var value *string
	if chessCom == "" {
		return fmt.Errorf("update chesscom with ''")
	}
	value = &chessCom

	result := Database.WithContext(ctx).
		Model(&User{}).
		Where("chat_id = ?", chatID).
		Update("chesscom", value)

	if result.Error != nil {
		return fmt.Errorf("failed to update chesscom: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("no user found with chat id: %d", chatID)
	}

	return nil
}

func UpdateState(chatID int64, state State) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	result := Database.WithContext(ctx).
		Model(&User{}).
		Where("chat_id = ?", chatID).
		Update("state", state)

	if result.Error != nil {
		return fmt.Errorf("failed to update state: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("no user found with chat id: %d", chatID)
	}

	return nil
}

func SetBannedUntil(chatID int64, until *time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	result := Database.WithContext(ctx).
		Model(&User{}).
		Where("chat_id = ?", chatID).
		Update("banned_until", until)

	if result.Error != nil {
		return fmt.Errorf("failed to update ban status: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("no user found with chat id: %d", chatID)
	}

	return nil
}

func SetNotGreenUntil(chatID int64, until *time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	result := Database.WithContext(ctx).
		Model(&User{}).
		Where("chat_id = ?", chatID).
		Update("not_green_until", until)

	if result.Error != nil {
		return fmt.Errorf("failed to update green status: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("no user found with chat id: %d", chatID)
	}

	return nil
}

func SetAllowToGreen(chatID int64, allow bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	result := Database.WithContext(ctx).
		Model(&User{}).
		Where("chat_id = ?", chatID).
		Update("allow_to_green", allow)

	if result.Error != nil {
		return fmt.Errorf("failed to update manual allow status: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("no user found with chat id: %d", chatID)
	}

	return nil
}

func IncrementTimesPlayed(chatID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	result := Database.WithContext(ctx).
		Model(&User{}).
		Where("chat_id = ?", chatID).
		Update("times_played", gorm.Expr("times_played + ?", 1))

	if result.Error != nil {
		return fmt.Errorf("failed to increment times played: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("no user found with chat id: %d", chatID)
	}

	return nil
}

func DecrementTimesPlayed(chatID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	result := Database.WithContext(ctx).
		Model(&User{}).
		Where("chat_id = ?", chatID).
		Where("times_played > ?", 0).
		Update("times_played", gorm.Expr("times_played - ?", 1))

	if result.Error != nil {
		return fmt.Errorf("failed to decrement times played: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("no user found with chat id: %d or times_played already 0", chatID)
	}

	return nil
}

// GetAll returns all users
func GetAll() ([]User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	var users []User
	result := Database.WithContext(ctx).Find(&users)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get all users: %w", result.Error)
	}

	return users, nil
}

func GetUser(chatID int64) (User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	var user User
	result := Database.WithContext(ctx).
		Select("state").
		Where("chat_id = ?", chatID).
		First(&user)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return User{}, fmt.Errorf("user not found")
		}
		return User{}, fmt.Errorf("failed to get user state: %w", result.Error)
	}

	return user, nil
}

// Delete removes a user by chat ID
func Delete(chatID int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	result := Database.WithContext(ctx).Where("chat_id = ?", chatID).Delete(&User{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete user: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("no user found with chat id: %d", chatID)
	}

	return nil
}

func Stringify(u User) string {
	builder := strings.Builder{}

	if u.SavedName != "" {
		builder.WriteString(fmt.Sprintf("ник: %s\n", u.SavedName))
	}
	if u.Lichess != nil && *u.Lichess != "" {
		builder.WriteString(fmt.Sprintf("lichess: [%s](https://lichess.org/@/%s)\n", *u.Lichess, *u.Lichess))
	}
	if u.ChessCom != nil && *u.ChessCom != "" {
		builder.WriteString(fmt.Sprintf("chess.com: [%s](https://www.chess.com/member/%s)\n", *u.ChessCom, *u.ChessCom))
	}

	return builder.String()
}

// UpdateLichessAndState updates lichess username and state in one transaction
func UpdateLichessAndState(chatID int64, lichess string, newState State) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	if lichess == "" {
		return fmt.Errorf("update lichess with ''")
	}

	result := Database.WithContext(ctx).
		Model(&User{}).
		Where("chat_id = ?", chatID).
		Updates(map[string]interface{}{
			"lichess": &lichess,
			"state":   newState,
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update lichess and state: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("no user found with chat id: %d", chatID)
	}

	return nil
}

// UpdateChessComAndState updates chess.com username and state in one transaction
func UpdateChessComAndState(chatID int64, chessCom string, newState State) error {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	if chessCom == "" {
		return fmt.Errorf("update chesscom with ''")
	}

	result := Database.WithContext(ctx).
		Model(&User{}).
		Where("chat_id = ?", chatID).
		Updates(map[string]interface{}{
			"chesscom": &chessCom,
			"state":    newState,
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update chesscom and state: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("no user found with chat id: %d", chatID)
	}

	return nil
}

// GetOrCreateUser combines getting and creating user in one operation
func GetOrCreateUser(update tgbotapi.Update) (User, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	message := update.Message
	userName := message.From.UserName
	tgName := strings.TrimSpace(message.From.FirstName + " " + message.From.LastName)

	user := User{
		ChatID:   message.Chat.ID,
		Username: userName,
		TgName:   tgName,
	}

	result := Database.WithContext(ctx).Where(User{ChatID: message.Chat.ID}).FirstOrCreate(&user)
	if result.Error != nil {
		return User{}, false, fmt.Errorf("failed to get/create user: %w", result.Error)
	}

	isNew := result.RowsAffected > 0
	if isNew {
		log.Printf("new user registered: id: %d, username: %s", message.Chat.ID, userName)
	}

	return user, isNew, nil
}

func TestTransliteration() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var users []User
	result := Database.WithContext(ctx).Find(&users)
	if result.Error != nil {
		return fmt.Errorf("failed to get users: %w", result.Error)
	}

	log.Printf("=== testing transliteration on %d users ===", len(users))

	for _, user := range users {
		if user.SavedName == "" {
			continue
		}

		transliterated := utils.Transliterate(user.SavedName)
		if user.SavedName != transliterated {
			log.Printf("user %d (%s): '%s' -> '%s'", user.ChatID, user.Username, user.SavedName, transliterated)
		} else {
			log.Printf("user %d (%s): '%s' (no change)", user.ChatID, user.Username, user.SavedName)
		}
	}

	log.Printf("=== transliteration test complete ===")
	return nil
}

type TransliteratedUser struct {
	ChatID  int64
	OldName string
	NewName string
}

func TransliterateAllSavedNames() ([]TransliteratedUser, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var users []User
	result := Database.WithContext(ctx).Find(&users)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get users: %w", result.Error)
	}

	log.Printf("=== transliterating saved names for %d users ===", len(users))

	var changedUsers []TransliteratedUser

	for _, user := range users {
		if user.SavedName == "" {
			continue
		}

		transliterated := utils.Transliterate(user.SavedName)
		if user.SavedName != transliterated {
			updateResult := Database.WithContext(ctx).
				Model(&User{}).
				Where("chat_id = ?", user.ChatID).
				Update("saved_name", transliterated)

			if updateResult.Error != nil {
				log.Printf("failed to update user %d: %v", user.ChatID, updateResult.Error)
				continue
			}

			log.Printf("user %d (%s): '%s' -> '%s'", user.ChatID, user.Username, user.SavedName, transliterated)
			changedUsers = append(changedUsers, TransliteratedUser{
				ChatID:  user.ChatID,
				OldName: user.SavedName,
				NewName: transliterated,
			})
		}
	}

	log.Printf("=== transliteration complete: %d users changed ===", len(changedUsers))
	return changedUsers, nil
}
