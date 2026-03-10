package privatechat

import (
	"context"
	"fmt"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sukalov/mshkbot/internal/bot"
	"github.com/sukalov/mshkbot/internal/db"
	"github.com/sukalov/mshkbot/internal/logger"
	"github.com/sukalov/mshkbot/internal/types"
	"github.com/sukalov/mshkbot/internal/utils"
)

// GetHandlers returns handler set for private messages
func GetHandlers() bot.HandlerSet {
	return bot.HandlerSet{
		Commands: map[string]func(b *bot.Bot, update tgbotapi.Update) error{
			"start":           handleStart,
			"help":            handleHelp,
			"me":              handleMe,
			"myratings":       handleMyRatings,
			"change_nickname": handleChangeNickname,
			"change_platform": handleChangePlatform,
			"checkin":         handleCheckinInPrivate,
			"checkout":        handleCheckinInPrivate,
		},
		Messages: []func(b *bot.Bot, update tgbotapi.Update) error{
			handlePrivateMessage,
		},
		Callbacks: map[string]func(b *bot.Bot, update tgbotapi.Update) error{
			"register":        handleRegister,
			"change_platform": handleChangePlatformCallback,
		},
	}
}

func handleCheckinInPrivate(b *bot.Bot, update tgbotapi.Update) error {
	return b.SendMessage(update.Message.Chat.ID, "записываться можно только в чате @moscowchessclub")
}

func handleStart(b *bot.Bot, update tgbotapi.Update) error {
	chatID := update.Message.Chat.ID
	userInfo := logger.ExtractUserInfoFromUpdate(&update, "private")

	// Log user started registration
	b.Logger.LogInfo("User started registration", userInfo, "User initiated /start command")

	// Get or create user in one operation
	user, isNew, err := db.GetOrCreateUser(update)
	if err != nil {
		log.Printf("failed to get/create user: %v", err)
		b.Logger.LogError("Registration start failed", userInfo, "Failed to get or create user in database", err)
		return err
	}

	if !isNew {
		// User exists, check their state
		switch user.State {
		case db.StateCompleted:
			b.Logger.LogInfo("Registration already completed", userInfo, "User attempted to start registration but already completed")
			return b.SendMessage(chatID, "вы уже зарегистрированы!")
		case db.StateAskedLichess:
			b.Logger.LogInfo("Registration in progress - lichess", userInfo, "User in lichess username input state")
			return b.SendMessage(chatID, "введите ваш никнейм на lichess:")
		case db.StateAskedChessCom:
			b.Logger.LogInfo("Registration in progress - chess.com", userInfo, "User in chess.com username input state")
			return b.SendMessage(chatID, "введите ваш никнейм на chess.com:")
		case db.StateAskedSavedName:
			b.Logger.LogInfo("Registration in progress - saved name", userInfo, "User in saved name input state")
			return b.SendMessage(chatID, "введите ваш никнейм для турниров:")
		}
	}

	// Show registration options for new users
	row := []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("lichess", "register:lichess"),
		tgbotapi.NewInlineKeyboardButtonData("chess.com", "register:chess.com"),
	}
	row2 := []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("нигде не играю (честное слово)", "register:none"),
	}

	return b.SendMessageWithButtons(chatID, "привет! чтобы записываться на турниры нужно показать свой шахматный уровень. где вы играете?", tgbotapi.NewInlineKeyboardMarkup(row, row2))
}

