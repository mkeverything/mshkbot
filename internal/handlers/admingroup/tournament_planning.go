package admingroup

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/google/uuid"
	"github.com/sukalov/mshkbot/internal/bot"
	"github.com/sukalov/mshkbot/internal/logger"
	"github.com/sukalov/mshkbot/internal/redis"
	"github.com/sukalov/mshkbot/internal/types"
)

// handlePlanTournament initiates the tournament planning flow
func handlePlanTournament(b *bot.Bot, update tgbotapi.Update) error {
	adminInfo := logger.ExtractUserInfoFromUpdate(&update, "admin group")
	b.Logger.LogInfo("Plan tournament initiated", adminInfo, "Admin initiated tournament planning")

	adminChatID := update.Message.From.ID

	// Initialize plan tournament state
	state := &bot.PlanTournamentState{
		Tournament: &bot.PlannedTournamentConfig{
			ID: uuid.New().String(),
		},
	}

	b.SetPlanTournamentState(adminChatID, "name", state)

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("отмена", "plan_tournament:cancel"),
		),
	)

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "*планирование турнира*\n\nшаг 1: введите название турнира (или отправьте '-' чтобы пропустить)")
	msg.ParseMode = "Markdown"
	msg.ReplyMarkup = keyboard

	_, err := b.Client.Send(msg)
	return err
}

// handlePlanTournamentInput handles messages during the tournament planning flow
func handlePlanTournamentInput(b *bot.Bot, update tgbotapi.Update) error {
	if update.Message == nil {
		return nil
	}

	adminChatID := update.Message.From.ID
	process, exists := b.GetAdminProcess(adminChatID)
	if !exists || process.Type != bot.ProcessTypePlanTournament {
		return nil
	}

	text := strings.TrimSpace(update.Message.Text)
	if text == "" {
		return nil
	}

	state := process.PlanTournamentState
	if state == nil || state.Tournament == nil {
		return nil
	}

	config := state.Tournament
	var nextStep string
	var nextPrompt string
	var validationError string

	switch process.CurrentStep {
	case "name":
		if text != "-" {
			config.Name = text
		}
		nextStep = "date"
		nextPrompt = "*планирование турнира*\n\nшаг 2: введите дату (формат: YYYY-MM-DD, например: 2026-02-01)"

	case "date":
		// Validate date format
		_, err := time.Parse("2006-01-02", text)
		if err != nil {
			validationError = "неверный формат даты. используйте YYYY-MM-DD (например: 2026-02-01)"
			break
		}
		config.Date = text
		nextStep = "start_time"
		nextPrompt = "*планирование турнира*\n\nшаг 3: введите время начала (формат: HH:MM, 24-часовой, московское время, например: 12:00)"

	case "start_time":
		// Validate time format
		_, err := time.Parse("15:04", text)
		if err != nil {
			validationError = "неверный формат времени. используйте HH:MM (например: 12:00)"
			break
		}
		config.StartTime = text
		nextStep = "end_date"

		// Show prompt with button to use same date
		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("та же дата", "plan_tournament:same_end_date"),
				tgbotapi.NewInlineKeyboardButtonData("отмена", "plan_tournament:cancel"),
			),
		)

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, "*планирование турнира*\n\nшаг 4: введите дату окончания (формат: YYYY-MM-DD, например: 2026-02-01)\n\nили нажмите кнопку ниже, если турнир заканчивается в тот же день")
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = keyboard
		_, err = b.Client.Send(msg)
		if err != nil {
			return err
		}
		b.SetPlanTournamentState(adminChatID, nextStep, state)
		return nil

	case "end_date":
		// Validate date format
		_, err := time.Parse("2006-01-02", text)
		if err != nil {
			validationError = "неверный формат даты. используйте YYYY-MM-DD (например: 2026-02-01)"
			break
		}
		config.EndDate = text
		nextStep = "end_time"
		nextPrompt = "*планирование турнира*\n\nшаг 5: введите время окончания (формат: HH:MM, 24-часовой, московское время, например: 21:00)"

	case "end_time":
		// Validate time format
		_, err := time.Parse("15:04", text)
		if err != nil {
			validationError = "неверный формат времени. используйте HH:MM (например: 21:00)"
			break
		}
		config.EndTime = text

		// Validate that end is after start
		moscowTZ := time.FixedZone("moscow", 3*60*60)
		startTimeStr := config.Date + " " + config.StartTime
		endTimeStr := config.EndDate + " " + config.EndTime
		startTime, _ := time.ParseInLocation("2006-01-02 15:04", startTimeStr, moscowTZ)
		endTime, _ := time.ParseInLocation("2006-01-02 15:04", endTimeStr, moscowTZ)
		if !endTime.After(startTime) {
			validationError = "время окончания должно быть позже времени начала"
			break
		}
		nextStep = "limit"
		nextPrompt = "*планирование турнира*\n\nшаг 6: введите лимит участников (число, например: 24)"

	case "limit":
		intVal, parseErr := strconv.Atoi(text)
		if parseErr != nil || intVal <= 0 {
			validationError = "введите положительное число"
			break
		}
		config.Limit = intVal
		nextStep = "lichess_limit"
		nextPrompt = "*планирование турнира*\n\nшаг 6: введите лимит рейтинга lichess (0 = без лимита, например: 1600)"

	case "lichess_limit":
		intVal, parseErr := strconv.Atoi(text)
		if parseErr != nil || intVal < 0 {
			validationError = "введите положительное число или 0"
			break
		}
		config.LichessLimit = intVal
		nextStep = "chesscom_limit"
		nextPrompt = "*планирование турнира*\n\nшаг 7: введите лимит рейтинга chess.com (0 = без лимита, например: 1200)"

	case "chesscom_limit":
		intVal, parseErr := strconv.Atoi(text)
		if parseErr != nil || intVal < 0 {
			validationError = "введите положительное число или 0"
			break
		}
		config.ChesscomLimit = intVal
		nextStep = "intro"
		nextPrompt = "*планирование турнира*\n\nшаг 8: введите текст объявления"

	case "intro":
		config.Intro = text
		nextStep = "confirm"

		// Build summary
		nameDisplay := config.Name
		if nameDisplay == "" {
			nameDisplay = "(без названия)"
		}

		// Format dates for display
		dateDisplay := config.Date
		if config.EndDate != "" && config.EndDate != config.Date {
			dateDisplay = fmt.Sprintf("%s - %s", config.Date, config.EndDate)
		}

		nextPrompt = fmt.Sprintf("*проверьте планирование турнира*\n\nназвание: %s\nдата: %s\nначало: %s\nокончание: %s\nлимит: %d\nlichess<%d, chesscom<%d\n\nтекст: _%s_\n\nсохранить турнир?",
			nameDisplay, dateDisplay, config.StartTime, config.EndTime, config.Limit, config.LichessLimit, config.ChesscomLimit, config.Intro)

		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("сохранить", "plan_tournament:confirm"),
				tgbotapi.NewInlineKeyboardButtonData("отмена", "plan_tournament:cancel"),
			),
		)

		msg := tgbotapi.NewMessage(update.Message.Chat.ID, nextPrompt)
		msg.ParseMode = "Markdown"
		msg.ReplyMarkup = keyboard
		_, err := b.Client.Send(msg)
		if err != nil {
			return err
		}
		return nil

	default:
		return nil
	}

	if validationError != "" {
		return b.SendMessage(update.Message.Chat.ID, validationError)
	}

	if nextStep != "" {
		b.SetPlanTournamentState(adminChatID, nextStep, state)

		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("отмена", "plan_tournament:cancel"),
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

	return nil
}

