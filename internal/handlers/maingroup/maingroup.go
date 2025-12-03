package maingroup

import (
	"context"
	"fmt"
	"log"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sukalov/mshkbot/internal/bot"
	"github.com/sukalov/mshkbot/internal/db"
	"github.com/sukalov/mshkbot/internal/types"
	"github.com/sukalov/mshkbot/internal/utils"
)

// GetHandlers returns handler set for main group
func GetHandlers() bot.HandlerSet {
	return bot.HandlerSet{
		Commands: map[string]func(b *bot.Bot, update tgbotapi.Update) error{
			"checkin":  handleCheckIn,
			"checkout": handleCheckOut,
			"help":     handleHelp,
		},
		Messages: []func(b *bot.Bot, update tgbotapi.Update) error{
			handleRegularMessage,
		},
		Callbacks: map[string]func(b *bot.Bot, update tgbotapi.Update) error{
			"action": handleAction,
		},
	}
}

func handleHelp(b *bot.Bot, update tgbotapi.Update) error {
	return b.SendMessage(update.Message.Chat.ID, "/checkin — записаться на турнир\n\n/checkout — выход из турнира")
}

func handleCheckIn(b *bot.Bot, update tgbotapi.Update) error {
	user, err := db.GetUser(update.Message.From.ID)
	if err != nil {
		if err.Error() == "user not found" {
			return b.ReplyToMessage(update.Message.Chat.ID, update.Message.MessageID, "напишите мне в личку чтобы зарегистрироваться")
		}
		return b.SendMessage(update.Message.From.ID, fmt.Sprintf("ошибка: %v. попробуйте ещё раз и если ничего не получается, напишите @sukalov", err))
	}
	if user.State != db.StateCompleted {
		return b.ReplyToMessage(update.Message.Chat.ID, update.Message.MessageID, "мы с вами в личке ещё не закончили регистрацию")
	}

	ctx := context.Background()

	if !b.Tournament.Metadata.Exists {
		return b.ReplyToMessage(update.Message.Chat.ID, update.Message.MessageID, utils.CheckinUnavailibleMessage())
	}

	userID := int(update.Message.From.ID)

	var existingPlayer *types.Player
	for _, player := range b.Tournament.List {
		if player.ID == userID {
			existingPlayer = &player
			break
		}
	}

	fullUser, err := db.GetByChatID(update.Message.From.ID)
	if err != nil {
		log.Printf("failed to get full user data: %v", err)
		return b.ReplyToMessage(update.Message.Chat.ID, update.Message.MessageID, "ошибка при получении данных пользователя")
	}

	if existingPlayer != nil {
		if existingPlayer.State == types.StateCheckedOut {
			return b.ReplyToMessage(update.Message.Chat.ID, update.Message.MessageID, "вы уже вышли, теперь придётся подождать")
		}
		return b.ReplyToMessage(update.Message.Chat.ID, update.Message.MessageID, utils.AlreadyCheckedInMessage())
	}

	lichessRatingLimit := b.Tournament.Metadata.LichessRatingLimit
	chesscomRatingLimit := b.Tournament.Metadata.ChesscomRatingLimit
	isGreenTournament := (lichessRatingLimit > 0 && lichessRatingLimit <= 1600) || (chesscomRatingLimit > 0 && chesscomRatingLimit <= 1400)

	if isGreenTournament {
		if fullUser.NotGreenUntil != nil && time.Now().Before(*fullUser.NotGreenUntil) {
			return b.ReplyToMessage(update.Message.Chat.ID, update.Message.MessageID, "вам нельзя в этом турнире играть")
		}
	}

	var peakRating *types.PeakRating

	if fullUser.Lichess != nil {
		lichessPeakRatings, err := utils.GetLichessAllTimeHigh(*fullUser.Lichess)
		if err != nil {
			log.Printf("failed to get lichess peak ratings for user %d: %v", userID, err)
		} else {
			lichessRatingLimit := b.Tournament.Metadata.LichessRatingLimit
			if lichessRatingLimit != 0 {
				if lichessPeakRatings.Blitz >= lichessRatingLimit ||
					lichessPeakRatings.Rapid >= lichessRatingLimit ||
					lichessPeakRatings.Classical >= lichessRatingLimit {
					return b.ReplyToMessage(update.Message.Chat.ID, update.Message.MessageID, "ваш пиковый рейтинг на личесе превышает лимит турнира")
				}
			}
			peakRating = &types.PeakRating{
				Site:         types.SiteLichess,
				BlitzPeak:    lichessPeakRatings.Blitz,
				SiteUsername: *fullUser.Lichess,
			}
		}
	}

	if fullUser.ChessCom != nil {
		chesscomPeakRatings, err := utils.GetChessComAllTimeHigh(*fullUser.ChessCom)
		if err != nil {
			log.Printf("failed to get chesscom peak ratings for user %d: %v", userID, err)
		} else {
			chesscomRatingLimit := b.Tournament.Metadata.ChesscomRatingLimit
			if chesscomRatingLimit != 0 {
				if chesscomPeakRatings.Blitz >= chesscomRatingLimit ||
					chesscomPeakRatings.Rapid >= chesscomRatingLimit ||
					chesscomPeakRatings.Classical >= chesscomRatingLimit {
					return b.ReplyToMessage(update.Message.Chat.ID, update.Message.MessageID, "ваш пиковый рейтинг на чесскоме превышает лимит турнира")
				}
			}
			peakRating = &types.PeakRating{
				Site:         types.SiteChesscom,
				BlitzPeak:    chesscomPeakRatings.Blitz,
				SiteUsername: *fullUser.ChessCom,
			}
		}
	}

	limit := b.Tournament.Metadata.Limit
	activePlayers := countActivePlayers(b.Tournament.List)

	var state string
	if limit > 0 && activePlayers >= limit {
		state = types.StateQueued
	} else {
		state = types.StateInTournament
	}

	newPlayer := types.Player{
		ID:               userID,
		Username:         fullUser.Username,
		SavedName:        fullUser.SavedName,
		TimeAdded:        time.Now().UTC(),
		State:            state,
		PeakRating:       peakRating,
		CheckinMessageID: update.Message.MessageID,
		CheckinChatID:    update.Message.Chat.ID,
	}

	b.Tournament.AddPlayer(ctx, newPlayer)
	log.Printf("user %d (%s) checked in to tournament", userID, fullUser.Username)

	if err := db.IncrementTimesPlayed(update.Message.From.ID); err != nil {
		log.Printf("failed to increment times played for user %d: %v", userID, err)
	}

	if err := updateAnnouncementMessage(b, update.Message.Chat.ID); err != nil {
		log.Printf("failed to update announcement message: %v", err)
	}

	if state == types.StateQueued {
		return b.ReplyToMessage(update.Message.Chat.ID, update.Message.MessageID, "места закончились, добавили вас в очередь")
	}
	return b.GiveReaction(update.Message.Chat.ID, update.Message.MessageID, utils.ApproveEmoji())
}