func handleRegister(b *bot.Bot, update tgbotapi.Update) error {
	chatID := update.CallbackQuery.Message.Chat.ID
	data := update.CallbackQuery.Data
	userInfo := logger.ExtractUserInfoFromUpdate(&update, "private")

	// answer callback query to remove loading state
	callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
	if _, err := b.Request(callback); err != nil {
		log.Printf("failed to answer callback: %v", err)
	}

	// parse option from callback data
	parts := strings.Split(data, ":")
	if len(parts) < 2 {
		b.Logger.LogError("Registration platform selection failed", userInfo, "Invalid callback data format", fmt.Errorf("invalid callback data: %s", data))
		return fmt.Errorf("invalid callback data: %s", data)
	}

	option := parts[1]

	switch option {
	case "lichess":
		b.Logger.LogInfo("Platform selected", userInfo, "User selected lichess as platform")
		if err := b.EditMessage(chatID, update.CallbackQuery.Message.MessageID, "введите ваш никнейм на lichess:"); err != nil {
			b.Logger.LogError("Failed to edit message", userInfo, "Failed to update message after platform selection", err)
			return fmt.Errorf("failed to edit message: %w", err)
		}
		if err := db.UpdateState(chatID, db.StateAskedLichess); err != nil {
			b.Logger.LogError("Failed to update state", userInfo, "Failed to update user state to AskedLichess", err)
			return fmt.Errorf("failed to update state: %w", err)
		}
		b.Logger.LogSuccess("State updated", userInfo, "User state updated to AskedLichess")

	case "chess.com":
		b.Logger.LogInfo("Platform selected", userInfo, "User selected chess.com as platform")
		if err := b.EditMessage(chatID, update.CallbackQuery.Message.MessageID, "введите ваш никнейм на chess.com:"); err != nil {
			b.Logger.LogError("Failed to edit message", userInfo, "Failed to update message after platform selection", err)
			return fmt.Errorf("failed to edit message: %w", err)
		}
		if err := db.UpdateState(chatID, db.StateAskedChessCom); err != nil {
			b.Logger.LogError("Failed to update state", userInfo, "Failed to update user state to AskedChessCom", err)
			return fmt.Errorf("failed to update state: %w", err)
		}
		b.Logger.LogSuccess("State updated", userInfo, "User state updated to AskedChessCom")

	case "none":
		b.Logger.LogInfo("Platform selected", userInfo, "User selected no platform")
		if err := b.EditMessage(chatID, update.CallbackQuery.Message.MessageID, "введите ваш псевдоним для турниров:"); err != nil {
			b.Logger.LogError("Failed to edit message", userInfo, "Failed to update message after platform selection", err)
			return fmt.Errorf("failed to edit message: %w", err)
		}
		if err := db.UpdateState(chatID, db.StateAskedSavedName); err != nil {
			b.Logger.LogError("Failed to update state", userInfo, "Failed to update user state to AskedSavedName", err)
			return fmt.Errorf("failed to update state: %w", err)
		}
		b.Logger.LogSuccess("State updated", userInfo, "User state updated to AskedSavedName")

	default:
		b.Logger.LogError("Unknown platform option", userInfo, fmt.Sprintf("Unknown registration option: %s", option), fmt.Errorf("unknown register option: %s", option))
		return fmt.Errorf("unknown register option: %s", option)
	}

	return nil
}

const helpMessage = "/help — показать это сообщение\n\n/me — показать вашу информацию\n\n/myratings — показать пиковые рейтинги\n\n/change_nickname — изменить никнейм для турниров\n\n/change_platform — изменить или добавить аккаунт lichess/chess.com"

func handleHelp(b *bot.Bot, update tgbotapi.Update) error {
	return b.SendMessage(update.Message.Chat.ID, helpMessage)
}

func handleMe(b *bot.Bot, update tgbotapi.Update) error {
	chatID := update.Message.Chat.ID

	if user, err := db.GetByChatID(chatID); err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	} else {
		return b.SendMessageWithMarkdown(chatID, db.Stringify(user), true)
	}
}