// handlePlanTournamentCallback handles callbacks for the plan tournament flow
func handlePlanTournamentCallback(b *bot.Bot, update tgbotapi.Update) error {
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
		return fmt.Errorf("invalid callback data: %s", data)
	}

	action := parts[1]

	if action == "cancel" {
		adminChatID := update.CallbackQuery.From.ID
		b.ClearAdminProcess(adminChatID)
		b.Logger.LogInfo("Tournament planning cancelled", adminInfo, "Admin cancelled tournament planning")
		return b.EditMessage(chatID, messageID, "планирование турнира отменено")
	}

	if action == "same_end_date" {
		adminChatID := update.CallbackQuery.From.ID
		process, exists := b.GetAdminProcess(adminChatID)
		if !exists || process.PlanTournamentState == nil || process.PlanTournamentState.Tournament == nil {
			return b.EditMessage(chatID, messageID, "ошибка: конфигурация не найдена")
		}

		config := process.PlanTournamentState.Tournament
		config.EndDate = config.Date // Set end date same as start date

		b.SetPlanTournamentState(adminChatID, "end_time", process.PlanTournamentState)

		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("отмена", "plan_tournament:cancel"),
			),
		)

		return b.EditMessageWithButtons(chatID, messageID,
			"*планирование турнира*\n\nшаг 5: введите время окончания (формат: HH:MM, 24-часовой, московское время, например: 21:00)",
			keyboard)
	}

	if action == "confirm" {
		adminChatID := update.CallbackQuery.From.ID
		process, exists := b.GetAdminProcess(adminChatID)
		if !exists || process.PlanTournamentState == nil || process.PlanTournamentState.Tournament == nil {
			b.Logger.LogError("Tournament planning failed", adminInfo, "Configuration not found for tournament planning", fmt.Errorf("config not found"))
			return b.EditMessage(chatID, messageID, "ошибка: конфигурация не найдена")
		}

		config := process.PlanTournamentState.Tournament

		// Parse times
		moscowTZ := time.FixedZone("moscow", 3*60*60)
		startTimeStr := config.Date + " " + config.StartTime
		// Use EndDate if set, otherwise fall back to start Date
		endDate := config.EndDate
		if endDate == "" {
			endDate = config.Date
		}
		endTimeStr := endDate + " " + config.EndTime

		startTime, err := time.ParseInLocation("2006-01-02 15:04", startTimeStr, moscowTZ)
		if err != nil {
			b.Logger.LogError("Tournament planning failed", adminInfo, "Failed to parse start time", err)
			return b.EditMessage(chatID, messageID, fmt.Sprintf("ошибка при обработке времени начала: %v", err))
		}

		endTime, err := time.ParseInLocation("2006-01-02 15:04", endTimeStr, moscowTZ)
		if err != nil {
			b.Logger.LogError("Tournament planning failed", adminInfo, "Failed to parse end time", err)
			return b.EditMessage(chatID, messageID, fmt.Sprintf("ошибка при обработке времени окончания: %v", err))
		}

		// Check for time conflicts
		ctx := context.Background()
		hasConflict, err := redis.HasTimeConflict(ctx, startTime, endTime, config.ID)
		if err != nil {
			b.Logger.LogError("Tournament planning failed", adminInfo, "Failed to check time conflicts", err)
			return b.EditMessage(chatID, messageID, fmt.Sprintf("ошибка при проверке конфликтов: %v", err))
		}
		if hasConflict {
			b.Logger.LogInfo("Tournament planning failed - time conflict", adminInfo, "Time conflict detected")
			return b.EditMessage(chatID, messageID, "ошибка: на это время уже запланирован другой турнир")
		}

		// Create planned tournament
		tournament := types.PlannedTournament{
			ID:            config.ID,
			Name:          config.Name,
			StartTime:     startTime.UTC(),
			EndTime:       endTime.UTC(),
			Limit:         config.Limit,
			LichessLimit:  config.LichessLimit,
			ChesscomLimit: config.ChesscomLimit,
			Intro:         config.Intro,
			Status:        types.StatusPlanned,
			CreatedAt:     time.Now().UTC(),
		}

		if err := redis.SavePlannedTournament(ctx, tournament); err != nil {
			b.Logger.LogError("Tournament planning failed", adminInfo, "Failed to save planned tournament", err)
			return b.EditMessage(chatID, messageID, fmt.Sprintf("ошибка при сохранении турнира: %v", err))
		}

		b.ClearAdminProcess(adminChatID)

		nameDisplay := tournament.Name
		if nameDisplay == "" {
			nameDisplay = "(без названия)"
		}

		// Format dates for display
		dateDisplay := config.Date
		endDateDisplay := ""
		if config.EndDate != "" && config.EndDate != config.Date {
			dateDisplay = fmt.Sprintf("%s - %s", config.Date, config.EndDate)
			endDateDisplay = config.EndDate
		}

		b.Logger.LogSuccess("Tournament planned", adminInfo, fmt.Sprintf("Tournament planned: %s on %s %s-%s", nameDisplay, dateDisplay, config.StartTime, config.EndTime))

		var successMsg string
		if endDateDisplay != "" {
			successMsg = fmt.Sprintf("✅ турнир запланирован:\n\nназвание: %s\nдата начала: %s\nдата окончания: %s\nначало: %s\nокончание: %s\nлимит: %d\nlichess<%d, chesscom<%d\n\nтурнир автоматически начнётся в запланированное время. используйте /start_tournament для немедленного запуска.",
				nameDisplay, config.Date, endDateDisplay, config.StartTime, config.EndTime, config.Limit, config.LichessLimit, config.ChesscomLimit)
		} else {
			successMsg = fmt.Sprintf("✅ турнир запланирован:\n\nназвание: %s\nдата: %s\nначало: %s\nокончание: %s\nлимит: %d\nlichess<%d, chesscom<%d\n\nтурнир автоматически начнётся в запланированное время. используйте /start_tournament для немедленного запуска.",
				nameDisplay, config.Date, config.StartTime, config.EndTime, config.Limit, config.LichessLimit, config.ChesscomLimit)
		}

		return b.EditMessage(chatID, messageID, successMsg)
	}

	return nil
}

