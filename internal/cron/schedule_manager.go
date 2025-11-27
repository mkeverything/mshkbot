package cron

import (
	"fmt"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type ScheduledEvent struct {
	ID            string       `json:"id"`
	Day           string       `json:"day"`
	Weekday       time.Weekday `json:"weekday"`
	StartHour     int          `json:"start_hour"`
	EndHour       int          `json:"end_hour"`
	Limit         int          `json:"limit"`
	LichessLimit  int          `json:"lichess_limit"`
	ChesscomLimit int          `json:"chesscom_limit"`
	Intro         string       `json:"intro"`
	Deleted       bool         `json:"deleted"`
}

type WeekSchedule struct {
	Events         []*ScheduledEvent `json:"events"`
	Approved       bool              `json:"approved"`
	MessageID      int               `json:"message_id"`
	EditingEventID string            `json:"editing_event_id"`
	EditingField   string            `json:"editing_field"`
}

type ScheduleManager struct {
	mu      sync.RWMutex
	current *WeekSchedule
}

func NewScheduleManager() *ScheduleManager {
	return &ScheduleManager{}
}

func (sm *ScheduleManager) GetDefaultEvents() []*ScheduledEvent {
	return []*ScheduledEvent{
		{
			ID:            "monday",
			Day:           "понедельник",
			Weekday:       time.Monday,
			StartHour:     12,
			EndHour:       21,
			Limit:         32,
			LichessLimit:  0,
			ChesscomLimit: 0,
			Intro:         "запись на южный турнир открыта! нажмите /checkin чтобы записаться",
		},
		{
			ID:            "tuesday",
			Day:           "вторник",
			Weekday:       time.Tuesday,
			StartHour:     12,
			EndHour:       21,
			Limit:         24,
			LichessLimit:  1600,
			ChesscomLimit: 1201,
			Intro:         "открыта запись на зелёный турнир. нажмите /checkin чтобы записаться",
		},
		{
			ID:            "wednesday",
			Day:           "среда",
			Weekday:       time.Wednesday,
			StartHour:     12,
			EndHour:       21,
			Limit:         24,
			LichessLimit:  0,
			ChesscomLimit: 0,
			Intro:         "начали запись на турнир в ладье. нажмите /checkin чтобы записаться",
		},
	}
}

func (sm *ScheduleManager) InitWeekSchedule() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.current = &WeekSchedule{
		Events:   sm.GetDefaultEvents(),
		Approved: false,
	}
}

func (sm *ScheduleManager) GetCurrentSchedule() *WeekSchedule {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.current
}

func (sm *ScheduleManager) IsApproved() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.current == nil {
		return false
	}
	return sm.current.Approved
}

func (sm *ScheduleManager) SetApproved(approved bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.current != nil {
		sm.current.Approved = approved
	}
}

func (sm *ScheduleManager) SetMessageID(messageID int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.current != nil {
		sm.current.MessageID = messageID
	}
}

func (sm *ScheduleManager) GetMessageID() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.current == nil {
		return 0
	}
	return sm.current.MessageID
}

func (sm *ScheduleManager) DeleteEvent(eventID string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.current == nil {
		return false
	}
	for _, e := range sm.current.Events {
		if e.ID == eventID {
			e.Deleted = true
			return true
		}
	}
	return false
}

func (sm *ScheduleManager) RestoreEvent(eventID string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.current == nil {
		return false
	}
	for _, e := range sm.current.Events {
		if e.ID == eventID {
			e.Deleted = false
			return true
		}
	}
	return false
}

func (sm *ScheduleManager) GetEvent(eventID string) *ScheduledEvent {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.current == nil {
		return nil
	}
	for _, e := range sm.current.Events {
		if e.ID == eventID {
			return e
		}
	}
	return nil
}

func (sm *ScheduleManager) SetEditingEvent(eventID, field string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.current != nil {
		sm.current.EditingEventID = eventID
		sm.current.EditingField = field
	}
}

func (sm *ScheduleManager) GetEditingState() (eventID, field string) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.current == nil {
		return "", ""
	}
	return sm.current.EditingEventID, sm.current.EditingField
}

func (sm *ScheduleManager) ClearEditingState() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.current != nil {
		sm.current.EditingEventID = ""
		sm.current.EditingField = ""
	}
}