func handleMyRatings(b *bot.Bot, update tgbotapi.Update) error {
	chatID := update.Message.Chat.ID
	var lichess, chesscom string
	if user, err := db.GetByChatID(chatID); err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	} else {

		if user.Lichess == nil || *user.Lichess == "" {
			lichess = "личес не указан"
		}
		if user.ChessCom == nil || *user.ChessCom == "" {
			chesscom = "чесском не указан"
		}

		if user.Lichess != nil {
			lichessTopRatings, err := utils.GetLichessAllTimeHigh(*user.Lichess)
			if err != nil {
				return fmt.Errorf("ошибка при запросе к базе личеса: %w", err)
			}

			lichess = fmt.Sprintf("пиковые рейтинги на личесе: блиц %d, рапид %d, классика %d", lichessTopRatings.Blitz, lichessTopRatings.Rapid, lichessTopRatings.Classical)
		}
		if user.ChessCom != nil {
			chesscomTopRatings, err := utils.GetChessComAllTimeHigh(*user.ChessCom)
			if err != nil {
				return fmt.Errorf("ошибка при запросе к базе чесскома: %w", err)
			}
			chesscom = fmt.Sprintf("пиковые рейтинги на чесскоме: блиц %d, рапид %d, классика %d", chesscomTopRatings.Blitz, chesscomTopRatings.Rapid, chesscomTopRatings.Classical)
		}

		return b.SendMessage(chatID, fmt.Sprintf("%s\n%s", lichess, chesscom))
	}
}

func handleChangeNickname(b *bot.Bot, update tgbotapi.Update) error {
	chatID := update.Message.Chat.ID
	userInfo := logger.ExtractUserInfoFromUpdate(&update, "private")

	b.Logger.LogInfo("Nickname change initiated", userInfo, "User started nickname change process")

	user, err := db.GetByChatID(chatID)
	if err != nil {
		b.Logger.LogError("Failed to get user", userInfo, "Failed to retrieve user for nickname change", err)
		return fmt.Errorf("failed to get user: %w", err)
	}

	if user.SavedName == "" {
		b.Logger.LogInfo("No saved name to change", userInfo, "User attempted to change nickname but has no saved name")
		return b.SendMessage(chatID, "у вас ещё нет сохранённого никнейма")
	}

	if err := db.UpdateState(chatID, db.StateEditingSavedName); err != nil {
		b.Logger.LogError("Failed to update state", userInfo, "Failed to update state to EditingSavedName", err)
		return fmt.Errorf("failed to update state: %w", err)
	}

	b.Logger.LogSuccess("State updated for nickname change", userInfo, fmt.Sprintf("User state updated to EditingSavedName, current name: %s", user.SavedName))
	return b.SendMessage(chatID, fmt.Sprintf("ваш текущий никнейм: %s\n\nвведите новый никнейм:", user.SavedName))
}

func handleChangePlatform(b *bot.Bot, update tgbotapi.Update) error {
	chatID := update.Message.Chat.ID
	userInfo := logger.ExtractUserInfoFromUpdate(&update, "private")

	b.Logger.LogInfo("Platform change initiated", userInfo, "User started platform change process")

	user, err := db.GetByChatID(chatID)
	if err != nil {
		b.Logger.LogError("Failed to get user", userInfo, "Failed to retrieve user for platform change", err)
		return b.SendMessage(chatID, "вы ещё не зарегистрированы. напишите /start для регистрации")
	}

	if user.State != db.StateCompleted {
		b.Logger.LogInfo("Registration not completed", userInfo, "User attempted to change platform but registration not completed")
		return b.SendMessage(chatID, "сначала завершите регистрацию")
	}

	var currentInfo string
	if user.Lichess != nil && *user.Lichess != "" {
		currentInfo += fmt.Sprintf("lichess: %s\n", *user.Lichess)
	}
	if user.ChessCom != nil && *user.ChessCom != "" {
		currentInfo += fmt.Sprintf("chess.com: %s\n", *user.ChessCom)
	}
	if currentInfo == "" {
		currentInfo = "платформы не указаны\n"
	}

	b.Logger.LogInfo("Platform change options displayed", userInfo, fmt.Sprintf("Current platforms: %s", currentInfo))

	row := []tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("lichess", "change_platform:lichess"),
		tgbotapi.NewInlineKeyboardButtonData("chess.com", "change_platform:chesscom"),
	}

	return b.SendMessageWithButtons(chatID, fmt.Sprintf("текущие аккаунты:\n%s\nвыберите платформу для изменения:", currentInfo), tgbotapi.NewInlineKeyboardMarkup(row))
}