// handlePlannedTournaments lists all planned tournaments
func handlePlannedTournaments(b *bot.Bot, update tgbotapi.Update) error {
	adminInfo := logger.ExtractUserInfoFromUpdate(&update, "admin group")
	b.Logger.LogInfo("Planned tournaments list requested", adminInfo, "Admin requested planned tournaments list")

	ctx := context.Background()
	tournaments, err := redis.GetPlannedTournaments(ctx)
	if err != nil {
		b.Logger.LogError("Failed to get planned tournaments", adminInfo, "Failed to retrieve planned tournaments", err)
		return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("ошибка при получении списка: %v", err))
	}

	if len(tournaments) == 0 {
		return b.SendMessage(update.Message.Chat.ID, "📋 запланированные турниры\n\nнет запланированных турниров. используйте /plan_tournament чтобы создать.")
	}

	message := "📋 запланированные турниры\n\n"

	activeCount := 0
	plannedCount := 0
	completedCount := 0

	for i, t := range tournaments {
		statusIcon := "⚪"
		switch t.Status {
		case types.StatusActive:
			statusIcon = "🟢"
			activeCount++
		case types.StatusPlanned:
			statusIcon = "🟡"
			plannedCount++
		case types.StatusCompleted:
			statusIcon = "✅"
			completedCount++
		case types.StatusCancelled:
			statusIcon = "❌"
		}

		name := t.Name
		if name == "" {
			name = "(без названия)"
		}

		// Convert to Moscow time for display
		moscowTZ := time.FixedZone("moscow", 3*60*60)
		startTime := t.StartTime.In(moscowTZ)
		endTime := t.EndTime.In(moscowTZ)

		message += fmt.Sprintf("%d. %s %s\n", i+1, statusIcon, name)
		message += fmt.Sprintf("   📅 %s %02d:%02d - %02d:%02d\n",
			startTime.Format("2006-01-02"),
			startTime.Hour(), startTime.Minute(),
			endTime.Hour(), endTime.Minute())
		message += fmt.Sprintf("   👥 лимит: %d", t.Limit)
		if t.LichessLimit > 0 || t.ChesscomLimit > 0 {
			message += fmt.Sprintf(" | lichess<%d, chesscom<%d", t.LichessLimit, t.ChesscomLimit)
		}
		message += "\n\n"
	}

	message += fmt.Sprintf("всего: %d (🟢 активных: %d, 🟡 запланированных: %d, ✅ завершённых: %d)",
		len(tournaments), activeCount, plannedCount, completedCount)

	return b.SendMessage(update.Message.Chat.ID, message)
}

