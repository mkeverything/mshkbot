package admingroup

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sukalov/mshkbot/internal/bot"
	"github.com/sukalov/mshkbot/internal/cron"
	"github.com/sukalov/mshkbot/internal/db"
	"github.com/sukalov/mshkbot/internal/handlers/maingroup"
	"github.com/sukalov/mshkbot/internal/logger"
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
			"plan_tournament":      handlePlanTournament,
			"planned_tournaments":  handlePlannedTournaments,
			"start_tournament":     handleStartTournament,
			"cancel_tournament":    handleCancelTournament,
			"edit_tournament":      handleEditTournament,
			"debug_tournaments":    handleDebugTournaments,
			"cleanup_tournaments":  handleCleanupTournaments,
			"suspend_from_green":   handleSuspendFromGreen,
			"ban_player":           handleBanPlayer,
			"unban_player":         handleUnbanPlayer,
			"admit_to_green":       handleAdmitToGreen,
			"allow_to_green":       handleAllowToGreen,
			"force_checkout":       handleForceCheckout,
			"test_transliteration": handleTestTransliteration,
			"transliterate_all":    handleTransliterateAll,
			"send_schedule":        handleSendSchedule,
		},
		Messages: []func(b *bot.Bot, update tgbotapi.Update) error{
			handleScheduleFieldInput,
			handlePlanTournamentInput,
			handleEditTournamentInput,
			handleAdminMessage,
		},
		Callbacks: map[string]func(b *bot.Bot, update tgbotapi.Update) error{
			"suspend_duration":       handleSuspendDuration,
			"ban_duration":           handleBanDuration,
			"schedule":               handleScheduleCallback,
			"stop_tournament":        handleStopTournamentCallback,
			"force_checkout":         handleForceCheckoutCallback,
			"plan_tournament":        handlePlanTournamentCallback,
			"start_tournament":       handleStartTournamentCallback,
			"cancel_tournament":      handleCancelTournamentCallback,
			"edit_tournament":        handleEditTournamentCallback,
			"edit_tournament_select": handleEditTournamentCallback,
			"edit_tournament_field":  handleEditTournamentCallback,
			"cleanup_tournament":     handleCleanupTournamentCallback,
			"cleanup_action":         handleCleanupActionCallback,
		},
	}
}

func handleHelp(b *bot.Bot, update tgbotapi.Update) error {
	return b.SendMessage(update.Message.Chat.ID, "команды администратора:\n\n/tournament - показать состояние текущего турнира\n\n/plan_tournament - запланировать новый турнир с расписанием\n\n/planned_tournaments - список всех запланированных турниров\n\n/start_tournament - немедленно запустить запланированный турнир\n\n/cancel_tournament - отменить запланированный турнир\n\n/edit_tournament - изменить запланированный турнир\n\n/stop_tournament - остановить текущий турнир\n\n/debug_tournaments - отладка: показать все турниры в redis\n\n/cleanup_tournaments - очистка: найти и исправить застрявшие турниры\n\n/send_schedule - показать расписание на неделю\n\n/suspend_from_green - отстранить пользователя от зелёных турниров\n\n/admit_to_green - допустить пользователя к зелёным турнирам\n\n/allow_to_green - разрешить пользователю участие в зелёном турнире вручную\n\n/force_checkout - принудительно выписать пользователя из турнира\n\n/ban_player - забанить пользователя\n\n/unban_player - разбанить пользователя")
}

func handleTournamentJSON(b *bot.Bot, update tgbotapi.Update) error {
	errorContext := utils.CreateErrorContext(&update, "handle_tournament_json")
	adminInfo := logger.ExtractUserInfoFromUpdate(&update, "admin group")

	if b.Tournament == nil {
		b.Logger.LogError("handle_tournament_json", adminInfo, "tournament manager is nil", fmt.Errorf("tournament manager is nil"))
		b.SendMessage(update.Message.Chat.ID, "ошибка: менеджер турниров не инициализирован")
		return nil
	}

	jsonStr, err := b.Tournament.GetTournamentJSON()
	if err != nil {
		b.Logger.LogError("handle_tournament_json", adminInfo, "failed to get tournament json", err)
		fallbackMsg := utils.FormatUserErrorMessage(errorContext.TraceID, "ошибка при получении данных турнира")
		b.SendMessage(update.Message.Chat.ID, fallbackMsg)
		return nil
	}

	if jsonStr == "" {
		b.SendMessage(update.Message.Chat.ID, "данные турнира пусты")
		return nil
	}

	if err := b.SendMessageWithMarkdown(update.Message.Chat.ID, fmt.Sprintf("```json\n%s```", jsonStr), true); err != nil {
		b.Logger.LogError("handle_tournament_json", adminInfo, "failed to send tournament json", err)
		return nil
	}
	return nil
}