func handleChangePlatformCallback(b *bot.Bot, update tgbotapi.Update) error {
	chatID := update.CallbackQuery.Message.Chat.ID
	data := update.CallbackQuery.Data
	userInfo := logger.ExtractUserInfoFromUpdate(&update, "private")

	callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
	if _, err := b.Request(callback); err != nil {
		log.Printf("failed to answer callback: %v", err)
	}

	parts := strings.Split(data, ":")
	if len(parts) < 2 {
		b.Logger.LogError("Platform change callback failed", userInfo, "Invalid callback data format", fmt.Errorf("invalid callback data: %s", data))
		return fmt.Errorf("invalid callback data: %s", data)
	}

	platform := parts[1]

	switch platform {
	case "lichess":
		b.Logger.LogInfo("Platform change selected", userInfo, "User selected to change lichess platform")
		if err := b.EditMessage(chatID, update.CallbackQuery.Message.MessageID, "введите новый никнейм на lichess:"); err != nil {
			b.Logger.LogError("Failed to edit message", userInfo, "Failed to update message for lichess change", err)
			return fmt.Errorf("failed to edit message: %w", err)
		}
		if err := db.UpdateState(chatID, db.StateEditingLichess); err != nil {
			b.Logger.LogError("Failed to update state", userInfo, "Failed to update state to EditingLichess", err)
			return fmt.Errorf("failed to update state: %w", err)
		}
		b.Logger.LogSuccess("State updated for platform change", userInfo, "User state updated to EditingLichess")

	case "chesscom":
		b.Logger.LogInfo("Platform change selected", userInfo, "User selected to change chess.com platform")
		if err := b.EditMessage(chatID, update.CallbackQuery.Message.MessageID, "введите новый никнейм на chess.com:"); err != nil {
			b.Logger.LogError("Failed to edit message", userInfo, "Failed to update message for chess.com change", err)
			return fmt.Errorf("failed to edit message: %w", err)
		}
		if err := db.UpdateState(chatID, db.StateEditingChessCom); err != nil {
			b.Logger.LogError("Failed to update state", userInfo, "Failed to update state to EditingChessCom", err)
			return fmt.Errorf("failed to update state: %w", err)
		}
		b.Logger.LogSuccess("State updated for platform change", userInfo, "User state updated to EditingChessCom")

	default:
		b.Logger.LogError("Unknown platform option", userInfo, fmt.Sprintf("Unknown platform: %s", platform), fmt.Errorf("unknown platform: %s", platform))
		return fmt.Errorf("unknown platform: %s", platform)
	}

	return nil
}

