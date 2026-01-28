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
			"stop_tournament":      handleStopTournament,
			"create_event":         handleCreateEvent,
			"start_custom":         handleStartCustomTournament,
			"suspend_from_green":   handleSuspendFromGreen,
			"ban_player":           handleBanPlayer,
			"unban_player":         handleUnbanPlayer,
			"admit_to_green":       handleAdmitToGreen,
			"allow_to_green":       handleAllowToGreen,
			"test_transliteration": handleTestTransliteration,
			"transliterate_all":    handleTransliterateAll,
			"send_schedule":        handleSendSchedule,
		},
		Messages: []func(b *bot.Bot, update tgbotapi.Update) error{
			handleScheduleFieldInput,
			handleAdminMessage,
			handleCreateEventInput,
			handleStartCustomInput,
		},
		Callbacks: map[string]func(b *bot.Bot, update tgbotapi.Update) error{
			"suspend_duration":   handleSuspendDuration,
			"ban_duration":       handleBanDuration,
			"schedule":           handleScheduleCallback,
			"create_event_param": handleCreateEventParam,
			"start_custom_param": handleStartCustomParam,
			"stop_tournament":    handleStopTournamentCallback,
		},
	}
}

func handleHelp(b *bot.Bot, update tgbotapi.Update) error {
	return b.SendMessage(update.Message.Chat.ID, "команды администратора:\n\n/tournament - показать состояние турнира\n\n/start_custom - создать кастомный турнир с настройками\n\n/stop_tournament - остановить текущий турнир\n\n/create_event - создать событие для использования в будущем\n\n/send_schedule - показать расписание на неделю\n\n/suspend_from_green - отстранить пользователя от зелёных турниров\n\n/admit_to_green - допустить пользователя к зелёным турнирам\n\n/allow_to_green - разрешить пользователю участие в зелёном турнире вручную\n\n/ban_player - забанить пользователя\n\n/unban_player - разбанить пользователя")
}

func handleTournamentJSON(b *bot.Bot, update tgbotapi.Update) error {
	errorContext := utils.CreateErrorContext(&update, "handle_tournament_json")

	if b.Tournament == nil {
		utils.LogError(errorContext, fmt.Errorf("tournament manager is nil"), "tournament manager not initialized")
		return b.SendMessage(update.Message.Chat.ID, "ошибка: менеджер турниров не инициализирован")
	}

	jsonStr, err := b.Tournament.GetTournamentJSON()
	if err != nil {
		utils.LogError(errorContext, err, "failed to get tournament json")
		fallbackMsg := utils.FormatUserErrorMessage(errorContext.TraceID, "ошибка при получении данных турнира")
		return b.SendMessage(update.Message.Chat.ID, fallbackMsg)
	}

	if jsonStr == "" {
		return b.SendMessage(update.Message.Chat.ID, "данные турнира пусты")
	}

	return b.SendMessageWithMarkdown(update.Message.Chat.ID, fmt.Sprintf("```json\n%s```", jsonStr), true)
}

func handleTournament(b *bot.Bot, update tgbotapi.Update) error {
	errorContext := utils.CreateErrorContext(&update, "handle_tournament")

	if b.Tournament == nil {
		utils.LogError(errorContext, fmt.Errorf("tournament manager is nil"), "tournament manager not initialized")
		return b.SendMessage(update.Message.Chat.ID, "ошибка: менеджер турниров не инициализирован")
	}

	if !b.Tournament.Metadata.Exists {
		return b.SendMessage(update.Message.Chat.ID, "турнир не создан")
	}

	message, err := buildTournamentMessageForAdmin(b)
	if err != nil {
		utils.LogError(errorContext, err, "failed to build tournament message")
		fallbackMsg := utils.FormatUserErrorMessage(errorContext.TraceID, "ошибка при получении данных турнира")
		return b.SendMessage(update.Message.Chat.ID, fallbackMsg)
	}

	return b.SendMessageWithMarkdown(update.Message.Chat.ID, message, true)
}