func handleTournament(b *bot.Bot, update tgbotapi.Update) error {
	errorContext := utils.CreateErrorContext(&update, "handle_tournament")
	adminInfo := logger.ExtractUserInfoFromUpdate(&update, "admin group")

	if b.Tournament == nil {
		b.Logger.LogError("handle_tournament", adminInfo, "tournament manager is nil", fmt.Errorf("tournament manager is nil"))
		b.SendMessage(update.Message.Chat.ID, "ошибка: менеджер турниров не инициализирован")
		return nil
	}

	if !b.Tournament.Metadata.Exists {
		b.SendMessage(update.Message.Chat.ID, "турнир не создан")
		return nil
	}

	message, err := buildTournamentMessageForAdmin(b)
	if err != nil {
		b.Logger.LogError("handle_tournament", adminInfo, "failed to build tournament message", err)
		fallbackMsg := utils.FormatUserErrorMessage(errorContext.TraceID, "ошибка при получении данных турнира")
		b.SendMessage(update.Message.Chat.ID, fallbackMsg)
		return nil
	}

	if err := b.SendMessageWithMarkdownV2(update.Message.Chat.ID, message, true); err != nil {
		b.Logger.LogError("handle_tournament", adminInfo, "failed to send tournament message", err)
		return nil
	}
	return nil
}

func formatPlayerLineForAdmin(num int, player types.Player) string {
	if player.SavedName == "" {
		return fmt.Sprintf("%d\\. \\(unknown\\)", num)
	}

	escapedName := utils.EscapeMDV2(player.SavedName)

	var playerLine string
	if player.CheckinMessageID != 0 && player.CheckinChatID != 0 {
		chatIDForLink := player.CheckinChatID
		if chatIDForLink < 0 {
			chatIDForLink = -chatIDForLink - 1000000000000
		}
		if chatIDForLink > 0 {
			messageLink := fmt.Sprintf("https://t.me/c/%d/%d", chatIDForLink, player.CheckinMessageID)
			playerLine = fmt.Sprintf("%d\\. [%s](%s)", num, escapedName, messageLink)
		} else {
			playerLine = fmt.Sprintf("%d\\. %s", num, escapedName)
		}
	} else {
		playerLine = fmt.Sprintf("%d\\. %s", num, escapedName)
	}

	if player.Username != "" {
		playerLine += fmt.Sprintf(" \\(@%s\\)", utils.EscapeMDV2(player.Username))
	}

	if player.PeakRating != nil {
		rating := utils.EscapeMDV2(fmt.Sprintf("%d", player.PeakRating.BlitzPeak))
		switch player.PeakRating.Site {
		case types.SiteLichess:
			if player.PeakRating.SiteUsername != "" {
				siteURL := fmt.Sprintf("https://lichess.org/@/%s", player.PeakRating.SiteUsername)
				playerLine += fmt.Sprintf(" \\([lichess](%s) %s\\)", siteURL, rating)
			} else {
				playerLine += fmt.Sprintf(" \\(lichess %s\\)", rating)
			}
		case types.SiteChesscom:
			if player.PeakRating.SiteUsername != "" {
				siteURL := fmt.Sprintf("https://www.chess.com/member/%s", player.PeakRating.SiteUsername)
				playerLine += fmt.Sprintf(" \\([chesscom](%s) %s\\)", siteURL, rating)
			} else {
				playerLine += fmt.Sprintf(" \\(chesscom %s\\)", rating)
			}
		default:
			playerLine += fmt.Sprintf(" \\(%s %s\\)", utils.EscapeMDV2(string(player.PeakRating.Site)), rating)
		}
	}

	return playerLine
}