func handlePrivateMessage(b *bot.Bot, update tgbotapi.Update) error {
	if update.Message == nil {
		return nil
	}

	chatID := update.Message.Chat.ID
	userInfo := logger.ExtractUserInfoFromUpdate(&update, "private")

	user, err := db.GetUser(chatID) // DB CALL 1
	if err != nil {
		log.Printf("failed to get user state: %v", err)
		b.Logger.LogError("Failed to get user state", userInfo, "Failed to retrieve user state from database", err)
		return nil
	}

	switch user.State {
	case db.StateAskedLichess:
		username := strings.TrimPrefix(strings.TrimSpace(update.Message.Text), "@")
		if username == "" {
			b.Logger.LogInfo("Empty username rejected", userInfo, "User attempted to submit empty lichess username")
			return b.SendMessage(chatID, "юзернейм не может быть пустым")
		}

		allTimeHigh, err := utils.GetLichessAllTimeHigh(username)
		if err != nil {
			errorContext := utils.CreateErrorContext(&update, "get_lichess_rating")
			utils.LogError(errorContext, err, "failed to get lichess all time high")
			b.Logger.LogError("Lichess rating fetch failed", userInfo, fmt.Sprintf("Failed to fetch lichess rating for username: %s", username), err)
			return b.SendMessage(chatID, utils.FormatUserErrorMessage(errorContext.TraceID, "произошла ошибка, попробуйте ещё раз"))
		}
		log.Printf("all time high: %d", allTimeHigh)

		// save the username
		if err := db.UpdateLichess(chatID, username); err != nil { // DB CALL 2
			errorContext := utils.CreateErrorContext(&update, "update_lichess_username")
			utils.LogError(errorContext, err, "failed to update lichess username")
			b.Logger.LogError("Lichess username update failed", userInfo, fmt.Sprintf("Failed to update lichess username: %s", username), err)
			return b.SendMessage(chatID, utils.FormatUserErrorMessage(errorContext.TraceID, "произошла ошибка, попробуйте ещё раз"))
		}
		b.Logger.LogSuccess("Lichess username saved", userInfo, fmt.Sprintf("Lichess username saved: %s", username))

		// ask for saved name
		if err := db.UpdateState(chatID, db.StateAskedSavedName); err != nil { // DB CALL 3
			b.Logger.LogError("State update failed", userInfo, "Failed to update state to AskedSavedName", err)
			return fmt.Errorf("failed to update state: %w", err)
		}
		b.Logger.LogSuccess("State updated", userInfo, "User state updated to AskedSavedName after lichess input")

		return b.SendMessage(chatID, "введите ваш никнейм для турниров:")

	case db.StateAskedChessCom:
		username := strings.TrimPrefix(strings.TrimSpace(update.Message.Text), "@")
		if username == "" {
			b.Logger.LogInfo("Empty username rejected", userInfo, "User attempted to submit empty chess.com username")
			return b.SendMessage(chatID, "юзернейм не может быть пустым")
		}

		// save the username
		if err := db.UpdateChessCom(chatID, username); err != nil {
			errorContext := utils.CreateErrorContext(&update, "update_chesscom_username")
			utils.LogError(errorContext, err, "failed to update chesscom username")
			b.Logger.LogError("Chess.com username update failed", userInfo, fmt.Sprintf("Failed to update chess.com username: %s", username), err)
			return b.SendMessage(chatID, utils.FormatUserErrorMessage(errorContext.TraceID, "произошла ошибка, попробуйте еще раз"))
		}
		b.Logger.LogSuccess("Chess.com username saved", userInfo, fmt.Sprintf("Chess.com username saved: %s", username))

		// ask for saved name
		if err := db.UpdateState(chatID, db.StateAskedSavedName); err != nil {
			b.Logger.LogError("State update failed", userInfo, "Failed to update state to AskedSavedName", err)
			return fmt.Errorf("failed to update state: %w", err)
		}
		b.Logger.LogSuccess("State updated", userInfo, "User state updated to AskedSavedName after chess.com input")

		return b.SendMessage(chatID, "введите ваш никнейм для турниров:")

	case db.StateAskedSavedName:
		savedName := utils.Transliterate(update.Message.Text)

		if savedName == "" {
			b.Logger.LogInfo("Empty nickname rejected", userInfo, "User attempted to submit empty saved name")
			return b.SendMessage(chatID, "никнейм не может быть пустым")
		}

		if err := db.UpdateSavedName(chatID, savedName); err != nil {
			errorContext := utils.CreateErrorContext(&update, "update_saved_name")
			utils.LogError(errorContext, err, "failed to update saved name")
			b.Logger.LogError("Saved name update failed", userInfo, fmt.Sprintf("Failed to update saved name: %s", savedName), err)
			return b.SendMessage(chatID, utils.FormatUserErrorMessage(errorContext.TraceID, "произошла ошибка, попробуйте еще раз"))
		}
		b.Logger.LogSuccess("Saved name updated", userInfo, fmt.Sprintf("Saved name updated: %s", savedName))

		if err := db.UpdateState(chatID, db.StateCompleted); err != nil {
			b.Logger.LogError("State update failed", userInfo, "Failed to update state to Completed", err)
			return fmt.Errorf("failed to update state: %w", err)
		}
		b.Logger.LogSuccess("Registration completed", userInfo, fmt.Sprintf("User registration completed with nickname: %s", savedName))

		return b.SendMessage(chatID, fmt.Sprintf("отлично! регистрация завершена. ваш никнейм: %s\n\nтеперь можете записываться на турниры в чате @moscowchessclub\n\n для записи на турнир нажмите /checkin в чате!!", savedName))

	case db.StateEditingSavedName:
		newName := utils.Transliterate(update.Message.Text)

		if newName == "" {
			b.Logger.LogInfo("Empty nickname rejected", userInfo, "User attempted to submit empty new nickname")
			return b.SendMessage(chatID, "никнейм не может быть пустым")
		}

		if err := db.UpdateSavedName(chatID, newName); err != nil {
			errorContext := utils.CreateErrorContext(&update, "update_saved_name_command")
			utils.LogError(errorContext, err, "failed to update saved name via command")
			b.Logger.LogError("Nickname change failed", userInfo, fmt.Sprintf("Failed to update saved name to: %s", newName), err)
			return b.SendMessage(chatID, utils.FormatUserErrorMessage(errorContext.TraceID, "произошла ошибка, попробуйте еще раз"))
		}
		b.Logger.LogSuccess("Nickname changed", userInfo, fmt.Sprintf("Nickname changed to: %s", newName))

		if err := db.UpdateState(chatID, db.StateCompleted); err != nil {
			b.Logger.LogError("State update failed", userInfo, "Failed to update state to Completed after nickname change", err)
			return fmt.Errorf("failed to update state: %w", err)
		}

		if err := b.SendMessage(chatID, fmt.Sprintf("никнейм успешно изменён на: %s", newName)); err != nil {
			return err
		}

		if err := updateTournamentPlayerName(b, int(chatID), newName); err != nil {
			log.Printf("failed to update tournament player name: %v", err)
			b.Logger.LogError("Tournament name update failed", userInfo, fmt.Sprintf("Failed to update tournament player name to: %s", newName), err)
		}

		return nil

	case db.StateEditingLichess:
		newUsername := strings.TrimPrefix(strings.TrimSpace(update.Message.Text), "@")
		if newUsername == "" {
			b.Logger.LogInfo("Empty username rejected", userInfo, "User attempted to submit empty lichess username for change")
			return b.SendMessage(chatID, "юзернейм не может быть пустым")
		}

		_, err := utils.GetLichessAllTimeHigh(newUsername)
		if err != nil {
			b.Logger.LogInfo("Lichess user not found", userInfo, fmt.Sprintf("Lichess user not found: %s", newUsername))
			return b.SendMessage(chatID, "пользователь не найден на lichess. проверьте никнейм и попробуйте ещё раз")
		}

		fullUser, err := db.GetByChatID(chatID)
		if err != nil {
			errorContext := utils.CreateErrorContext(&update, "get_user_for_lichess_update")
			utils.LogError(errorContext, err, "failed to get user for lichess update")
			b.Logger.LogError("User fetch failed", userInfo, "Failed to get user for lichess update", err)
			return b.SendMessage(chatID, utils.FormatUserErrorMessage(errorContext.TraceID, "произошла ошибка, попробуйте ещё раз"))
		}

		previousUsername := fullUser.Lichess

		if err := db.UpdateLichessAndState(chatID, newUsername, db.StateCompleted); err != nil {
			errorContext := utils.CreateErrorContext(&update, "update_lichess_and_state")
			utils.LogError(errorContext, err, "failed to update lichess username and state")
			b.Logger.LogError("Lichess update failed", userInfo, fmt.Sprintf("Failed to update lichess username to: %s", newUsername), err)
			return b.SendMessage(chatID, utils.FormatUserErrorMessage(errorContext.TraceID, "произошла ошибка, попробуйте ещё раз"))
		}

		if previousUsername != nil && *previousUsername != "" {
			b.Logger.LogSuccess("Lichess platform changed", userInfo, fmt.Sprintf("Lichess username changed from %s to %s", *previousUsername, newUsername))
			notifyAdminAboutPlatformChange(b, update, "lichess", *previousUsername, newUsername, fullUser)
		} else {
			b.Logger.LogSuccess("Lichess platform added", userInfo, fmt.Sprintf("Lichess username added: %s", newUsername))
		}

		return b.SendMessage(chatID, fmt.Sprintf("lichess аккаунт успешно изменён на: %s", newUsername))

	case db.StateEditingChessCom:
		newUsername := strings.TrimPrefix(strings.TrimSpace(update.Message.Text), "@")
		if newUsername == "" {
			b.Logger.LogInfo("Empty username rejected", userInfo, "User attempted to submit empty chess.com username for change")
			return b.SendMessage(chatID, "юзернейм не может быть пустым")
		}

		_, err := utils.GetChessComAllTimeHigh(newUsername)
		if err != nil {
			b.Logger.LogInfo("Chess.com user not found", userInfo, fmt.Sprintf("Chess.com user not found: %s", newUsername))
			return b.SendMessage(chatID, "пользователь не найден на chess.com. проверьте никнейм и попробуйте ещё раз")
		}

		fullUser, err := db.GetByChatID(chatID)
		if err != nil {
			errorContext := utils.CreateErrorContext(&update, "get_user_for_chesscom_update")
			utils.LogError(errorContext, err, "failed to get user for chesscom update")
			b.Logger.LogError("User fetch failed", userInfo, "Failed to get user for chess.com update", err)
			return b.SendMessage(chatID, utils.FormatUserErrorMessage(errorContext.TraceID, "произошла ошибка, попробуйте ещё раз"))
		}

		previousUsername := fullUser.ChessCom

		if err := db.UpdateChessComAndState(chatID, newUsername, db.StateCompleted); err != nil {
			errorContext := utils.CreateErrorContext(&update, "update_chesscom_and_state")
			utils.LogError(errorContext, err, "failed to update chesscom username and state")
			b.Logger.LogError("Chess.com update failed", userInfo, fmt.Sprintf("Failed to update chess.com username to: %s", newUsername), err)
			return b.SendMessage(chatID, utils.FormatUserErrorMessage(errorContext.TraceID, "произошла ошибка, попробуйте ещё раз"))
		}

		if previousUsername != nil && *previousUsername != "" {
			b.Logger.LogSuccess("Chess.com platform changed", userInfo, fmt.Sprintf("Chess.com username changed from %s to %s", *previousUsername, newUsername))
			notifyAdminAboutPlatformChange(b, update, "chess.com", *previousUsername, newUsername, fullUser)
		} else {
			b.Logger.LogSuccess("Chess.com platform added", userInfo, fmt.Sprintf("Chess.com username added: %s", newUsername))
		}

		return b.SendMessage(chatID, fmt.Sprintf("chess.com аккаунт успешно изменён на: %s", newUsername))

	default:
		log.Printf("private message from %d: %s", update.Message.From.ID, update.Message.Text)
		b.Logger.LogInfo("Unrecognized message", userInfo, fmt.Sprintf("Received unrecognized message: %s", update.Message.Text))
		forwardUnparsableMessage(b, update)

		feedback := fmt.Sprintf("непонятно. вот всё что я умею:\n\n%s", helpMessage)
		return b.SendMessage(chatID, feedback)
	}
}