func formatPlayerLineForAdmin(num int, player types.Player) (string, error) {
	if player.SavedName == "" {
		return "", fmt.Errorf("player has empty saved name")
	}

	var playerLine string
	if player.CheckinMessageID != 0 && player.CheckinChatID != 0 {
		chatIDForLink := player.CheckinChatID
		if chatIDForLink < 0 {
			chatIDForLink = -chatIDForLink - 1000000000000
		}
		if chatIDForLink <= 0 {
			return "", fmt.Errorf("invalid chat ID for link: %d", chatIDForLink)
		}
		messageLink := fmt.Sprintf("https://t.me/c/%d/%d", chatIDForLink, player.CheckinMessageID)
		playerLine = fmt.Sprintf("%d. [%s](%s)", num, player.SavedName, messageLink)
	} else {
		playerLine = fmt.Sprintf("%d. %s", num, player.SavedName)
	}

	if player.Username != "" {
		playerLine += fmt.Sprintf(" (@%s)", player.Username)
	}

	if player.PeakRating != nil {
		var siteURL string
		switch player.PeakRating.Site {
		case types.SiteLichess:
			if player.PeakRating.SiteUsername == "" {
				return "", fmt.Errorf("lichess player has empty username")
			}
			siteURL = fmt.Sprintf("https://lichess.org/@/%s", player.PeakRating.SiteUsername)
			playerLine += fmt.Sprintf(" ([%s](%s) %d)", player.PeakRating.Site, siteURL, player.PeakRating.BlitzPeak)
		case types.SiteChesscom:
			if player.PeakRating.SiteUsername == "" {
				return "", fmt.Errorf("chess.com player has empty username")
			}
			siteURL = fmt.Sprintf("https://www.chess.com/member/%s", player.PeakRating.SiteUsername)
			playerLine += fmt.Sprintf(" ([%s](%s) %d)", player.PeakRating.Site, siteURL, player.PeakRating.BlitzPeak)
		default:
			return "", fmt.Errorf("unknown rating site: %s", player.PeakRating.Site)
		}
	}

	return playerLine, nil
}

func formatPlayerLineForAdminWithMetadata(num int, player types.Player, metadata types.TournamentMetadata) (string, error) {
	playerLine, err := formatPlayerLineForAdmin(num, player)
	if err != nil {
		return "", err
	}

	if player.AllowToGreen {
		playerLine += " 🍀"
	}

	return playerLine, nil
}

func buildTournamentMessageForAdmin(b *bot.Bot) (string, error) {
	if b.Tournament == nil {
		return "", fmt.Errorf("tournament manager is nil")
	}

	if b.Tournament.List == nil {
		return "", fmt.Errorf("tournament list is nil")
	}

	message := "участники:\n"

	count := 1
	for _, player := range b.Tournament.List {
		if player.State == types.StateInTournament {
			playerLine, err := formatPlayerLineForAdminWithMetadata(count, player, b.Tournament.Metadata)
			if err != nil {
				log.Printf("failed to format player line for admin: %v", err)
				continue
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
			playerLine, err := formatPlayerLineForAdminWithMetadata(i+1, player, b.Tournament.Metadata)
			if err != nil {
				log.Printf("failed to format queued player line for admin: %v", err)
				continue
			}
			message += playerLine + "\n"
		}
	}

	return message, nil
}

func handleStopTournament(b *bot.Bot, update tgbotapi.Update) error {
	if !b.Tournament.Metadata.Exists {
		return b.SendMessage(update.Message.Chat.ID, "турнир не создан")
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("да, остановить", "stop_tournament:confirm"),
			tgbotapi.NewInlineKeyboardButtonData("отмена", "stop_tournament:cancel"),
		),
	)

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "⚠️ *подтвердите остановку турнира*\n\nэто действие остановит турнир и открепит объявление.")
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard

	_, err := b.Client.Send(msg)
	return err
}

func handleStopTournamentCallback(b *bot.Bot, update tgbotapi.Update) error {
	callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
	if _, err := b.Request(callback); err != nil {
		log.Printf("failed to answer callback: %v", err)
	}

	chatID := update.CallbackQuery.Message.Chat.ID
	messageID := update.CallbackQuery.Message.MessageID
	data := update.CallbackQuery.Data

	parts := strings.Split(data, ":")
	if len(parts) < 2 {
		return fmt.Errorf("invalid callback data: %s", data)
	}

	action := parts[1]

	if action == "cancel" {
		return b.EditMessage(chatID, messageID, "отмена: турнир не будет остановлен")
	}

	if action == "confirm" {
		ctx := context.Background()
		announcementMessageID := b.Tournament.Metadata.AnnouncementMessageID
		if announcementMessageID != 0 {
			if err := b.UnpinMessage(b.GetMainGroupID(), announcementMessageID); err != nil {
				log.Printf("failed to unpin message: %v", err)
			}
		}
		if err := b.Tournament.RemoveTournament(ctx); err != nil {
			return b.EditMessage(chatID, messageID, fmt.Sprintf("ошибка при остановке турнира: %v", err))
		}
		return b.EditMessage(chatID, messageID, "✅ турнир остановлен")
	}

	return nil
}