func formatPlayerLineForAdminWithMetadata(num int, player types.Player) string {
	playerLine := formatPlayerLineForAdmin(num, player)

	if player.AllowToGreen {
		playerLine += " 🍀"
	}

	return playerLine
}

func escapeMDV2Header(s string) string {
	return utils.EscapeMDV2(s)
}

func buildTournamentMessageForAdmin(b *bot.Bot) (string, error) {
	if b.Tournament == nil {
		return "", fmt.Errorf("tournament manager is nil")
	}

	if b.Tournament.List == nil {
		return "", fmt.Errorf("tournament list is nil")
	}

	message := escapeMDV2Header("участники:") + "\n"

	count := 1
	for _, player := range b.Tournament.List {
		if player.State == types.StateInTournament {
			message += formatPlayerLineForAdminWithMetadata(count, player) + "\n"
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
		message += "\n" + escapeMDV2Header("очередь:") + "\n"
		for i, player := range queuedPlayers {
			message += formatPlayerLineForAdminWithMetadata(i+1, player) + "\n"
		}
	}

	return message, nil
}

func handleStopTournament(b *bot.Bot, update tgbotapi.Update) error {
	adminInfo := logger.ExtractUserInfoFromUpdate(&update, "admin group")
	b.Logger.LogInfo("Tournament stop initiated", adminInfo, "Admin initiated tournament stop process")

	if !b.Tournament.Metadata.Exists {
		b.Logger.LogInfo("Tournament stop failed - no tournament", adminInfo, "Admin attempted to stop tournament but no tournament exists")
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
	adminInfo := logger.ExtractUserInfoFromUpdate(&update, "admin group")

	callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
	if _, err := b.Request(callback); err != nil {
		log.Printf("failed to answer callback: %v", err)
	}

	chatID := update.CallbackQuery.Message.Chat.ID
	messageID := update.CallbackQuery.Message.MessageID
	data := update.CallbackQuery.Data

	parts := strings.Split(data, ":")
	if len(parts) < 2 {
		b.Logger.LogError("Tournament stop callback failed", adminInfo, "Invalid callback data format", fmt.Errorf("invalid callback data: %s", data))
		return fmt.Errorf("invalid callback data: %s", data)
	}

	action := parts[1]

	if action == "cancel" {
		b.Logger.LogInfo("Tournament stop cancelled", adminInfo, "Admin cancelled tournament stop")
		return b.EditMessage(chatID, messageID, "отмена: турнир не будет остановлен")
	}

	if action == "confirm" {
		ctx := context.Background()
		if err := scheduler.EndCurrentTournament(ctx, adminInfo); err != nil {
			return b.EditMessage(chatID, messageID, fmt.Sprintf("ошибка при остановке турнира: %v", err))
		}
		return b.EditMessage(chatID, messageID, "✅ турнир остановлен")
	}

	return nil
}

// PrivacyHiddenMessage indicates that privacy settings hide the user ID
const PrivacyHiddenMessage = `этот человек выставил максимальные настройки приватности и его невозможно определить автоматически, так что вам придётся сделать следующее:

1. открываем web.telegram.org
2. логинимся
3. открываем чат с этим игроком
4. копируем ссылку и присылаем сюда

p.s. если у него указан юзернейм всё ещё можно прислать его`

var webTelegramRegex = regexp.MustCompile(`web\.telegram\.org/(?:k|a)/#(-?\d+)`)

func resolveUserFromInput(b *bot.Bot, update tgbotapi.Update) (*db.User, error) {
	text := strings.TrimSpace(update.Message.Text)

	// 1. Try Web Telegram URL
	if strings.Contains(text, "web.telegram.org") {
		matches := webTelegramRegex.FindStringSubmatch(text)
		if len(matches) > 1 {
			chatIDStr := matches[1]
			chatID, err := strconv.ParseInt(chatIDStr, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid chat id in url: %v", err)
			}
			user, err := db.GetByChatID(chatID)
			if err != nil {
				return nil, fmt.Errorf("user with id %d not found in db", chatID)
			}
			return &user, nil
		}
	}

	// 2. Try Username
	if text != "" && !strings.Contains(text, "web.telegram.org") {
		username := strings.TrimPrefix(text, "@")
		// Simple validation to distinguish from other text if needed, but for now assume any text not URL is username attempt
		// unless it's a forward
		if update.Message.ForwardFrom == nil && update.Message.ForwardDate == 0 {
			user, err := db.GetByUsername(username)
			if err == nil {
				return &user, nil
			}
		}
	}

	// 3. Try Forwarded Message
	if update.Message.ForwardFrom != nil {
		user, err := db.GetByChatID(update.Message.ForwardFrom.ID)
		if err != nil {
			return nil, fmt.Errorf("user %d not found in db. they must be registered first", update.Message.ForwardFrom.ID)
		}
		return &user, nil
	}

	// 4. Check for hidden forward
	if update.Message.ForwardDate != 0 && update.Message.ForwardFrom == nil {
		return nil, fmt.Errorf("privacy_hidden")
	}

	// Fallback: if text was provided but didn't match username above, return specific error
	if text != "" {
		// We already tried matching username. If we are here, it failed.
		return nil, fmt.Errorf("пользователь с юзернеймом %s не найден", strings.TrimPrefix(text, "@"))
	}

	return nil, fmt.Errorf("unknown input")
}

func handleAdminMessage(b *bot.Bot, update tgbotapi.Update) error {
	if update.Message == nil {
		return nil
	}

	adminChatID := update.Message.From.ID
	adminInfo := logger.ExtractUserInfoFromUpdate(&update, "admin group")

	process, exists := b.GetAdminProcess(adminChatID)
	if !exists {
		log.Printf("admin group message: %s", update.Message.Text)
		return nil
	}

	// Skip user resolution for tournament planning processes
	if process.Type == bot.ProcessTypePlanTournament {
		return nil
	}

	user, err := resolveUserFromInput(b, update)
	if err != nil {
		if err.Error() == "privacy_hidden" {
			return b.SendMessage(update.Message.Chat.ID, PrivacyHiddenMessage)
		}
		// Don't clear process on input error to allow retry
		b.Logger.LogError("User resolution failed", adminInfo, fmt.Sprintf("Failed to resolve user for %s: %v", process.Type, err), err)
		return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("ошибка: %v", err))
	}

	// User found, proceed with the process
	username := user.SavedName
	if user.Username != "" {
		username += fmt.Sprintf(" (@%s)", user.Username)
	}

	// Create user info for the target user
	targetUserInfo := &logger.UserInfo{
		ID:        user.ChatID,
		Username:  user.Username,
		FirstName: user.SavedName,
		ChatType:  "admin group",
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
			b.Logger.LogError("Suspension failed", adminInfo, "Unknown duration for suspension", fmt.Errorf("unknown duration: %s", process.Duration))
			return b.SendMessage(update.Message.Chat.ID, "неизвестная длительность")
		}

		if err := db.SetNotGreenUntil(user.ChatID, until); err != nil {
			b.ClearAdminProcess(adminChatID)
			b.Logger.LogError("Suspension failed", adminInfo, fmt.Sprintf("Failed to suspend user %s from green tournaments", username), err)
			return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("ошибка при обновлении статуса: %v", err))
		}

		b.ClearAdminProcess(adminChatID)

		durationText := "навсегда"
		if process.Duration == "month" {
			durationText = "на месяц"
		}

		b.Logger.LogSuccess("User suspended from green tournaments", targetUserInfo, fmt.Sprintf("User suspended from green tournaments by admin %s for %s", adminInfo.Username, durationText))
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
			b.Logger.LogError("Ban failed", adminInfo, "Unknown duration for ban", fmt.Errorf("unknown duration: %s", process.Duration))
			return b.SendMessage(update.Message.Chat.ID, "неизвестная длительность")
		}

		if err := db.SetBannedUntil(user.ChatID, until); err != nil {
			b.ClearAdminProcess(adminChatID)
			b.Logger.LogError("Ban failed", adminInfo, fmt.Sprintf("Failed to ban user %s", username), err)
			return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("ошибка при обновлении статуса: %v", err))
		}

		b.ClearAdminProcess(adminChatID)

		durationText := "навсегда"
		if process.Duration == "month" {
			durationText = "на месяц"
		}

		b.Logger.LogSuccess("User banned", targetUserInfo, fmt.Sprintf("User banned by admin %s for %s", adminInfo.Username, durationText))
		return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("пользователь %s забанен %s", username, durationText))

	case bot.ProcessTypeUnban:
		if err := db.SetBannedUntil(user.ChatID, nil); err != nil {
			b.ClearAdminProcess(adminChatID)
			b.Logger.LogError("Unban failed", adminInfo, fmt.Sprintf("Failed to unban user %s", username), err)
			return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("ошибка при обновлении статуса: %v", err))
		}

		b.ClearAdminProcess(adminChatID)

		b.Logger.LogSuccess("User unbanned", targetUserInfo, fmt.Sprintf("User unbanned by admin %s", adminInfo.Username))
		return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("пользователь %s разбанен", username))

	case bot.ProcessTypeAdmitToGreen:
		if err := db.SetNotGreenUntil(user.ChatID, nil); err != nil {
			b.ClearAdminProcess(adminChatID)
			b.Logger.LogError("Admit to green failed", adminInfo, fmt.Sprintf("Failed to admit user %s to green tournaments", username), err)
			return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("ошибка при обновлении статуса: %v", err))
		}

		b.ClearAdminProcess(adminChatID)

		b.Logger.LogSuccess("User admitted to green tournaments", targetUserInfo, fmt.Sprintf("User admitted to green tournaments by admin %s", adminInfo.Username))
		return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("пользователь %s допущен к зелёным турнирам", username))

	case bot.ProcessTypeAllowToGreen:
		if err := db.SetAllowToGreen(user.ChatID, true); err != nil {
			b.ClearAdminProcess(adminChatID)
			b.Logger.LogError("Allow to green failed", adminInfo, fmt.Sprintf("Failed to allow user %s to green tournaments", username), err)
			return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("ошибка при обновлении статуса: %v", err))
		}

		b.ClearAdminProcess(adminChatID)

		b.Logger.LogSuccess("User allowed to green tournaments", targetUserInfo, fmt.Sprintf("User manually allowed to green tournaments by admin %s", adminInfo.Username))
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

	if err := b.EditMessage(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID, "введите telegram username пользователя или перешлите его сообщение сюда:"); err != nil {
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

	if err := b.EditMessage(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID, "введите telegram username пользователя или перешлите его сообщение сюда:"); err != nil {
		return fmt.Errorf("failed to edit message: %w", err)
	}

	return nil
}