func forwardUnparsableMessage(b *bot.Bot, update tgbotapi.Update) {
	adminChatID := b.GetAdminGroupID()
	if adminChatID == 0 {
		return
	}

	user := update.Message.From
	userLink := fmt.Sprintf("[%s %s](tg://user?id=%d)", user.FirstName, user.LastName, user.ID)
	if user.UserName != "" {
		userLink = fmt.Sprintf("[%s %s](tg://user?id=%d) (@%s)", user.FirstName, user.LastName, user.ID, user.UserName)
	}

	dbUser, err := db.GetByChatID(user.ID)
	var dbInfo string
	if err == nil {
		dbInfo = fmt.Sprintf("\nв базе: %s", dbUser.SavedName)
		if dbUser.Lichess != nil {
			dbInfo += fmt.Sprintf(" | lichess: %s", *dbUser.Lichess)
		}
		if dbUser.ChessCom != nil {
			dbInfo += fmt.Sprintf(" | chesscom: %s", *dbUser.ChessCom)
		}
	} else {
		dbInfo = "\nв базе не найден"
	}

	header := fmt.Sprintf("непонятное сообщение от %s%s:", userLink, dbInfo)
	if err := b.SendMessageWithMarkdown(adminChatID, header, true); err != nil {
		log.Printf("failed to send header to admin chat: %v", err)
		return
	}

	if err := b.ForwardMessage(adminChatID, update.Message.Chat.ID, update.Message.MessageID); err != nil {
		log.Printf("failed to forward message to admin chat: %v", err)
	}
}