func handleCreateEvent(b *bot.Bot, update tgbotapi.Update) error {
	if b.Tournament.Metadata.Exists {
		return b.SendMessage(update.Message.Chat.ID, "турнир уже создан. остановите его перед созданием нового события.")
	}

	adminChatID := update.Message.From.ID
	config := &bot.EventConfig{
		Limit:         0,
		LichessLimit:  0,
		ChesscomLimit: 0,
		Intro:         "",
	}
	b.SetAdminProcessWithConfig(adminChatID, bot.ProcessTypeCreateEvent, "limit", config)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("отмена", "create_event_param:cancel"),
		),
	)

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "*создание кастомного события*\n\nшаг 1: введите лимит участников (число, например: 24)")
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard

	_, err := b.Client.Send(msg)
	return err
}

func handleStartCustomTournament(b *bot.Bot, update tgbotapi.Update) error {
	if b.Tournament.Metadata.Exists {
		return b.SendMessage(update.Message.Chat.ID, "турнир уже создан. остановите его перед созданием нового.")
	}

	adminChatID := update.Message.From.ID
	config := &bot.EventConfig{
		Limit:         0,
		LichessLimit:  0,
		ChesscomLimit: 0,
		Intro:         "",
	}
	b.SetAdminProcessWithConfig(adminChatID, bot.ProcessTypeStartCustom, "limit", config)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("отмена", "start_custom_param:cancel"),
		),
	)

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "*запуск кастомного турнира*\n\nшаг 1: введите лимит участников (число, например: 24)")
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard

	_, err := b.Client.Send(msg)
	return err
}

func handleCreateEventInput(b *bot.Bot, update tgbotapi.Update) error {
	if update.Message == nil {
		return nil
	}

	adminChatID := update.Message.From.ID
	process, exists := b.GetAdminProcess(adminChatID)
	if !exists || process.Type != bot.ProcessTypeCreateEvent {
		return nil
	}

	text := strings.TrimSpace(update.Message.Text)
	if text == "" {
		return nil
	}

	config := process.EventConfig
	var nextStep string
	var nextPrompt string

	switch process.CurrentStep {
	case "limit":
		intVal, parseErr := strconv.Atoi(text)
		if parseErr != nil || intVal <= 0 {
			return b.SendMessage(update.Message.Chat.ID, "введите положительное число")
		}
		config.Limit = intVal
		nextStep = "lichess_limit"
		nextPrompt = "*создание кастомного события*\n\nшаг 2: введите лимит рейтинга lichess (0 = без лимита, например: 1600)"

	case "lichess_limit":
		intVal, parseErr := strconv.Atoi(text)
		if parseErr != nil || intVal < 0 {
			return b.SendMessage(update.Message.Chat.ID, "введите положительное число или 0")
		}
		config.LichessLimit = intVal
		nextStep = "chesscom_limit"
		nextPrompt = "*создание кастомного события*\n\nшаг 3: введите лимит рейтинга chess.com (0 = без лимита, например: 1200)"

	case "chesscom_limit":
		intVal, parseErr := strconv.Atoi(text)
		if parseErr != nil || intVal < 0 {
			return b.SendMessage(update.Message.Chat.ID, "введите положительное число или 0")
		}
		config.ChesscomLimit = intVal
		nextStep = "intro"
		nextPrompt = "*создание кастомного события*\n\nшаг 4: введите текст объявления"

	case "intro":
		config.Intro = text
		nextStep = "confirm"
		nextPrompt = fmt.Sprintf("*проверьте конфигурацию события*\n\nлимит: %d\nlichess<%d, chesscom<%d\n\nтекст: _%s_\n\nсоздать событие?",
			config.Limit, config.LichessLimit, config.ChesscomLimit, text)

		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("создать", "create_event_param:confirm"),
				tgbotapi.NewInlineKeyboardButtonData("отмена", "create_event_param:cancel"),
			),
		)

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, nextPrompt)
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = keyboard
		_, err := b.Client.Send(msg)
		if err != nil {
			return err
		}
		return b.GiveReaction(update.Message.Chat.ID, update.Message.MessageID, utils.ApproveEmoji())

	default:
		return nil
	}

	if nextStep != "" {
		b.SetAdminProcessWithConfig(adminChatID, bot.ProcessTypeCreateEvent, nextStep, config)

		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("отмена", "create_event_param:cancel"),
			),
		)

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, nextPrompt)
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = keyboard
		_, err := b.Client.Send(msg)
		if err != nil {
			return err
		}
	}

	return b.GiveReaction(update.Message.Chat.ID, update.Message.MessageID, utils.ApproveEmoji())
}