// handleStartTournament shows planned tournaments to start immediately
func handleStartTournament(b *bot.Bot, update tgbotapi.Update) error {
	adminInfo := logger.ExtractUserInfoFromUpdate(&update, "admin group")
	b.Logger.LogInfo("Start tournament initiated", adminInfo, "Admin initiated tournament start")

	// Check if tournament already active
	if b.Tournament.Metadata.Exists {
		b.Logger.LogInfo("Start tournament failed - tournament active", adminInfo, "Tournament already active")
		return b.SendMessage(update.Message.Chat.ID, "турнир уже идёт. сначала остановите текущий турнир командой /stop_tournament")
	}

	ctx := context.Background()
	tournaments, err := redis.GetPlannedTournaments(ctx)
	if err != nil {
		b.Logger.LogError("Failed to get planned tournaments", adminInfo, "Failed to retrieve planned tournaments", err)
		return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("ошибка при получении списка: %v", err))
	}

	// Filter only planned tournaments
	var planned []types.PlannedTournament
	for _, t := range tournaments {
		if t.Status == types.StatusPlanned {
			planned = append(planned, t)
		}
	}

	if len(planned) == 0 {
		return b.SendMessage(update.Message.Chat.ID, "нет запланированных турниров для запуска. используйте /plan_tournament чтобы создать.")
	}

	// Build keyboard with tournament buttons
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, t := range planned {
		name := t.Name
		if name == "" {
			name = "(без названия)"
		}
		moscowTZ := time.FixedZone("moscow", 3*60*60)
		startTime := t.StartTime.In(moscowTZ)
		label := fmt.Sprintf("%s (%s %02d:%02d)", name, startTime.Format("2006-01-02"), startTime.Hour(), startTime.Minute())
		btn := tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("start_tournament:%s", t.ID))
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(btn))
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("отмена", "start_tournament:cancel"),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "выберите турнир для немедленного запуска:")
	msg.ReplyMarkup = keyboard

	_, err = b.Client.Send(msg)
	return err
}

// handleStartTournamentCallback handles the start tournament callback
func handleStartTournamentCallback(b *bot.Bot, update tgbotapi.Update) error {
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
		return fmt.Errorf("invalid callback data: %s", data)
	}

	action := parts[1]

	if action == "cancel" {
		b.Logger.LogInfo("Start tournament cancelled", adminInfo, "Admin cancelled tournament start")
		return b.EditMessage(chatID, messageID, "запуск турнира отменён")
	}

	// Get tournament ID
	tournamentID := action

	ctx := context.Background()
	tournament, err := redis.GetPlannedTournamentByID(ctx, tournamentID)
	if err != nil {
		b.Logger.LogError("Start tournament failed", adminInfo, "Failed to get tournament", err)
		return b.EditMessage(chatID, messageID, fmt.Sprintf("ошибка: турнир не найден: %v", err))
	}

	if tournament.Status != types.StatusPlanned {
		b.Logger.LogInfo("Start tournament failed - not planned", adminInfo, fmt.Sprintf("Tournament %s is not in planned status", tournamentID))
		return b.EditMessage(chatID, messageID, "ошибка: этот турнир уже не в статусе 'запланирован'")
	}

	// Check if tournament already active
	if b.Tournament.Metadata.Exists {
		b.Logger.LogInfo("Start tournament failed - tournament active", adminInfo, "Tournament already active")
		return b.EditMessage(chatID, messageID, "ошибка: турнир уже идёт")
	}

	name := tournament.Name
	if name == "" {
		name = "(без названия)"
	}

	// Start the tournament
	scheduler.StartPlannedTournament(*tournament, adminInfo)

	return b.EditMessage(chatID, messageID, fmt.Sprintf("✅ турнир '%s' запущен!\n\nтурнир автоматически завершится в запланированное время (%s). используйте /stop_tournament для немедленной остановки.",
		name, tournament.EndTime.In(time.FixedZone("moscow", 3*60*60)).Format("15:04")))
}

// handleCancelTournament shows planned tournaments to cancel
func handleCancelTournament(b *bot.Bot, update tgbotapi.Update) error {
	adminInfo := logger.ExtractUserInfoFromUpdate(&update, "admin group")
	b.Logger.LogInfo("Cancel tournament initiated", adminInfo, "Admin initiated tournament cancellation")

	ctx := context.Background()
	tournaments, err := redis.GetPlannedTournaments(ctx)
	if err != nil {
		b.Logger.LogError("Failed to get planned tournaments", adminInfo, "Failed to retrieve planned tournaments", err)
		return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("ошибка при получении списка: %v", err))
	}

	// Filter only planned tournaments (not active or completed)
	var planned []types.PlannedTournament
	for _, t := range tournaments {
		if t.Status == types.StatusPlanned {
			planned = append(planned, t)
		}
	}

	if len(planned) == 0 {
		return b.SendMessage(update.Message.Chat.ID, "нет запланированных турниров для отмены.")
	}

	// Build keyboard with tournament buttons
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, t := range planned {
		name := t.Name
		if name == "" {
			name = "(без названия)"
		}
		moscowTZ := time.FixedZone("moscow", 3*60*60)
		startTime := t.StartTime.In(moscowTZ)
		label := fmt.Sprintf("%s (%s %02d:%02d)", name, startTime.Format("2006-01-02"), startTime.Hour(), startTime.Minute())
		btn := tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("cancel_tournament:%s", t.ID))
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(btn))
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("отмена", "cancel_tournament:cancel"),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "выберите турнир для отмены:")
	msg.ReplyMarkup = keyboard

	_, err = b.Client.Send(msg)
	return err
}

// handleCancelTournamentCallback handles the cancel tournament callback
func handleCancelTournamentCallback(b *bot.Bot, update tgbotapi.Update) error {
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
		return fmt.Errorf("invalid callback data: %s", data)
	}

	action := parts[1]

	if action == "cancel" {
		b.Logger.LogInfo("Cancel tournament cancelled", adminInfo, "Admin cancelled tournament cancellation")
		return b.EditMessage(chatID, messageID, "отмена турнира отменена")
	}

	// Get tournament ID
	tournamentID := action

	ctx := context.Background()
	tournament, err := redis.GetPlannedTournamentByID(ctx, tournamentID)
	if err != nil {
		b.Logger.LogError("Cancel tournament failed", adminInfo, "Failed to get tournament", err)
		return b.EditMessage(chatID, messageID, fmt.Sprintf("ошибка: турнир не найден: %v", err))
	}

	if tournament.Status != types.StatusPlanned {
		b.Logger.LogInfo("Cancel tournament failed - not planned", adminInfo, fmt.Sprintf("Tournament %s is not in planned status", tournamentID))
		return b.EditMessage(chatID, messageID, "ошибка: можно отменить только запланированные турниры")
	}

	// Update tournament status to cancelled
	tournament.Status = types.StatusCancelled
	if err := redis.SavePlannedTournament(ctx, *tournament); err != nil {
		b.Logger.LogError("Cancel tournament failed", adminInfo, "Failed to update tournament status", err)
		return b.EditMessage(chatID, messageID, fmt.Sprintf("ошибка при отмене турнира: %v", err))
	}

	name := tournament.Name
	if name == "" {
		name = "(без названия)"
	}

	b.Logger.LogSuccess("Tournament cancelled", adminInfo, fmt.Sprintf("Tournament %s cancelled", name))

	return b.EditMessage(chatID, messageID, fmt.Sprintf("✅ турнир '%s' отменён", name))
}

