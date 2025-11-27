package bot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sukalov/mshkbot/internal/tournament"
)

// reactionType represents a reaction type for telegram API
type reactionType struct {
	Type  string `json:"type"`
	Emoji string `json:"emoji,omitempty"`
}

type setMessageReactionRequest struct {
	ChatID    int64          `json:"chat_id"`
	MessageID int            `json:"message_id"`
	Reaction  []reactionType `json:"reaction,omitempty"`
	IsBig     bool           `json:"is_big,omitempty"`
}

type Bot struct {
	Client         *tgbotapi.BotAPI
	updateChan     tgbotapi.UpdatesChannel
	stopChan       chan struct{}
	name           string
	mu             sync.Mutex
	mainGroupID    int64
	adminGroupID   int64
	adminUserIDs   map[int64]bool
	adminMu        sync.RWMutex
	Tournament     *tournament.TournamentManager
	adminProcesses *AdminProcessStore
}

// creates a new bot instance
func New(name, token string, mainGroupID, adminGroupID int64) (*Bot, error) {
	botClient, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}

	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60
	updateChan := botClient.GetUpdatesChan(updateConfig)

	return &Bot{
		Client:         botClient,
		updateChan:     updateChan,
		stopChan:       make(chan struct{}),
		name:           name,
		mainGroupID:    mainGroupID,
		adminGroupID:   adminGroupID,
		adminUserIDs:   make(map[int64]bool),
		Tournament:     &tournament.TournamentManager{},
		adminProcesses: NewAdminProcessStore(),
	}, nil
}

// HandlerSet contains handlers for a specific chat type
type HandlerSet struct {
	Commands  map[string]func(b *Bot, update tgbotapi.Update) error
	Messages  []func(b *Bot, update tgbotapi.Update) error
	Callbacks map[string]func(b *Bot, update tgbotapi.Update) error
}

// begins processing updates with handlers for different chat types
func (b *Bot) Start(
	mainGroupHandlers HandlerSet,
	adminGroupHandlers HandlerSet,
	privateHandlers HandlerSet,
) {
	log.Printf("[%s] authorized on account %s", b.name, b.Client.Self.UserName)
	if err := b.Tournament.Init(); err != nil {
		log.Printf("[%s] failed to initialize tournament: %v", b.name, err)
	}
	log.Printf("[%s] tournament initialized: %v", b.name, b.Tournament)
	// fetch admin list on startup
	b.refreshAdminList()

	for {
		select {
		case update := <-b.updateChan:
			go b.routeUpdate(update, mainGroupHandlers, adminGroupHandlers, privateHandlers)
		case <-b.stopChan:
			return
		}
	}
}

func (b *Bot) GetMainGroupID() int64 {
	return b.mainGroupID
}

func (b *Bot) GetAdminGroupID() int64 {
	return b.adminGroupID
}

func (b *Bot) refreshAdminList() {
	config := tgbotapi.ChatAdministratorsConfig{
		ChatConfig: tgbotapi.ChatConfig{
			ChatID: b.adminGroupID,
		},
	}

	admins, err := b.Client.GetChatAdministrators(config)
	if err != nil {
		log.Printf("[%s] failed to get admin list: %v", b.name, err)
		return
	}

	b.adminMu.Lock()
	defer b.adminMu.Unlock()

	// clear and rebuild admin list
	b.adminUserIDs = make(map[int64]bool)
	for _, admin := range admins {
		b.adminUserIDs[admin.User.ID] = true
		log.Printf("[%s] registered admin: %d (%s)", b.name, admin.User.ID, admin.User.UserName)
	}
}

func (b *Bot) IsAdmin(userID int64) bool {
	b.adminMu.RLock()
	defer b.adminMu.RUnlock()
	return b.adminUserIDs[userID]
}

// routes updates to appropriate handler set based on chat type or user id
func (b *Bot) routeUpdate(
	update tgbotapi.Update,
	mainGroupHandlers HandlerSet,
	adminGroupHandlers HandlerSet,
	privateHandlers HandlerSet,
) {
	var chatID int64

	// determine chat id and user id from update
	if update.Message != nil {
		chatID = update.Message.Chat.ID
	} else if update.CallbackQuery != nil && update.CallbackQuery.Message != nil {
		chatID = update.CallbackQuery.Message.Chat.ID
	} else {
		log.Printf("[%s] update has no chat id", b.name)
		return
	}

	// route to appropriate handler set
	var handlers HandlerSet
	var chatType string

	log.Printf("chatid, maingroupid: %d, %d", chatID, b.mainGroupID)

	switch {
	case chatID == b.mainGroupID:
		log.Printf("[%s] main group message: %s", b.name, update.Message.Text)
		handlers = mainGroupHandlers
		chatType = "main group"
	case chatID == b.adminGroupID:
		handlers = adminGroupHandlers
		chatType = "admin group"
	case chatID > 0:
		handlers = privateHandlers
		chatType = "private"
	default:
		log.Printf("[%s] unrecognized chat id: %d", b.name, chatID)
		return
	}

	log.Printf("[%s] routing to %s handler", b.name, chatType)
	b.processUpdate(update, handlers)
}