func handleStartCustomInput(b *bot.Bot, update tgbotapi.Update) error {
	if update.Message == nil {
		return nil
	}

	adminChatID := update.Message.From.ID
	process, exists := b.GetAdminProcess(adminChatID)
	if !exists || process.Type != bot.ProcessTypeStartCustom {
		return nil
	}

	text := strings.TrimSpace(update.Message.Text)
	if text == "" {
		return nil
	}

	config := process.CustomConfig
	var nextStep string
	var nextPrompt string

	switch process.CurrentStep {
	case "limit":
		intVal, parseErr := strconv.Atoi(text)
		if parseErr != nil || intVal <= 0 {
			return b.SendMessage(update.Message.Chat.ID, "введите положительное число")
		}
		config.Limit = intVal
		nextStep = "lichess_limit"
		nextPrompt = "*запуск кастомного турнира*\n\nшаг 2: введите лимит рейтинга lichess (0 = без лимита, например: 1600)"

	case "lichess_limit":
		intVal, parseErr := strconv.Atoi(text)
		if parseErr != nil || intVal < 0 {
			return b.SendMessage(update.Message.Chat.ID, "введите положительное число или 0")
		}
		config.LichessLimit = intVal
		nextStep = "chesscom_limit"
		nextPrompt = "*запуск кастомного турнира*\n\nшаг 3: введите лимит рейтинга chess.com (0 = без лимита, например: 1200)"

	case "chesscom_limit":
		intVal, parseErr := strconv.Atoi(text)
		if parseErr != nil || intVal < 0 {
			return b.SendMessage(update.Message.Chat.ID, "введите положительное число или 0")
		}
		config.ChesscomLimit = intVal
		nextStep = "intro"
		nextPrompt = "*запуск кастомного турнира*\n\nшаг 4: введите текст объявления"

	case "intro":
		config.Intro = text
		nextStep = "confirm"
		nextPrompt = fmt.Sprintf("*проверьте конфигурацию турнира*\n\nлимит: %d\nlichess<%d, chesscom<%d\n\nтекст: _%s_\n\nзапустить турнир?",
			config.Limit, config.LichessLimit, config.ChesscomLimit, text)

		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("запустить", "start_custom_param:confirm"),
				tgbotapi.NewInlineKeyboardButtonData("отмена", "start_custom_param:cancel"),
			),
		)

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, nextPrompt)
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = keyboard
		_, err := b.Client.Send(msg)
		if err != nil {
			return err
		}
		return b.GiveReaction(update.Message.Chat.ID, update.Message.MessageID, utils.ApproveEmoji())

	default:
		return nil
	}

	if nextStep != "" {
		b.SetAdminProcessWithConfig(adminChatID, bot.ProcessTypeStartCustom, nextStep, config)

		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("отмена", "start_custom_param:cancel"),
			),
		)

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, nextPrompt)
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = keyboard
		_, err := b.Client.Send(msg)
		if err != nil {
			return err
		}
	}

	return b.GiveReaction(update.Message.Chat.ID, update.Message.MessageID, utils.ApproveEmoji())
}