// handleEditTournament shows planned tournaments to edit
func handleEditTournament(b *bot.Bot, update tgbotapi.Update) error {
	adminInfo := logger.ExtractUserInfoFromUpdate(&update, "admin group")
	b.Logger.LogInfo("Edit tournament initiated", adminInfo, "Admin initiated tournament editing")

	ctx := context.Background()
	tournaments, err := redis.GetPlannedTournaments(ctx)
	if err != nil {
		b.Logger.LogError("Failed to get planned tournaments", adminInfo, "Failed to retrieve planned tournaments", err)
		return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("ошибка при получении списка: %v", err))
	}

	// Filter only planned tournaments (not active or completed)
	var planned []types.PlannedTournament
	for _, t := range tournaments {
		if t.Status == types.StatusPlanned {
			planned = append(planned, t)
		}
	}

	if len(planned) == 0 {
		return b.SendMessage(update.Message.Chat.ID, "нет запланированных турниров для редактирования.")
	}

	// Build keyboard with tournament buttons
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, t := range planned {
		name := t.Name
		if name == "" {
			name = "(без названия)"
		}
		moscowTZ := time.FixedZone("moscow", 3*60*60)
		startTime := t.StartTime.In(moscowTZ)
		label := fmt.Sprintf("%s (%s %02d:%02d)", name, startTime.Format("2006-01-02"), startTime.Hour(), startTime.Minute())
		btn := tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("edit_tournament_select:%s", t.ID))
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(btn))
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("отмена", "edit_tournament:cancel"),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "выберите турнир для редактирования:")
	msg.ReplyMarkup = keyboard

	_, err = b.Client.Send(msg)
	return err
}

// handleEditTournamentCallback handles the edit tournament callbacks
func handleEditTournamentCallback(b *bot.Bot, update tgbotapi.Update) error {
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
		return fmt.Errorf("invalid callback data: %s", data)
	}

	action := parts[1]

	if action == "cancel" {
		b.Logger.LogInfo("Edit tournament cancelled", adminInfo, "Admin cancelled tournament editing")
		return b.EditMessage(chatID, messageID, "редактирование турнира отменено")
	}

	// Handle tournament selection
	if strings.HasPrefix(action, "edit_tournament_select") && len(parts) >= 3 {
		tournamentID := parts[2]

		ctx := context.Background()
		tournament, err := redis.GetPlannedTournamentByID(ctx, tournamentID)
		if err != nil {
			b.Logger.LogError("Edit tournament failed", adminInfo, "Failed to get tournament", err)
			return b.EditMessage(chatID, messageID, fmt.Sprintf("ошибка: турнир не найден: %v", err))
		}

		if tournament.Status != types.StatusPlanned {
			b.Logger.LogInfo("Edit tournament failed - not planned", adminInfo, fmt.Sprintf("Tournament %s is not in planned status", tournamentID))
			return b.EditMessage(chatID, messageID, "ошибка: можно редактировать только запланированные турниры")
		}

		// Show edit options
		name := tournament.Name
		if name == "" {
			name = "(без названия)"
		}

		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("название", fmt.Sprintf("edit_tournament_field:%s:name", tournamentID)),
				tgbotapi.NewInlineKeyboardButtonData("дата начала", fmt.Sprintf("edit_tournament_field:%s:date", tournamentID)),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("время начала", fmt.Sprintf("edit_tournament_field:%s:start_time", tournamentID)),
				tgbotapi.NewInlineKeyboardButtonData("дата окончания", fmt.Sprintf("edit_tournament_field:%s:end_date", tournamentID)),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("время окончания", fmt.Sprintf("edit_tournament_field:%s:end_time", tournamentID)),
				tgbotapi.NewInlineKeyboardButtonData("лимит участников", fmt.Sprintf("edit_tournament_field:%s:limit", tournamentID)),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("лимит lichess", fmt.Sprintf("edit_tournament_field:%s:lichess_limit", tournamentID)),
				tgbotapi.NewInlineKeyboardButtonData("лимит chess.com", fmt.Sprintf("edit_tournament_field:%s:chesscom_limit", tournamentID)),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("текст объявления", fmt.Sprintf("edit_tournament_field:%s:intro", tournamentID)),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("<< назад", "edit_tournament:cancel"),
			),
		)

		return b.EditMessageWithButtons(chatID, messageID,
			fmt.Sprintf("*редактирование турнира*\n\n%s\n\nвыберите поле для редактирования:", name),
			keyboard)
	}

	// Handle field selection
	if strings.HasPrefix(action, "edit_tournament_field") && len(parts) >= 4 {
		tournamentID := parts[2]
		field := parts[3]

		ctx := context.Background()
		tournament, err := redis.GetPlannedTournamentByID(ctx, tournamentID)
		if err != nil {
			b.Logger.LogError("Edit tournament failed", adminInfo, "Failed to get tournament", err)
			return b.EditMessage(chatID, messageID, fmt.Sprintf("ошибка: турнир не найден: %v", err))
		}

		// Store editing state in admin process
		adminChatID := update.CallbackQuery.From.ID
		state := &bot.PlanTournamentState{
			Tournament: &bot.PlannedTournamentConfig{
				ID:            tournament.ID,
				Name:          tournament.Name,
				Limit:         tournament.Limit,
				LichessLimit:  tournament.LichessLimit,
				ChesscomLimit: tournament.ChesscomLimit,
				Intro:         tournament.Intro,
			},
		}

		// Convert times to Moscow time for editing
		moscowTZ := time.FixedZone("moscow", 3*60*60)
		startTime := tournament.StartTime.In(moscowTZ)
		endTime := tournament.EndTime.In(moscowTZ)
		state.Tournament.Date = startTime.Format("2006-01-02")
		state.Tournament.StartTime = startTime.Format("15:04")
		state.Tournament.EndDate = endTime.Format("2006-01-02")
		state.Tournament.EndTime = endTime.Format("15:04")

		b.SetPlanTournamentState(adminChatID, fmt.Sprintf("edit_%s:%s", field, tournamentID), state)

		var fieldName string
		var currentValue string

		switch field {
		case "name":
			fieldName = "название"
			currentValue = tournament.Name
			if currentValue == "" {
				currentValue = "(без названия)"
			}
		case "date":
			fieldName = "дата начала"
			currentValue = startTime.Format("2006-01-02")
		case "start_time":
			fieldName = "время начала"
			currentValue = startTime.Format("15:04")
		case "end_date":
			fieldName = "дата окончания"
			currentValue = endTime.Format("2006-01-02")
		case "end_time":
			fieldName = "время окончания"
			currentValue = endTime.Format("15:04")
		case "limit":
			fieldName = "лимит участников"
			currentValue = fmt.Sprintf("%d", tournament.Limit)
		case "lichess_limit":
			fieldName = "лимит рейтинга lichess"
			currentValue = fmt.Sprintf("%d", tournament.LichessLimit)
		case "chesscom_limit":
			fieldName = "лимит рейтинга chess.com"
			currentValue = fmt.Sprintf("%d", tournament.ChesscomLimit)
		case "intro":
			fieldName = "текст объявления"
			currentValue = tournament.Intro
		}

		return b.EditMessage(chatID, messageID,
			fmt.Sprintf("*редактирование турнира*\n\nполе: %s\nтекущее значение: %s\n\nотправьте новое значение:", fieldName, currentValue))
	}

	return nil
}

