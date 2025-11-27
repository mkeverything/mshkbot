package admingroup

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sukalov/mshkbot/internal/bot"
	"github.com/sukalov/mshkbot/internal/cron"
	"github.com/sukalov/mshkbot/internal/db"
	"github.com/sukalov/mshkbot/internal/types"
	"github.com/sukalov/mshkbot/internal/utils"
)

var scheduler *cron.Scheduler

// GetHandlers returns handler set for admin group
func GetHandlers(s *cron.Scheduler) bot.HandlerSet {
	scheduler = s
	return bot.HandlerSet{
		Commands: map[string]func(b *bot.Bot, update tgbotapi.Update) error{
			"help":                 handleHelp,
			"tournament":           handleTournament,
			"tournament_json":      handleTournamentJSON,
			"create_tournament":    handleCreateTournament,
			"remove_tournament":    handleRemoveTournament,
			"suspend_from_green":   handleSuspendFromGreen,
			"ban_player":           handleBanPlayer,
			"unban_player":         handleUnbanPlayer,
			"admit_to_green":       handleAdmitToGreen,
			"test_transliteration": handleTestTransliteration,
			"transliterate_all":    handleTransliterateAll,
			"send_schedule":        handleSendSchedule,
		},
		Messages: []func(b *bot.Bot, update tgbotapi.Update) error{
			handleScheduleFieldInput,
			handleAdminMessage,
		},
		Callbacks: map[string]func(b *bot.Bot, update tgbotapi.Update) error{
			"suspend_duration": handleSuspendDuration,
			"ban_duration":     handleBanDuration,
			"schedule":         handleScheduleCallback,
		},
	}
}

func handleHelp(b *bot.Bot, update tgbotapi.Update) error {
	return b.SendMessage(update.Message.Chat.ID, "команды администратора:\n\n/tournament - показать состояние турнира\n\n/send_schedule - показать расписание на неделю (отправляется автоматически в воскресенье 15:00)\n\n/suspend_from_green - отстранить пользователя от зелёных турниров\n\n/admit_to_green - допустить пользователя к зелёным турнирам\n\n/ban_player - забанить пользователя\n\n/unban_player - разбанить пользователя")
}

func handleTournamentJSON(b *bot.Bot, update tgbotapi.Update) error {
	jsonStr, err := b.Tournament.GetTournamentJSON()
	if err != nil {
		return err
	}
	return b.SendMessageWithMarkdown(update.Message.Chat.ID, fmt.Sprintf("```json\n%s```", jsonStr), true)
}

func handleTournament(b *bot.Bot, update tgbotapi.Update) error {
	if !b.Tournament.Metadata.Exists {
		return b.SendMessage(update.Message.Chat.ID, "турнир не создан")
	}

	message := buildTournamentMessageForAdmin(b)
	return b.SendMessageWithMarkdown(update.Message.Chat.ID, message, true)
}

func buildTournamentMessageForAdmin(b *bot.Bot) string {
	message := "участники:\n"

	count := 1
	for _, player := range b.Tournament.List {
		if player.State == types.StateInTournament {
			playerLine := fmt.Sprintf("%d. [%s](tg://user?id=%d)", count, player.SavedName, player.ID)
			if player.PeakRating != nil {
				var siteURL string
				switch player.PeakRating.Site {
				case types.SiteLichess:
					siteURL = fmt.Sprintf("https://lichess.org/@/%s", player.PeakRating.SiteUsername)
					playerLine += fmt.Sprintf(" ([%s](%s) %d)", player.PeakRating.Site, siteURL, player.PeakRating.BlitzPeak)
				case types.SiteChesscom:
					siteURL = fmt.Sprintf("https://www.chess.com/member/%s", player.PeakRating.SiteUsername)
					playerLine += fmt.Sprintf(" ([%s](%s) %d)", player.PeakRating.Site, siteURL, player.PeakRating.BlitzPeak)
				}
			}
			message += playerLine + "\n"
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
			playerLine := fmt.Sprintf("%d. [%s](tg://user?id=%d)", i+1, player.SavedName, player.ID)
			if player.PeakRating != nil {
				var siteURL string
				switch player.PeakRating.Site {
				case types.SiteLichess:
					siteURL = fmt.Sprintf("https://lichess.org/@/%s", player.PeakRating.SiteUsername)
					playerLine += fmt.Sprintf(" ([%s](%s) %d)", player.PeakRating.Site, siteURL, player.PeakRating.BlitzPeak)
				case types.SiteChesscom:
					siteURL = fmt.Sprintf("https://www.chess.com/member/%s", player.PeakRating.SiteUsername)
					playerLine += fmt.Sprintf(" ([%s](%s) %d)", player.PeakRating.Site, siteURL, player.PeakRating.BlitzPeak)
				}
			}
			message += playerLine + "\n"
		}
	}

	return message
}