func handleCreateEventParam(b *bot.Bot, update tgbotapi.Update) error {
	callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
	if _, err := b.Request(callback); err != nil {
		log.Printf("failed to answer callback: %v", err)
	}

	chatID := update.CallbackQuery.Message.Chat.ID
	messageID := update.CallbackQuery.Message.MessageID
	data := update.CallbackQuery.Data

	parts := strings.Split(data, ":")
	if len(parts) < 2 {
		return fmt.Errorf("invalid callback data: %s", data)
	}

	action := parts[1]

	if action == "cancel" {
		adminChatID := update.CallbackQuery.From.ID
		b.ClearAdminProcess(adminChatID)
		return b.EditMessage(chatID, messageID, "создание события отменено")
	}

	if action == "confirm" {
		adminChatID := update.CallbackQuery.From.ID
		process, exists := b.GetAdminProcess(adminChatID)
		if !exists || process.EventConfig == nil {
			return b.EditMessage(chatID, messageID, "ошибка: конфигурация не найдена")
		}

		config := process.EventConfig
		b.ClearAdminProcess(adminChatID)

		return b.EditMessage(chatID, messageID, fmt.Sprintf("✅ событие создано:\n\nлимит: %d\nlichess<%d, chesscom<%d\n\nтекст: _%s_\n\nиспользуйте /start_custom для запуска турнира с этой конфигурацией",
			config.Limit, config.LichessLimit, config.ChesscomLimit, config.Intro))
	}

	return nil
}

func handleStartCustomParam(b *bot.Bot, update tgbotapi.Update) error {
	callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
	if _, err := b.Request(callback); err != nil {
		log.Printf("failed to answer callback: %v", err)
	}

	chatID := update.CallbackQuery.Message.Chat.ID
	messageID := update.CallbackQuery.Message.MessageID
	data := update.CallbackQuery.Data

	parts := strings.Split(data, ":")
	if len(parts) < 2 {
		return fmt.Errorf("invalid callback data: %s", data)
	}

	action := parts[1]

	if action == "cancel" {
		adminChatID := update.CallbackQuery.From.ID
		b.ClearAdminProcess(adminChatID)
		return b.EditMessage(chatID, messageID, "создание турнира отменено")
	}

	if action == "confirm" {
		adminChatID := update.CallbackQuery.From.ID
		process, exists := b.GetAdminProcess(adminChatID)
		if !exists || process.CustomConfig == nil {
			return b.EditMessage(chatID, messageID, "ошибка: конфигурация не найдена")
		}

		config := process.CustomConfig
		b.ClearAdminProcess(adminChatID)

		ctx := context.Background()
		if err := b.Tournament.CreateTournament(ctx, config.Limit, config.LichessLimit, config.ChesscomLimit, config.Intro); err != nil {
			return b.EditMessage(chatID, messageID, fmt.Sprintf("ошибка при создании турнира: %v", err))
		}

		return b.EditMessage(chatID, messageID, fmt.Sprintf("✅ турнир запущен:\n\nлимит: %d\nlichess<%d, chesscom<%d\n\nтекст: _%s_",
			config.Limit, config.LichessLimit, config.ChesscomLimit, config.Intro))
	}

	return nil
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

	case bot.ProcessTypeAllowToGreen:
		if err := db.SetAllowToGreen(user.ChatID, true); err != nil {
			b.ClearAdminProcess(adminChatID)
			return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("ошибка при обновлении статуса: %v", err))
		}

		b.ClearAdminProcess(adminChatID)

		return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("пользователю %s разрешено участие в зелёных турнирах вручную", username))
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

func handleAllowToGreen(b *bot.Bot, update tgbotapi.Update) error {
	adminChatID := update.Message.From.ID
	b.SetAdminProcess(adminChatID, bot.ProcessTypeAllowToGreen, "")
	return b.SendMessage(update.Message.Chat.ID, "введите telegram username пользователя для разрешения участия в зелёных турнирах:")
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
	case "save_defaults":
		return handleScheduleSaveDefaults(b, chatID, messageID)
	}

	return nil
}

func handleScheduleApprove(b *bot.Bot, chatID int64, messageID int) error {
	scheduler.ScheduleManager.SetApproved(true)
	scheduler.ScheduleManager.ClearEditingState()

	message := scheduler.ScheduleManager.FormatScheduleMessage()
	return b.EditMessage(chatID, messageID, message)
}

func handleScheduleSaveDefaults(b *bot.Bot, chatID int64, messageID int) error {
	if err := scheduler.ScheduleManager.SaveCurrentAsDefaults(); err != nil {
		return b.SendMessage(chatID, fmt.Sprintf("ошибка сохранения: %v", err))
	}

	message := scheduler.ScheduleManager.FormatScheduleMessage()
	message += "\n\n*текущие настройки сохранены как дефолт*"
	keyboard := cron.GetScheduleMainKeyboard()

	return b.EditMessageWithButtons(chatID, messageID, message, keyboard)
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