func notifyAdminAboutPlatformChange(b *bot.Bot, update tgbotapi.Update, platform, previousUsername, newUsername string, dbUser db.User) {
	adminChatID := b.GetAdminGroupID()
	if adminChatID == 0 {
		return
	}

	tgUser := update.Message.From
	userLink := fmt.Sprintf("[%s %s](tg://user?id=%d)", tgUser.FirstName, tgUser.LastName, tgUser.ID)
	if tgUser.UserName != "" {
		userLink = fmt.Sprintf("[%s %s](tg://user?id=%d) (@%s)", tgUser.FirstName, tgUser.LastName, tgUser.ID, tgUser.UserName)
	}

	var previousLink, newLink string
	if platform == "lichess" {
		previousLink = fmt.Sprintf("[%s](https://lichess.org/@/%s)", previousUsername, previousUsername)
		newLink = fmt.Sprintf("[%s](https://lichess.org/@/%s)", newUsername, newUsername)
	} else {
		previousLink = fmt.Sprintf("[%s](https://www.chess.com/member/%s)", previousUsername, previousUsername)
		newLink = fmt.Sprintf("[%s](https://www.chess.com/member/%s)", newUsername, newUsername)
	}

	message := fmt.Sprintf("смена аккаунта %s\n\nпользователь: %s\nник в боте: %s\n\nбыло: %s\nстало: %s",
		platform,
		userLink,
		dbUser.SavedName,
		previousLink,
		newLink,
	)

	if err := b.SendMessageWithMarkdown(adminChatID, message, true); err != nil {
		log.Printf("failed to send platform change notification to admin chat: %v", err)
	}
}