// handles incoming updates with provided handler set
func (b *Bot) processUpdate(update tgbotapi.Update, handlers HandlerSet) error {
	// handle command updates
	if update.Message != nil && update.Message.IsCommand() {
		command := update.Message.Command()
		if handler, exists := handlers.Commands[command]; exists {
			if err := handler(b, update); err != nil {
				return b.SendMessage(update.Message.From.ID, fmt.Sprintf("ошибка при выполнении команды %s", command))
			}
			return nil
		}
		return b.SendMessage(update.Message.From.ID, fmt.Sprintf("неизвестная команда: /%s", command))
	}

	// handle callback queries
	if update.CallbackQuery != nil {
		data := update.CallbackQuery.Data
		// extract callback query identifier (before first colon if exists)
		query := data
		for i, c := range data {
			if c == ':' {
				query = data[:i]
				break
			}
		}

		if handler, exists := handlers.Callbacks[query]; exists {
			if err := handler(b, update); err != nil {
				return b.SendMessage(update.Message.From.ID, "ошибка")
			}
			return nil
		}

		log.Printf("[%s] unhandled callback: %s", b.name, query)
		// send error message only for private chats
		if update.CallbackQuery.Message != nil && update.CallbackQuery.Message.Chat.ID > 0 {
			b.SendMessage(update.CallbackQuery.Message.Chat.ID, "команда не распознана")
		}
		return nil
	}

	// run generic message handlers
	for _, handler := range handlers.Messages {
		if err := handler(b, update); err != nil {
			log.Printf("[%s] message handler error: %v", b.name, err)
		}
	}
	return nil
}

// halts the bot
func (b *Bot) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	close(b.stopChan)
}

func (b *Bot) SendMessage(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.DisableWebPagePreview = true
	_, err := b.Client.Send(msg)
	return err
}

func (b *Bot) SendMessageAndGetID(chatID int64, text string) (int, error) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.DisableWebPagePreview = true
	sentMsg, err := b.Client.Send(msg)
	if err != nil {
		return 0, err
	}
	return sentMsg.MessageID, nil
}

func (b *Bot) SendMessageWithMarkdown(chatID int64, text string, disableLinks bool) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	msg.DisableWebPagePreview = disableLinks
	_, err := b.Client.Send(msg)
	return err
}

func (b *Bot) SendMessageWithButtons(
	chatID int64,
	text string,
	keyboard tgbotapi.InlineKeyboardMarkup,
) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = keyboard
	msg.ParseMode = "Markdown"
	msg.DisableWebPagePreview = true

	_, err := b.Client.Send(msg)
	return err
}

func (b *Bot) SendMessageWithButtonsAndGetID(
	chatID int64,
	text string,
	keyboard tgbotapi.InlineKeyboardMarkup,
) (int, error) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = keyboard
	msg.ParseMode = "Markdown"
	msg.DisableWebPagePreview = true

	sentMsg, err := b.Client.Send(msg)
	if err != nil {
		return 0, err
	}
	return sentMsg.MessageID, nil
}

func (b *Bot) EditMessageWithButtons(
	chatID int64,
	messageID int,
	text string,
	keyboard tgbotapi.InlineKeyboardMarkup,
) error {
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	edit.ParseMode = "Markdown"
	edit.DisableWebPagePreview = true
	edit.ReplyMarkup = &keyboard

	_, err := b.Client.Send(edit)
	return err
}

func (b *Bot) Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	return b.Client.Request(c)
}

// removeReaction removes all reactions from a message
func (b *Bot) RemoveReaction(chatID int64, messageID int) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/setMessageReaction", b.Client.Token)

	reqBody := setMessageReactionRequest{
		ChatID:    chatID,
		MessageID: messageID,
		Reaction:  []reactionType{}, // empty array removes reactions
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		return fmt.Errorf("telegram api error: %v", result)
	}

	return nil
}

// giveReaction sends a reaction to a message using direct telegram API call
func (b *Bot) GiveReaction(chatID int64, messageID int, emoji string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/setMessageReaction", b.Client.Token)

	reqBody := setMessageReactionRequest{
		ChatID:    chatID,
		MessageID: messageID,
		Reaction: []reactionType{
			{
				Type:  "emoji",
				Emoji: emoji,
			},
		},
		IsBig: false,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		return fmt.Errorf("telegram api error: %v", result)
	}

	return nil
}

// replyToMessage sends a text message as a reply to a specific message
func (b *Bot) ReplyToMessage(chatID int64, messageID int, text string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", b.Client.Token)

	reqBody := map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
		"reply_parameters": map[string]interface{}{
			"message_id": messageID,
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		return fmt.Errorf("telegram api error: %v", result)
	}

	return nil
}

func (b *Bot) PinMessage(chatID int64, messageID int) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/pinChatMessage", b.Client.Token)

	reqBody := map[string]interface{}{
		"chat_id":    chatID,
		"message_id": messageID,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		return fmt.Errorf("telegram api error: %v", result)
	}

	return nil
}

func (b *Bot) EditMessage(chatID int64, messageID int, text string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/editMessageText", b.Client.Token)

	reqBody := map[string]interface{}{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       text,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		return fmt.Errorf("telegram api error: %v", result)
	}

	return nil
}

func (b *Bot) UnpinMessage(chatID int64, messageID int) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/unpinChatMessage", b.Client.Token)

	reqBody := map[string]interface{}{
		"chat_id":    chatID,
		"message_id": messageID,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		return fmt.Errorf("telegram api error: %v", result)
	}

	return nil
}

func (b *Bot) SetAdminProcess(adminChatID int64, processType AdminProcessType, duration string) {
	b.adminProcesses.Set(adminChatID, processType, duration)
}

func (b *Bot) GetAdminProcess(adminChatID int64) (*AdminProcess, bool) {
	return b.adminProcesses.Get(adminChatID)
}

func (b *Bot) ClearAdminProcess(adminChatID int64) {
	b.adminProcesses.Clear(adminChatID)
}