// handleEditTournamentInput handles input during tournament editing
func handleEditTournamentInput(b *bot.Bot, update tgbotapi.Update) error {
	if update.Message == nil {
		return nil
	}

	adminChatID := update.Message.From.ID
	process, exists := b.GetAdminProcess(adminChatID)
	if !exists || process.Type != bot.ProcessTypePlanTournament {
		return nil
	}

	// Check if we're in edit mode
	if !strings.HasPrefix(process.CurrentStep, "edit_") {
		return nil
	}

	text := strings.TrimSpace(update.Message.Text)
	if text == "" {
		return nil
	}

	// Parse the edit step
	parts := strings.Split(process.CurrentStep, ":")
	if len(parts) < 2 {
		return nil
	}

	fieldPart := parts[0] // e.g., "edit_name"
	tournamentID := parts[1]

	field := strings.TrimPrefix(fieldPart, "edit_")

	ctx := context.Background()
	tournament, err := redis.GetPlannedTournamentByID(ctx, tournamentID)
	if err != nil {
		return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("ошибка: турнир не найден: %v", err))
	}

	// Validate and update field
	var validationError string

	switch field {
	case "name":
		tournament.Name = text

	case "date":
		_, err := time.Parse("2006-01-02", text)
		if err != nil {
			validationError = "неверный формат даты. используйте YYYY-MM-DD"
		} else {
			// Update date while keeping time
			moscowTZ := time.FixedZone("moscow", 3*60*60)
			oldStart := tournament.StartTime.In(moscowTZ)
			oldEnd := tournament.EndTime.In(moscowTZ)
			newStart, _ := time.ParseInLocation("2006-01-02 15:04", text+" "+oldStart.Format("15:04"), moscowTZ)
			newEnd, _ := time.ParseInLocation("2006-01-02 15:04", text+" "+oldEnd.Format("15:04"), moscowTZ)
			tournament.StartTime = newStart.UTC()
			tournament.EndTime = newEnd.UTC()
		}

	case "start_time":
		_, err := time.Parse("15:04", text)
		if err != nil {
			validationError = "неверный формат времени. используйте HH:MM"
		} else {
			// Update start time
			moscowTZ := time.FixedZone("moscow", 3*60*60)
			oldStart := tournament.StartTime.In(moscowTZ)
			newStart, _ := time.ParseInLocation("2006-01-02 15:04", oldStart.Format("2006-01-02")+" "+text, moscowTZ)
			if !newStart.Before(tournament.EndTime.In(moscowTZ)) {
				validationError = "время начала должно быть раньше времени окончания"
			} else {
				tournament.StartTime = newStart.UTC()
			}
		}

	case "end_date":
		_, err := time.Parse("2006-01-02", text)
		if err != nil {
			validationError = "неверный формат даты. используйте YYYY-MM-DD"
		} else {
			// Update end date while keeping end time
			moscowTZ := time.FixedZone("moscow", 3*60*60)
			oldEnd := tournament.EndTime.In(moscowTZ)
			newEnd, _ := time.ParseInLocation("2006-01-02 15:04", text+" "+oldEnd.Format("15:04"), moscowTZ)
			if !newEnd.After(tournament.StartTime.In(moscowTZ)) {
				validationError = "дата окончания должна быть позже даты начала"
			} else {
				tournament.EndTime = newEnd.UTC()
			}
		}

	case "end_time":
		_, err := time.Parse("15:04", text)
		if err != nil {
			validationError = "неверный формат времени. используйте HH:MM"
		} else {
			// Update end time
			moscowTZ := time.FixedZone("moscow", 3*60*60)
			oldEnd := tournament.EndTime.In(moscowTZ)
			newEnd, _ := time.ParseInLocation("2006-01-02 15:04", oldEnd.Format("2006-01-02")+" "+text, moscowTZ)
			if !newEnd.After(tournament.StartTime.In(moscowTZ)) {
				validationError = "время окончания должно быть позже времени начала"
			} else {
				tournament.EndTime = newEnd.UTC()
			}
		}

	case "limit":
		intVal, err := strconv.Atoi(text)
		if err != nil || intVal <= 0 {
			validationError = "введите положительное число"
		} else {
			tournament.Limit = intVal
		}

	case "lichess_limit":
		intVal, err := strconv.Atoi(text)
		if err != nil || intVal < 0 {
			validationError = "введите положительное число или 0"
		} else {
			tournament.LichessLimit = intVal
		}

	case "chesscom_limit":
		intVal, err := strconv.Atoi(text)
		if err != nil || intVal < 0 {
			validationError = "введите положительное число или 0"
		} else {
			tournament.ChesscomLimit = intVal
		}

	case "intro":
		tournament.Intro = text
	}

	if validationError != "" {
		return b.SendMessage(update.Message.Chat.ID, validationError)
	}

	// Check for time conflicts after date/time changes
	if field == "date" || field == "start_time" || field == "end_date" || field == "end_time" {
		hasConflict, err := redis.HasTimeConflict(ctx, tournament.StartTime, tournament.EndTime, tournament.ID)
		if err != nil {
			return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("ошибка при проверке конфликтов: %v", err))
		}
		if hasConflict {
			return b.SendMessage(update.Message.Chat.ID, "ошибка: на это время уже запланирован другой турнир")
		}
	}

	// Save updated tournament
	if err := redis.SavePlannedTournament(ctx, *tournament); err != nil {
		return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("ошибка при сохранении: %v", err))
	}

	// Clear admin process
	b.ClearAdminProcess(adminChatID)

	return b.SendMessage(update.Message.Chat.ID, "✅ изменения сохранены")
}