func updateTournamentPlayerName(b *bot.Bot, playerID int, newName string) error {
	ctx := context.Background()

	if !b.Tournament.Metadata.Exists {
		return nil
	}

	var currentPlayer *types.Player
	for _, player := range b.Tournament.List {
		if player.ID == playerID {
			currentPlayer = &player
			break
		}
	}

	if currentPlayer == nil {
		return nil
	}

	updatedPlayer := *currentPlayer
	updatedPlayer.SavedName = newName

	if err := b.Tournament.EditPlayer(ctx, playerID, updatedPlayer); err != nil {
		return fmt.Errorf("failed to update player in tournament: %w", err)
	}

	log.Printf("updated player %d name to %s in tournament", playerID, newName)

	announcementMessageID := b.Tournament.Metadata.AnnouncementMessageID
	if announcementMessageID == 0 {
		return nil
	}

	messageIntro := b.Tournament.Metadata.AnnouncementIntro
	if messageIntro == "" {
		messageIntro = "запись на турнир открыта"
	}

	message := buildTournamentListMessage(b, messageIntro)
	if err := b.EditMessage(b.GetMainGroupID(), announcementMessageID, message); err != nil {
		return fmt.Errorf("failed to update announcement message: %w", err)
	}

	log.Printf("updated announcement message after name change")
	return nil
}

func buildTournamentListMessage(b *bot.Bot, messageIntro string) string {
	message := fmt.Sprintf("%s\n\nучастники:\n", messageIntro)

	count := 1
	for _, player := range b.Tournament.List {
		if player.State == types.StateInTournament {
			message += fmt.Sprintf("%d. %s\n", count, player.SavedName)
			count++
		}
	}

	if count == 1 {
		message += "пока никого нет\n"
	}

	queuedPlayers := []types.Player{}
	for _, player := range b.Tournament.List {
		if player.State == types.StateQueued {
			queuedPlayers = append(queuedPlayers, player)
		}
	}

	if len(queuedPlayers) > 0 {
		message += "\nочередь:\n"
		for i, player := range queuedPlayers {
			message += fmt.Sprintf("%d. %s\n", i+1, player.SavedName)
		}
	}

	return message
}