func handleCreateTournament(b *bot.Bot, update tgbotapi.Update) error {
	ctx := context.Background()
	if b.Tournament.Metadata.Exists {
		return b.SendMessage(update.Message.Chat.ID, "турнир уже создан")
	}
	if err := b.Tournament.CreateTournament(ctx, 26, 0, 0, "ТУРНИР НАЧАЛСЯ!!!"); err != nil {
		return err
	}
	return b.GiveReaction(update.Message.Chat.ID, update.Message.MessageID, utils.ApproveEmoji())
}

func handleRemoveTournament(b *bot.Bot, update tgbotapi.Update) error {
	ctx := context.Background()
	if !b.Tournament.Metadata.Exists {
		return b.SendMessage(update.Message.Chat.ID, "его и так нет")
	}
	announcementMessageID := b.Tournament.Metadata.AnnouncementMessageID
	if announcementMessageID != 0 {
		if err := b.UnpinMessage(b.GetMainGroupID(), announcementMessageID); err != nil {
			log.Printf("failed to unpin message: %v", err)
		}
	}
	if err := b.Tournament.RemoveTournament(ctx); err != nil {
		return err
	}
	return b.GiveReaction(update.Message.Chat.ID, update.Message.MessageID, utils.ApproveEmoji())
}

func handleAdminMessage(b *bot.Bot, update tgbotapi.Update) error {
	if update.Message == nil {
		return nil
	}

	adminChatID := update.Message.From.ID

	process, exists := b.GetAdminProcess(adminChatID)
	if !exists {
		log.Printf("admin group message: %s", update.Message.Text)
		return nil
	}

	username := strings.TrimPrefix(strings.TrimSpace(update.Message.Text), "@")
	if username == "" {
		b.ClearAdminProcess(adminChatID)
		return b.SendMessage(update.Message.Chat.ID, "юзернейм не может быть пустым")
	}

	user, err := db.GetByUsername(username)
	if err != nil {
		b.ClearAdminProcess(adminChatID)
		return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("пользователь с юзернеймом %s не найден", username))
	}

	var until *time.Time
	now := time.Now().UTC()

	switch process.Type {
	case bot.ProcessTypeSuspension:
		switch process.Duration {
		case "month":
			t := now.AddDate(0, 1, 0)
			until = &t
		case "forever":
			t := now.AddDate(100, 0, 0)
			until = &t
		default:
			b.ClearAdminProcess(adminChatID)
			return b.SendMessage(update.Message.Chat.ID, "неизвестная длительность")
		}

		if err := db.SetNotGreenUntil(user.ChatID, until); err != nil {
			b.ClearAdminProcess(adminChatID)
			return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("ошибка при обновлении статуса: %v", err))
		}

		b.ClearAdminProcess(adminChatID)

		durationText := "навсегда"
		if process.Duration == "month" {
			durationText = "на месяц"
		}

		return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("пользователь %s отстранён от зелёных %s", username, durationText))

	case bot.ProcessTypeBan:
		switch process.Duration {
		case "month":
			t := now.AddDate(0, 1, 0)
			until = &t
		case "forever":
			t := now.AddDate(100, 0, 0)
			until = &t
		default:
			b.ClearAdminProcess(adminChatID)
			return b.SendMessage(update.Message.Chat.ID, "неизвестная длительность")
		}

		if err := db.SetBannedUntil(user.ChatID, until); err != nil {
			b.ClearAdminProcess(adminChatID)
			return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("ошибка при обновлении статуса: %v", err))
		}

		b.ClearAdminProcess(adminChatID)

		durationText := "навсегда"
		if process.Duration == "month" {
			durationText = "на месяц"
		}

		return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("пользователь %s забанен %s", username, durationText))

	case bot.ProcessTypeUnban:
		if err := db.SetBannedUntil(user.ChatID, nil); err != nil {
			b.ClearAdminProcess(adminChatID)
			return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("ошибка при обновлении статуса: %v", err))
		}

		b.ClearAdminProcess(adminChatID)

		return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("пользователь %s разбанен", username))

	case bot.ProcessTypeAdmitToGreen:
		if err := db.SetNotGreenUntil(user.ChatID, nil); err != nil {
			b.ClearAdminProcess(adminChatID)
			return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("ошибка при обновлении статуса: %v", err))
		}

		b.ClearAdminProcess(adminChatID)

		return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("пользователь %s допущен к зелёным турнирам", username))
	}

	return nil
}