func handleCheckOut(b *bot.Bot, update tgbotapi.Update) error {
	ctx := context.Background()

	if !b.Tournament.Metadata.Exists {
		return b.ReplyToMessage(update.Message.Chat.ID, update.Message.MessageID, utils.NoTournamentMessage())
	}

	userID := int(update.Message.From.ID)

	var currentPlayer *types.Player
	for _, player := range b.Tournament.List {
		if player.ID == userID {
			currentPlayer = &player
			break
		}
	}

	if currentPlayer == nil {
		return b.ReplyToMessage(update.Message.Chat.ID, update.Message.MessageID, "вы не записаны на турнир")
	}

	if currentPlayer.State == types.StateCheckedOut {
		return b.ReplyToMessage(update.Message.Chat.ID, update.Message.MessageID, "вы уже отписались")
	}

	wasInTournament := currentPlayer.State == types.StateInTournament

	updatedPlayer := *currentPlayer
	updatedPlayer.State = types.StateCheckedOut
	updatedPlayer.CheckedOutTime = time.Now().UTC()

	if err := b.Tournament.EditPlayer(ctx, userID, updatedPlayer); err != nil {
		log.Printf("failed to check out player: %v", err)
		return b.ReplyToMessage(update.Message.Chat.ID, update.Message.MessageID, "ошибка при отписке")
	}

	log.Printf("user %d checked out from tournament", userID)

	if err := db.DecrementTimesPlayed(update.Message.From.ID); err != nil {
		log.Printf("failed to decrement times played for user %d: %v", userID, err)
	}

	if wasInTournament {
		if err := promoteQueuedPlayer(b, ctx); err != nil {
			log.Printf("failed to promote queued player: %v", err)
		}
	}

	if err := updateAnnouncementMessage(b, update.Message.Chat.ID); err != nil {
		log.Printf("failed to update announcement message: %v", err)
	}

	go schedulePlayerCleanup(b, userID, 15*time.Minute)

	return b.GiveReaction(update.Message.Chat.ID, update.Message.MessageID, utils.SadEmoji())
}