// handleDebugTournaments shows all tournaments with detailed debug info
func handleDebugTournaments(b *bot.Bot, update tgbotapi.Update) error {
	adminInfo := logger.ExtractUserInfoFromUpdate(&update, "admin group")
	b.Logger.LogInfo("Debug tournaments requested", adminInfo, "Admin requested debug tournament list")

	ctx := context.Background()
	tournaments, err := redis.GetPlannedTournaments(ctx)
	if err != nil {
		b.Logger.LogError("Failed to get planned tournaments", adminInfo, "Failed to retrieve planned tournaments", err)
		return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("ошибка при получении списка: %v", err))
	}

	if len(tournaments) == 0 {
		return b.SendMessage(update.Message.Chat.ID, "🔍 отладка: запланированные турниры\n\nв redis нет турниров")
	}

	message := "🔍 отладка: все запланированные турниры\n\n"

	for i, t := range tournaments {
		statusIcon := "⚪"
		switch t.Status {
		case types.StatusActive:
			statusIcon = "🟢"
		case types.StatusPlanned:
			statusIcon = "🟡"
		case types.StatusCompleted:
			statusIcon = "✅"
		case types.StatusCancelled:
			statusIcon = "❌"
		default:
			statusIcon = "⚠️"
		}

		name := t.Name
		if name == "" {
			name = "(без названия)"
		}

		// Convert to Moscow time for display
		moscowTZ := time.FixedZone("moscow", 3*60*60)
		startTime := t.StartTime.In(moscowTZ)
		endTime := t.EndTime.In(moscowTZ)

		message += fmt.Sprintf("%d. %s %s\n", i+1, statusIcon, name)
		message += fmt.Sprintf("   🆔 id: %s\n", t.ID)
		message += fmt.Sprintf("   📊 статус: %q\n", t.Status)
		message += fmt.Sprintf("   📅 %s %02d:%02d - %02d:%02d\n",
			startTime.Format("2006-01-02"),
			startTime.Hour(), startTime.Minute(),
			endTime.Hour(), endTime.Minute())
		message += fmt.Sprintf("   👥 лимит: %d", t.Limit)
		if t.LichessLimit > 0 || t.ChesscomLimit > 0 {
			message += fmt.Sprintf(" | lichess<%d, chesscom<%d", t.LichessLimit, t.ChesscomLimit)
		}
		message += "\n\n"
	}

	message += fmt.Sprintf("всего турниров в redis: %d", len(tournaments))

	return b.SendMessage(update.Message.Chat.ID, message)
}