func handleSuspendFromGreen(b *bot.Bot, update tgbotapi.Update) error {
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("месяц", "suspend_duration:month"),
			tgbotapi.NewInlineKeyboardButtonData("навсегда", "suspend_duration:forever"),
			tgbotapi.NewInlineKeyboardButtonData("отмена", "suspend_duration:cancel"),
		),
	)

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "выберите длительность отстранения:")
	msg.ReplyMarkup = keyboard

	_, err := b.Client.Send(msg)
	return err
}

func handleSuspendDuration(b *bot.Bot, update tgbotapi.Update) error {
	callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
	if _, err := b.Request(callback); err != nil {
		log.Printf("failed to answer callback: %v", err)
	}

	adminChatID := update.CallbackQuery.From.ID
	data := update.CallbackQuery.Data

	parts := strings.Split(data, ":")
	if len(parts) < 2 {
		return fmt.Errorf("invalid callback data: %s", data)
	}

	duration := parts[1]

	if duration == "cancel" {
		b.ClearAdminProcess(adminChatID)
		if err := b.EditMessage(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID, "отменено"); err != nil {
			return fmt.Errorf("failed to edit message: %w", err)
		}
		return nil
	}

	b.SetAdminProcess(adminChatID, bot.ProcessTypeSuspension, duration)

	if err := b.EditMessage(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID, "введите telegram username пользователя:"); err != nil {
		return fmt.Errorf("failed to edit message: %w", err)
	}

	return nil
}

func handleBanPlayer(b *bot.Bot, update tgbotapi.Update) error {
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("месяц", "ban_duration:month"),
			tgbotapi.NewInlineKeyboardButtonData("навсегда", "ban_duration:forever"),
			tgbotapi.NewInlineKeyboardButtonData("отмена", "ban_duration:cancel"),
		),
	)

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "выберите длительность бана:")
	msg.ReplyMarkup = keyboard

	_, err := b.Client.Send(msg)
	return err
}

func handleBanDuration(b *bot.Bot, update tgbotapi.Update) error {
	callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
	if _, err := b.Request(callback); err != nil {
		log.Printf("failed to answer callback: %v", err)
	}

	adminChatID := update.CallbackQuery.From.ID
	data := update.CallbackQuery.Data

	parts := strings.Split(data, ":")
	if len(parts) < 2 {
		return fmt.Errorf("invalid callback data: %s", data)
	}

	duration := parts[1]

	if duration == "cancel" {
		b.ClearAdminProcess(adminChatID)
		if err := b.EditMessage(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID, "отменено"); err != nil {
			return fmt.Errorf("failed to edit message: %w", err)
		}
		return nil
	}

	b.SetAdminProcess(adminChatID, bot.ProcessTypeBan, duration)

	if err := b.EditMessage(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID, "введите telegram username пользователя:"); err != nil {
		return fmt.Errorf("failed to edit message: %w", err)
	}

	return nil
}

func handleUnbanPlayer(b *bot.Bot, update tgbotapi.Update) error {
	adminChatID := update.Message.From.ID
	b.SetAdminProcess(adminChatID, bot.ProcessTypeUnban, "")
	return b.SendMessage(update.Message.Chat.ID, "введите telegram username пользователя для разбана:")
}

func handleAdmitToGreen(b *bot.Bot, update tgbotapi.Update) error {
	adminChatID := update.Message.From.ID
	b.SetAdminProcess(adminChatID, bot.ProcessTypeAdmitToGreen, "")
	return b.SendMessage(update.Message.Chat.ID, "учтите, игрок всё равно может не пройти по рейтингу. эта команда просто снимет внутрней бан.\n\nвведите telegram_username пользователя для допуска к зелёным турнирам:")
}

func handleTestTransliteration(b *bot.Bot, update tgbotapi.Update) error {
	if err := db.TestTransliteration(); err != nil {
		log.Printf("failed to test transliteration: %v", err)
		return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("ошибка: %v", err))
	}
	return b.SendMessage(update.Message.Chat.ID, "тест завершён, проверьте логи")
}