func handleUnbanPlayer(b *bot.Bot, update tgbotapi.Update) error {
	adminChatID := update.Message.From.ID
	b.SetAdminProcess(adminChatID, bot.ProcessTypeUnban, "")
	return b.SendMessage(update.Message.Chat.ID, "введите telegram username пользователя для разбана или перешлите его сообщение сюда:")
}

func handleAllowToGreen(b *bot.Bot, update tgbotapi.Update) error {
	adminChatID := update.Message.From.ID
	b.SetAdminProcess(adminChatID, bot.ProcessTypeAllowToGreen, "")
	return b.SendMessage(update.Message.Chat.ID, "введите telegram username пользователя или перешлите его сообщение сюда для разрешения участия в зелёных турнирах:")
}

func handleAdmitToGreen(b *bot.Bot, update tgbotapi.Update) error {
	adminChatID := update.Message.From.ID
	b.SetAdminProcess(adminChatID, bot.ProcessTypeAdmitToGreen, "")
	return b.SendMessage(update.Message.Chat.ID, "учтите, игрок всё равно может не пройти по рейтингу. эта команда просто снимет внутрней бан.\n\nвведите telegram_username пользователя или перешлите его сообщение сюда для допуска к зелёным турнирам:")
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

func handleForceCheckout(b *bot.Bot, update tgbotapi.Update) error {
	adminInfo := logger.ExtractUserInfoFromUpdate(&update, "admin group")
	b.Logger.LogInfo("Force checkout initiated", adminInfo, "Admin initiated force checkout process")

	if !b.Tournament.Metadata.Exists {
		b.Logger.LogInfo("Force checkout failed - no tournament", adminInfo, "Admin attempted force checkout but no tournament exists")
		return b.SendMessage(update.Message.Chat.ID, "запись сейчас не идёт")
	}

	activePlayers := []types.Player{}
	for _, p := range b.Tournament.List {
		if p.State == types.StateInTournament || p.State == types.StateQueued {
			activePlayers = append(activePlayers, p)
		}
	}

	if len(activePlayers) == 0 {
		b.Logger.LogInfo("Force checkout - no players", adminInfo, "Admin attempted force checkout but no active players in tournament")
		return b.SendMessage(update.Message.Chat.ID, "в турнире пока никого нет")
	}

	b.Logger.LogInfo("Force checkout - players listed", adminInfo, fmt.Sprintf("Admin shown %d active players for force checkout", len(activePlayers)))

	var rows [][]tgbotapi.InlineKeyboardButton
	for _, p := range activePlayers {
		btn := tgbotapi.NewInlineKeyboardButtonData(p.SavedName, fmt.Sprintf("force_checkout:%d", p.ID))
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(btn))
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "выберите игрока для удаления из турнира:")
	msg.ReplyMarkup = keyboard

	_, err := b.Client.Send(msg)
	return err
}