func (sm *ScheduleManager) UpdateEventField(eventID, field string, value interface{}) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if sm.current == nil {
		return fmt.Errorf("no schedule initialized")
	}

	var event *ScheduledEvent
	for _, e := range sm.current.Events {
		if e.ID == eventID {
			event = e
			break
		}
	}
	if event == nil {
		return fmt.Errorf("event not found: %s", eventID)
	}

	switch field {
	case "limit":
		if v, ok := value.(int); ok {
			event.Limit = v
		} else {
			return fmt.Errorf("invalid value type for limit")
		}
	case "lichess_limit":
		if v, ok := value.(int); ok {
			event.LichessLimit = v
		} else {
			return fmt.Errorf("invalid value type for lichess_limit")
		}
	case "chesscom_limit":
		if v, ok := value.(int); ok {
			event.ChesscomLimit = v
		} else {
			return fmt.Errorf("invalid value type for chesscom_limit")
		}
	case "intro":
		if v, ok := value.(string); ok {
			event.Intro = v
		} else {
			return fmt.Errorf("invalid value type for intro")
		}
	default:
		return fmt.Errorf("unknown field: %s", field)
	}

	return nil
}

func (sm *ScheduleManager) GetActiveEvents() []*ScheduledEvent {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.current == nil {
		return nil
	}

	var active []*ScheduledEvent
	for _, e := range sm.current.Events {
		if !e.Deleted {
			active = append(active, e)
		}
	}
	return active
}

func (sm *ScheduleManager) GetEventForWeekday(weekday time.Weekday) *ScheduledEvent {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.current == nil || !sm.current.Approved {
		return nil
	}

	for _, e := range sm.current.Events {
		if e.Weekday == weekday && !e.Deleted {
			return e
		}
	}
	return nil
}

func (sm *ScheduleManager) FormatScheduleMessage() string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if sm.current == nil {
		return "расписание не инициализировано"
	}

	msg := "📅 *расписание турниров на неделю*\n\n"

	for _, e := range sm.current.Events {
		statusIcon := "✅"
		if e.Deleted {
			statusIcon = "❌"
		}

		msg += fmt.Sprintf("%s *%s* (%02d:00 - %02d:00)\n", statusIcon, e.Day, e.StartHour, e.EndHour)
		msg += fmt.Sprintf("   лимит: %d", e.Limit)
		if e.LichessLimit > 0 || e.ChesscomLimit > 0 {
			msg += fmt.Sprintf(" | lichess<%d, chesscom<%d", e.LichessLimit, e.ChesscomLimit)
		}
		msg += "\n"
		msg += fmt.Sprintf("   текст: _%s_\n\n", truncateString(e.Intro, 50))
	}

	if sm.current.Approved {
		msg += "✅ *расписание подтверждено*"
	} else {
		msg += "⚠️ *ожидает подтверждения*\nтурниры не будут запущены, пока не нажмёте \"всё верно\""
	}

	return msg
}

func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-3]) + "..."
}

func GetScheduleMainKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ всё верно", "schedule:approve"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✏️ редактировать", "schedule:edit"),
			tgbotapi.NewInlineKeyboardButtonData("🗑 удалить", "schedule:delete"),
		),
	)
}

func GetScheduleSelectEventKeyboard(action string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("понедельник", fmt.Sprintf("schedule:%s:monday", action)),
			tgbotapi.NewInlineKeyboardButtonData("вторник", fmt.Sprintf("schedule:%s:tuesday", action)),
			tgbotapi.NewInlineKeyboardButtonData("среда", fmt.Sprintf("schedule:%s:wednesday", action)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ назад", "schedule:back"),
		),
	)
}

func (sm *ScheduleManager) GetDeleteEventKeyboard() tgbotapi.InlineKeyboardMarkup {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var buttons []tgbotapi.InlineKeyboardButton
	for _, e := range sm.current.Events {
		label := e.Day
		if e.Deleted {
			label = "🔄 " + e.Day + " (восстановить)"
		} else {
			label = "🗑 " + e.Day
		}
		buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("schedule:delete_event:%s", e.ID)))
	}

	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(buttons...),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ назад", "schedule:back"),
		),
	)
}

func GetScheduleEditFieldKeyboard(eventID string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("лимит участников", fmt.Sprintf("schedule:field:%s:limit", eventID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("лимит lichess", fmt.Sprintf("schedule:field:%s:lichess_limit", eventID)),
			tgbotapi.NewInlineKeyboardButtonData("лимит chesscom", fmt.Sprintf("schedule:field:%s:chesscom_limit", eventID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("текст объявления", fmt.Sprintf("schedule:field:%s:intro", eventID)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ назад", "schedule:back"),
		),
	)
}

func GetScheduleBackKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("⬅️ назад", "schedule:back"),
		),
	)
}