// handleCleanupTournaments shows stuck tournaments and allows cleanup
func handleCleanupTournaments(b *bot.Bot, update tgbotapi.Update) error {
	adminInfo := logger.ExtractUserInfoFromUpdate(&update, "admin group")
	b.Logger.LogInfo("Cleanup tournaments initiated", adminInfo, "Admin initiated tournament cleanup")

	ctx := context.Background()
	tournaments, err := redis.GetPlannedTournaments(ctx)
	if err != nil {
		b.Logger.LogError("Failed to get planned tournaments", adminInfo, "Failed to retrieve planned tournaments", err)
		return b.SendMessage(update.Message.Chat.ID, fmt.Sprintf("ошибка при получении списка: %v", err))
	}

	// Find stuck tournaments (active but no active tournament in memory, or invalid status)
	var stuck []types.PlannedTournament
	for _, t := range tournaments {
		// Tournament is stuck if:
		// 1. Status is "active" but no tournament is currently running
		// 2. Status is empty or invalid
		// 3. Status is not one of the known statuses
		isStuck := false
		if t.Status == types.StatusActive && !b.Tournament.Metadata.Exists {
			isStuck = true
		} else if t.Status != types.StatusPlanned &&
			t.Status != types.StatusActive &&
			t.Status != types.StatusCompleted &&
			t.Status != types.StatusCancelled {
			isStuck = true
		}

		if isStuck {
			stuck = append(stuck, t)
		}
	}

	if len(stuck) == 0 {
		return b.SendMessage(update.Message.Chat.ID, "🧹 очистка турниров\n\nзастрявших турниров не найдено. все турниры имеют корректный статус.")
	}

	// Build keyboard with stuck tournament buttons
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, t := range stuck {
		name := t.Name
		if name == "" {
			name = "(без названия)"
		}
		moscowTZ := time.FixedZone("moscow", 3*60*60)
		startTime := t.StartTime.In(moscowTZ)
		label := fmt.Sprintf("%s (%s) - %s %02d:%02d", name, t.Status, startTime.Format("2006-01-02"), startTime.Hour(), startTime.Minute())
		btn := tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("cleanup_tournament:%s", t.ID))
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(btn))
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("отмена", "cleanup_tournament:cancel"),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)
	msg := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("🧹 найдено %d застрявших турниров:\n\nвыберите турнир для удаления или установки статуса 'завершён':", len(stuck)))
	msg.ReplyMarkup = keyboard

	_, err = b.Client.Send(msg)
	return err
}

// handleCleanupTournamentCallback handles the cleanup tournament callback
func handleCleanupTournamentCallback(b *bot.Bot, update tgbotapi.Update) error {
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
		return fmt.Errorf("invalid callback data: %s", data)
	}

	action := parts[1]

	if action == "cancel" {
		b.Logger.LogInfo("Cleanup tournament cancelled", adminInfo, "Admin cancelled tournament cleanup")
		return b.EditMessage(chatID, messageID, "очистка турниров отменена")
	}

	// Get tournament ID
	tournamentID := action

	ctx := context.Background()
	tournament, err := redis.GetPlannedTournamentByID(ctx, tournamentID)
	if err != nil {
		b.Logger.LogError("Cleanup tournament failed", adminInfo, "Failed to get tournament", err)
		return b.EditMessage(chatID, messageID, fmt.Sprintf("ошибка: турнир не найден: %v", err))
	}

	// Show options: mark as completed or delete
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ отметить как завершённый", fmt.Sprintf("cleanup_action:%s:complete", tournamentID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗑 удалить полностью", fmt.Sprintf("cleanup_action:%s:delete", tournamentID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("отмена", "cleanup_tournament:cancel"),
		),
	)

	name := tournament.Name
	if name == "" {
		name = "(без названия)"
	}

	return b.EditMessageWithButtons(chatID, messageID,
		fmt.Sprintf("🧹 очистка турнира\n\n%s\nтекущий статус: %q\n\nвыберите действие:", name, tournament.Status),
		keyboard)
}

// handleCleanupActionCallback handles the cleanup action (complete or delete)
func handleCleanupActionCallback(b *bot.Bot, update tgbotapi.Update) error {
	adminInfo := logger.ExtractUserInfoFromUpdate(&update, "admin group")

	callback := tgbotapi.NewCallback(update.CallbackQuery.ID, "")
	if _, err := b.Request(callback); err != nil {
		log.Printf("failed to answer callback: %v", err)
	}

	chatID := update.CallbackQuery.Message.Chat.ID
	messageID := update.CallbackQuery.Message.MessageID
	data := update.CallbackQuery.Data

	parts := strings.Split(data, ":")
	if len(parts) < 3 {
		return fmt.Errorf("invalid callback data: %s", data)
	}

	tournamentID := parts[1]
	action := parts[2]

	ctx := context.Background()

	if action == "complete" {
		tournament, err := redis.GetPlannedTournamentByID(ctx, tournamentID)
		if err != nil {
			b.Logger.LogError("Cleanup tournament failed", adminInfo, "Failed to get tournament", err)
			return b.EditMessage(chatID, messageID, fmt.Sprintf("ошибка: турнир не найден: %v", err))
		}

		tournament.Status = types.StatusCompleted
		if err := redis.SavePlannedTournament(ctx, *tournament); err != nil {
			b.Logger.LogError("Cleanup tournament failed", adminInfo, "Failed to update tournament status", err)
			return b.EditMessage(chatID, messageID, fmt.Sprintf("ошибка при обновлении статуса: %v", err))
		}

		name := tournament.Name
		if name == "" {
			name = "(без названия)"
		}

		b.Logger.LogSuccess("Tournament cleaned up", adminInfo, fmt.Sprintf("Tournament %s marked as completed", name))
		return b.EditMessage(chatID, messageID, fmt.Sprintf("✅ турнир '%s' отмечен как завершённый", name))
	}

	if action == "delete" {
		tournament, err := redis.GetPlannedTournamentByID(ctx, tournamentID)
		if err != nil {
			b.Logger.LogError("Cleanup tournament failed", adminInfo, "Failed to get tournament", err)
			return b.EditMessage(chatID, messageID, fmt.Sprintf("ошибка: турнир не найден: %v", err))
		}

		if err := redis.DeletePlannedTournament(ctx, tournamentID); err != nil {
			b.Logger.LogError("Cleanup tournament failed", adminInfo, "Failed to delete tournament", err)
			return b.EditMessage(chatID, messageID, fmt.Sprintf("ошибка при удалении турнира: %v", err))
		}

		name := tournament.Name
		if name == "" {
			name = "(без названия)"
		}

		b.Logger.LogSuccess("Tournament deleted", adminInfo, fmt.Sprintf("Tournament %s deleted", name))
		return b.EditMessage(chatID, messageID, fmt.Sprintf("🗑 турнир '%s' удалён", name))
	}

	return nil
}