func handleRegularMessage(b *bot.Bot, update tgbotapi.Update) error {
	if update.Message == nil {
		return nil
	}
	log.Printf("main group message: %s", update.Message.Text)
	return nil
}

func handleAction(b *bot.Bot, update tgbotapi.Update) error {
	return b.SendMessage(update.CallbackQuery.Message.Chat.ID, "action in main group")
}

func countActivePlayers(players []types.Player) int {
	count := 0
	for _, player := range players {
		if player.State == types.StateInTournament || player.State == types.StateQueued {
			count++
		}
	}
	return count
}

func schedulePlayerCleanup(b *bot.Bot, playerID int, delay time.Duration) {
	time.Sleep(delay)

	ctx := context.Background()

	var shouldRemove bool
	for _, player := range b.Tournament.List {
		if player.ID == playerID && player.State == types.StateCheckedOut {
			shouldRemove = true
			break
		}
	}

	if shouldRemove {
		if err := b.Tournament.RemovePlayer(ctx, playerID); err != nil {
			log.Printf("failed to cleanup checked-out player %d: %v", playerID, err)
			return
		}

		log.Printf("cleaned up checked-out player %d after %v", playerID, delay)
	}
}

func updateAnnouncementMessage(b *bot.Bot, chatID int64) error {
	announcementMessageID := b.Tournament.Metadata.AnnouncementMessageID
	if announcementMessageID == 0 {
		return nil
	}

	messageIntro := b.Tournament.Metadata.AnnouncementIntro
	if messageIntro == "" {
		messageIntro = "ТУРНИР НАЧАЛСЯ!!!"
	}

	message := buildTournamentListMessage(b, messageIntro)

	return b.EditMessage(chatID, announcementMessageID, message)
}

func promoteQueuedPlayer(b *bot.Bot, ctx context.Context) error {
	var firstQueuedPlayer *types.Player

	for _, player := range b.Tournament.List {
		if player.State == types.StateQueued {
			firstQueuedPlayer = &player
			break
		}
	}

	if firstQueuedPlayer == nil {
		return nil
	}

	updatedPlayer := *firstQueuedPlayer
	updatedPlayer.State = types.StateInTournament

	if err := b.Tournament.EditPlayer(ctx, firstQueuedPlayer.ID, updatedPlayer); err != nil {
		return fmt.Errorf("failed to promote player: %w", err)
	}

	log.Printf("promoted player %d (%s) from queue to tournament", firstQueuedPlayer.ID, firstQueuedPlayer.Username)
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
			message += fmt.Sprintf("%d. %s &#9816;\n", i+1, player.SavedName)
		}
	}

	return message
}