func handleTransliterateAll(b *bot.Bot, update tgbotapi.Update) error {
	changedUsers, err := db.TransliterateAllSavedNames()
	if err != nil {
		log.Printf("failed to transliterate all: %v", err)
		return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("ошибка: %v", err))
	}

	if len(changedUsers) == 0 {
		return b.SendMessage(update.Message.Chat.ID, "нет пользователей для изменения")
	}

	successCount := 0
	failCount := 0

	for _, user := range changedUsers {
		notificationMessage := fmt.Sprintf("я автоматически убрал из никнеймов заглавные буквы и перевёл все на русский. ваш новый никнейм: %s\n\nесли вам не нравится, что у меня получилось, поменять псевдоним можно командой /change_nickname", user.NewName)
		if err := b.SendMessage(user.ChatID, notificationMessage); err != nil {
			log.Printf("failed to notify user %d: %v", user.ChatID, err)
			failCount++
		} else {
			successCount++
		}
	}

	summary := fmt.Sprintf("транслитерация завершена:\n\nизменено пользователей: %d\nуведомлено: %d\nне удалось уведомить: %d", len(changedUsers), successCount, failCount)
	return b.SendMessage(update.Message.Chat.ID, summary)
}

func handleSendSchedule(b *bot.Bot, update tgbotapi.Update) error {
	scheduler.ScheduleManager.InitWeekSchedule()

	message := scheduler.ScheduleManager.FormatScheduleMessage()
	keyboard := cron.GetScheduleMainKeyboard()

	messageID, err := b.SendMessageWithButtonsAndGetID(update.Message.Chat.ID, message, keyboard)
	if err != nil {
		return err
	}

	scheduler.ScheduleManager.SetMessageID(messageID)
	return nil
}

func handleScheduleCallback(b *bot.Bot, update tgbotapi.Update) error {
	callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
	if _, err := b.Request(callback); err != nil {
		log.Printf("failed to answer callback: %v", err)
	}

	chatID := update.CallbackQuery.Message.Chat.ID
	messageID := update.CallbackQuery.Message.MessageID
	data := update.CallbackQuery.Data

	if scheduler.ScheduleManager.GetCurrentSchedule() == nil {
		return b.EditMessage(chatID, messageID, "расписание не инициализировано. используйте /send_schedule для создания нового")
	}

	parts := strings.Split(data, ":")
	if len(parts) < 2 {
		return fmt.Errorf("invalid callback data: %s", data)
	}

	action := parts[1]

	switch action {
	case "approve":
		return handleScheduleApprove(b, chatID, messageID)
	case "edit":
		return handleScheduleShowEditEvents(b, chatID, messageID)
	case "delete":
		return handleScheduleShowDeleteEvents(b, chatID, messageID)
	case "back":
		return handleScheduleBack(b, chatID, messageID)
	case "edit_event":
		if len(parts) < 3 {
			return fmt.Errorf("missing event id")
		}
		return handleScheduleSelectEditEvent(b, chatID, messageID, parts[2])
	case "delete_event":
		if len(parts) < 3 {
			return fmt.Errorf("missing event id")
		}
		return handleScheduleDeleteEvent(b, chatID, messageID, parts[2])
	case "field":
		if len(parts) < 4 {
			return fmt.Errorf("missing event id or field")
		}
		return handleScheduleSelectField(b, chatID, messageID, parts[2], parts[3])
	}

	return nil
}

func handleScheduleApprove(b *bot.Bot, chatID int64, messageID int) error {
	scheduler.ScheduleManager.SetApproved(true)
	scheduler.ScheduleManager.ClearEditingState()

	message := scheduler.ScheduleManager.FormatScheduleMessage()
	return b.EditMessage(chatID, messageID, message)
}

func handleScheduleShowEditEvents(b *bot.Bot, chatID int64, messageID int) error {
	message := scheduler.ScheduleManager.FormatScheduleMessage()
	message += "\n\n*выберите турнир для редактирования:*"
	keyboard := cron.GetScheduleSelectEventKeyboard("edit_event")

	return b.EditMessageWithButtons(chatID, messageID, message, keyboard)
}

func handleScheduleShowDeleteEvents(b *bot.Bot, chatID int64, messageID int) error {
	message := scheduler.ScheduleManager.FormatScheduleMessage()
	message += "\n\n*выберите турнир для удаления/восстановления (только на эту неделю):*"
	keyboard := scheduler.ScheduleManager.GetDeleteEventKeyboard()

	return b.EditMessageWithButtons(chatID, messageID, message, keyboard)
}