func handleForceCheckoutCallback(b *bot.Bot, update tgbotapi.Update) error {
	adminInfo := logger.ExtractUserInfoFromUpdate(&update, "admin group")

	callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
	if _, err := b.Request(callback); err != nil {
		log.Printf("failed to answer callback: %v", err)
	}

	data := update.CallbackQuery.Data
	parts := strings.Split(data, ":")
	if len(parts) < 2 {
		b.Logger.LogError("Force checkout callback failed", adminInfo, "Invalid callback data format", fmt.Errorf("invalid callback data: %s", data))
		return fmt.Errorf("invalid callback data: %s", data)
	}

	playerID, err := strconv.Atoi(parts[1])
	if err != nil {
		b.Logger.LogError("Force checkout callback failed", adminInfo, "Invalid player ID in callback data", fmt.Errorf("invalid player id: %s", parts[1]))
		return fmt.Errorf("invalid player id: %s", parts[1])
	}

	ctx := context.Background()
	var player *types.Player
	for _, p := range b.Tournament.List {
		if p.ID == playerID {
			player = &p
			break
		}
	}

	if player == nil || (player.State != types.StateInTournament && player.State != types.StateQueued) {
		b.Logger.LogInfo("Force checkout - player not found", adminInfo, fmt.Sprintf("Admin attempted to force checkout player %d but player not in tournament", playerID))
		return b.EditMessage(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID, "игрок уже не в турнире")
	}

	// Create user info for the target player
	targetPlayerInfo := &logger.UserInfo{
		ID:        int64(playerID),
		Username:  player.Username,
		FirstName: player.SavedName,
		ChatType:  "admin group",
	}

	wasInTournament := player.State == types.StateInTournament
	updatedPlayer := *player
	updatedPlayer.State = types.StateCheckedOut
	updatedPlayer.CheckedOutTime = time.Now().UTC()

	if err := b.Tournament.EditPlayer(ctx, playerID, updatedPlayer); err != nil {
		b.Logger.LogError("Force checkout failed", adminInfo, fmt.Sprintf("Failed to force checkout player %s", player.SavedName), err)
		return b.EditMessage(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID, fmt.Sprintf("ошибка при удалении игрока: %v", err))
	}

	b.Logger.LogSuccess("Player force checked out", targetPlayerInfo, fmt.Sprintf("Player force checked out from tournament by admin %s", adminInfo.Username))

	if err := db.DecrementTimesPlayed(int64(playerID)); err != nil {
		log.Printf("failed to decrement times played for user %d: %v", playerID, err)
		b.Logger.LogError("Times played decrement failed", adminInfo, fmt.Sprintf("Failed to decrement times played for player %s", player.SavedName), err)
	}

	if wasInTournament {
		if err := maingroup.PromoteQueuedPlayer(b, ctx); err != nil {
			log.Printf("failed to promote queued player: %v", err)
			b.Logger.LogError("Queue promotion failed", adminInfo, "Failed to promote queued player after force checkout", err)
		}
	}

	if err := maingroup.UpdateAnnouncementMessage(b, b.GetMainGroupID()); err != nil {
		log.Printf("failed to update announcement message: %v", err)
		b.Logger.LogError("Announcement update failed", adminInfo, "Failed to update tournament announcement after force checkout", err)
	}

	return b.EditMessage(update.CallbackQuery.Message.Chat.ID, update.CallbackQuery.Message.MessageID, fmt.Sprintf("✅ игрок %s удалён из турнира", player.SavedName))
}