func handleScheduleBack(b *bot.Bot, chatID int64, messageID int) error {
	scheduler.ScheduleManager.ClearEditingState()

	message := scheduler.ScheduleManager.FormatScheduleMessage()
	keyboard := cron.GetScheduleMainKeyboard()

	return b.EditMessageWithButtons(chatID, messageID, message, keyboard)
}

func handleScheduleSelectEditEvent(b *bot.Bot, chatID int64, messageID int, eventID string) error {
	event := scheduler.ScheduleManager.GetEvent(eventID)
	if event == nil {
		return b.EditMessage(chatID, messageID, "турнир не найден")
	}

	message := scheduler.ScheduleManager.FormatScheduleMessage()
	message += fmt.Sprintf("\n\n*редактирование: %s*\nвыберите поле:", event.Day)
	keyboard := cron.GetScheduleEditFieldKeyboard(eventID)

	return b.EditMessageWithButtons(chatID, messageID, message, keyboard)
}

func handleScheduleDeleteEvent(b *bot.Bot, chatID int64, messageID int, eventID string) error {
	event := scheduler.ScheduleManager.GetEvent(eventID)
	if event == nil {
		return b.EditMessage(chatID, messageID, "турнир не найден")
	}

	if event.Deleted {
		scheduler.ScheduleManager.RestoreEvent(eventID)
	} else {
		scheduler.ScheduleManager.DeleteEvent(eventID)
	}

	message := scheduler.ScheduleManager.FormatScheduleMessage()
	message += "\n\n*выберите турнир для удаления/восстановления (только на эту неделю):*"
	keyboard := scheduler.ScheduleManager.GetDeleteEventKeyboard()

	return b.EditMessageWithButtons(chatID, messageID, message, keyboard)
}

func handleScheduleSelectField(b *bot.Bot, chatID int64, messageID int, eventID, field string) error {
	event := scheduler.ScheduleManager.GetEvent(eventID)
	if event == nil {
		return b.EditMessage(chatID, messageID, "турнир не найден")
	}

	scheduler.ScheduleManager.SetEditingEvent(eventID, field)

	var fieldName string
	var currentValue string

	switch field {
	case "limit":
		fieldName = "лимит участников"
		currentValue = fmt.Sprintf("%d", event.Limit)
	case "lichess_limit":
		fieldName = "лимит рейтинга lichess"
		currentValue = fmt.Sprintf("%d", event.LichessLimit)
	case "chesscom_limit":
		fieldName = "лимит рейтинга chess.com"
		currentValue = fmt.Sprintf("%d", event.ChesscomLimit)
	case "intro":
		fieldName = "текст объявления"
		currentValue = event.Intro
	default:
		return fmt.Errorf("unknown field: %s", field)
	}

	message := fmt.Sprintf("*редактирование %s*\n\nполе: %s\nтекущее значение: `%s`\n\nотправьте новое значение:", event.Day, fieldName, currentValue)
	keyboard := cron.GetScheduleBackKeyboard()

	return b.EditMessageWithButtons(chatID, messageID, message, keyboard)
}

func handleScheduleFieldInput(b *bot.Bot, update tgbotapi.Update) error {
	if update.Message == nil {
		return nil
	}

	eventID, field := scheduler.ScheduleManager.GetEditingState()
	if eventID == "" || field == "" {
		return nil
	}

	text := strings.TrimSpace(update.Message.Text)
	if text == "" {
		return nil
	}

	var value interface{}
	var err error

	switch field {
	case "limit", "lichess_limit", "chesscom_limit":
		intVal, parseErr := strconv.Atoi(text)
		if parseErr != nil {
			return b.SendMessage(update.Message.Chat.ID, "введите число")
		}
		if intVal < 0 {
			return b.SendMessage(update.Message.Chat.ID, "число должно быть положительным")
		}
		value = intVal
	case "intro":
		value = text
	default:
		return nil
	}

	if err = scheduler.ScheduleManager.UpdateEventField(eventID, field, value); err != nil {
		return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("ошибка: %v", err))
	}

	scheduler.ScheduleManager.ClearEditingState()

	scheduleMessageID := scheduler.ScheduleManager.GetMessageID()
	if scheduleMessageID != 0 {
		message := scheduler.ScheduleManager.FormatScheduleMessage()
		keyboard := cron.GetScheduleMainKeyboard()
		if err := b.EditMessageWithButtons(update.Message.Chat.ID, scheduleMessageID, message, keyboard); err != nil {
			log.Printf("failed to update schedule message: %v", err)
		}
	}

	return b.GiveReaction(update.Message.Chat.ID, update.Message.MessageID, utils.ApproveEmoji())
}
