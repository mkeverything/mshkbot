package maingroup

import (
	"context"
	"fmt"
	"log"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sukalov/mshkbot/internal/bot"
	"github.com/sukalov/mshkbot/internal/db"
	"github.com/sukalov/mshkbot/internal/logger"
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
	userInfo := logger.ExtractUserInfoFromUpdate(&update, "main group")
	b.Logger.LogInfo("Check-in initiated", userInfo, "User initiated check-in to tournament")

	user, err := db.GetUser(update.Message.From.ID)
	if err != nil {
		if err.Error() == "user not found" {
			b.Logger.LogInfo("Check-in failed - user not registered", userInfo, "User attempted check-in but is not registered")
			return b.ReplyToMessage(update.Message.Chat.ID, update.Message.MessageID, "напишите мне в личку чтобы зарегистрироваться")
		}
		errorContext := utils.CreateErrorContext(&update, "get_user_for_checkin")
		utils.LogError(errorContext, err, "failed to get user for checkin")
		b.Logger.LogError("Check-in failed", userInfo, "Failed to get user data for check-in", err)
		return b.SendMessage(update.Message.From.ID, utils.FormatUserErrorMessage(errorContext.TraceID, "ошибка при получении данных пользователя"))
	}
	if user.State != db.StateCompleted {
		b.Logger.LogInfo("Check-in failed - registration incomplete", userInfo, "User attempted check-in but registration not completed")
		return b.ReplyToMessage(update.Message.Chat.ID, update.Message.MessageID, "мы с вами в личке ещё не закончили регистрацию")
	}

	fullUser, err := db.GetByChatID(update.Message.From.ID)
	if err != nil {
		errorContext := utils.CreateErrorContext(&update, "get_user_for_checkin_complete")
		utils.LogError(errorContext, err, "failed to get full user data")
		b.Logger.LogError("Check-in failed", userInfo, "Failed to get full user data for check-in", err)
		return b.ReplyToMessage(update.Message.Chat.ID, update.Message.MessageID, utils.FormatUserErrorMessage(errorContext.TraceID, "ошибка при получении данных пользователя"))
	}

	_, err = checkInUser(b, fullUser, update.Message.Chat.ID, update.Message.MessageID, update.Message.Chat.ID, "main group", true)
	return err
}

func ForceCheckInUser(b *bot.Bot, update tgbotapi.Update, fullUser db.User) (bool, error) {
	if fullUser.State != db.StateCompleted {
		return false, b.ReplyToMessage(update.Message.Chat.ID, update.Message.MessageID, "мы с ним в личке ещё не закончили регистрацию")
	}

	return checkInUser(b, fullUser, update.Message.Chat.ID, update.Message.MessageID, b.GetMainGroupID(), "admin group", update.Message.ForwardFrom != nil)
}

func checkInUser(b *bot.Bot, fullUser db.User, responseChatID int64, responseMessageID int, announcementChatID int64, chatType string, reactOnSuccess bool) (bool, error) {
	ctx := context.Background()
	userID := int(fullUser.ChatID)
	userInfo := &logger.UserInfo{
		ID:        fullUser.ChatID,
		Username:  fullUser.Username,
		FirstName: fullUser.SavedName,
		ChatType:  chatType,
	}

	if !b.Tournament.Metadata.Exists {
		b.Logger.LogInfo("Check-in failed - no tournament", userInfo, "User attempted check-in but no tournament exists")
		return false, b.ReplyToMessage(responseChatID, responseMessageID, utils.CheckinUnavailibleMessage())
	}

	var existingPlayer *types.Player
	for _, player := range b.Tournament.List {
		if player.ID == userID {
			existingPlayer = &player
			break
		}
	}

	if existingPlayer != nil {
		if existingPlayer.State == types.StateCheckedOut {
			timeSinceCheckout := time.Since(existingPlayer.CheckedOutTime)

			if timeSinceCheckout < types.CheckoutCooldownDuration {
				remaining := types.CheckoutCooldownDuration - timeSinceCheckout
				b.Logger.LogInfo("Check-in blocked - cooldown not complete", userInfo, fmt.Sprintf("User attempted re-check-in with %v remaining", remaining))
				return false, b.ReplyToMessage(responseChatID, responseMessageID, fmt.Sprintf("вы уже вышли, теперь придётся подождать. осталось минут: %d", int(remaining.Minutes())+1))
			}

			if err := b.Tournament.RemovePlayer(ctx, userID); err != nil {
				b.Logger.LogError("Failed to remove old player instance", userInfo, "Failed to remove checked-out player before re-check-in", err)
			} else {
				b.Logger.LogSuccess("Old player instance removed", userInfo, "Removed checked-out player after cooldown")
			}
			existingPlayer = nil
		} else {
			b.Logger.LogInfo("Check-in failed - already checked in", userInfo, "User attempted check-in but already in tournament")
			return false, b.ReplyToMessage(responseChatID, responseMessageID, utils.AlreadyCheckedInMessage())
		}
	}

	lichessRatingLimit := b.Tournament.Metadata.LichessRatingLimit
	chesscomRatingLimit := b.Tournament.Metadata.ChesscomRatingLimit
	isGreenTournament := (lichessRatingLimit > 0 && lichessRatingLimit <= 1600) || (chesscomRatingLimit > 0 && chesscomRatingLimit <= 1400)

	if isGreenTournament {
		if fullUser.NotGreenUntil != nil && time.Now().Before(*fullUser.NotGreenUntil) {
			b.Logger.LogInfo("Check-in failed - suspended from green tournaments", userInfo, "User attempted check-in to green tournament but is suspended")
			return false, b.ReplyToMessage(responseChatID, responseMessageID, "вам нельзя в этом турнире играть")
		}
	}

	var peakRating *types.PeakRating
	manualAllowUsed := false

	if fullUser.Lichess != nil {
		lichessPeakRatings, err := utils.GetLichessAllTimeHigh(*fullUser.Lichess)
		if err != nil {
			log.Printf("failed to get lichess peak ratings for user %d: %v", userID, err)
			b.Logger.LogError("Rating fetch failed", userInfo, fmt.Sprintf("Failed to get lichess peak ratings for: %s", *fullUser.Lichess), err)
		} else {
			lichessRatingLimit := b.Tournament.Metadata.LichessRatingLimit
			if lichessRatingLimit != 0 {
				maxRating := lichessPeakRatings.Blitz
				if lichessPeakRatings.Rapid > maxRating {
					maxRating = lichessPeakRatings.Rapid
				}
				if lichessPeakRatings.Classical > maxRating {
					maxRating = lichessPeakRatings.Classical
				}
				if maxRating >= lichessRatingLimit {
					if fullUser.AllowToGreen {
						manualAllowUsed = true
						b.Logger.LogInfo("Manual allow used for green tournament", userInfo, "User exceeded rating limit but has manual allow")
					} else {
						b.Logger.LogInfo("Check-in failed - rating limit exceeded (lichess)", userInfo, fmt.Sprintf("Lichess peak rating %d exceeds limit %d", maxRating, lichessRatingLimit))
						return false, b.ReplyToMessage(responseChatID, responseMessageID, "ваш пиковый рейтинг на личесе превышает лимит турнира")
					}
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
			b.Logger.LogError("Rating fetch failed", userInfo, fmt.Sprintf("Failed to get chess.com peak ratings for: %s", *fullUser.ChessCom), err)
		} else {
			chesscomRatingLimit := b.Tournament.Metadata.ChesscomRatingLimit
			if chesscomRatingLimit != 0 {
				maxRating := chesscomPeakRatings.Blitz
				if chesscomPeakRatings.Rapid > maxRating {
					maxRating = chesscomPeakRatings.Rapid
				}
				if chesscomPeakRatings.Classical > maxRating {
					maxRating = chesscomPeakRatings.Classical
				}
				if maxRating >= chesscomRatingLimit {
					if fullUser.AllowToGreen {
						manualAllowUsed = true
						b.Logger.LogInfo("Manual allow used for green tournament", userInfo, "User exceeded rating limit but has manual allow")
					} else {
						b.Logger.LogInfo("Check-in failed - rating limit exceeded (chess.com)", userInfo, fmt.Sprintf("Chess.com peak rating %d exceeds limit %d", maxRating, chesscomRatingLimit))
						return false, b.ReplyToMessage(responseChatID, responseMessageID, "ваш пиковый рейтинг на чесскоме превышает лимит турнира")
					}
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
		CheckinMessageID: responseMessageID,
		CheckinChatID:    responseChatID,
		AllowToGreen:     manualAllowUsed,
	}

	b.Tournament.AddPlayer(ctx, newPlayer)
	log.Printf("user %d (%s) checked in to tournament", userID, fullUser.Username)

	if state == types.StateQueued {
		b.Logger.LogSuccess("User checked in (queued)", userInfo, fmt.Sprintf("User %s checked in to tournament (queued, position: %d)", fullUser.SavedName, activePlayers+1))
	} else {
		b.Logger.LogSuccess("User checked in", userInfo, fmt.Sprintf("User %s checked in to tournament (active player)", fullUser.SavedName))
	}

	if err := db.IncrementTimesPlayed(fullUser.ChatID); err != nil {
		log.Printf("failed to increment times played for user %d: %v", userID, err)
		b.Logger.LogError("Times played increment failed", userInfo, "Failed to increment times played counter", err)
	}

	if err := UpdateAnnouncementMessage(b, announcementChatID); err != nil {
		log.Printf("failed to update announcement message: %v", err)
		b.Logger.LogError("Announcement update failed", userInfo, "Failed to update tournament announcement message", err)
	}

	if state == types.StateQueued {
		return true, b.ReplyToMessage(responseChatID, responseMessageID, "места закончились, добавили вас в очередь")
	}
	emoji := utils.ApproveEmoji()
	if emoji == "🎉" {
		if adminID := b.GetAdminGroupID(); adminID != 0 {
			mention := utils.FormatPlayerMentionAdminWithMetadataMarkdownV2(newPlayer)
			text := utils.RandomCongratulation(mention)
			if err := b.SendMessageWithMarkdownV2(adminID, text, true); err != nil {
				b.Logger.LogError("Rare check-in reaction admin notify", userInfo, "Failed to send congratulations to admin group", err)
			} else {
				b.Logger.LogSuccess("Rare check-in reaction", userInfo, fmt.Sprintf("Admin group notified for player %s", fullUser.SavedName))
			}
		}
	}
	if reactOnSuccess {
		return true, b.GiveReaction(responseChatID, responseMessageID, emoji)
	}

	return true, b.SendMessage(responseChatID, fmt.Sprintf("записали %s", fullUser.SavedName))
}

func handleCheckOut(b *bot.Bot, update tgbotapi.Update) error {
	ctx := context.Background()
	userInfo := logger.ExtractUserInfoFromUpdate(&update, "main group")
	b.Logger.LogInfo("Check-out initiated", userInfo, "User initiated check-out from tournament")

	if !b.Tournament.Metadata.Exists {
		b.Logger.LogInfo("Check-out failed - no tournament", userInfo, "User attempted check-out but no tournament exists")
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
		b.Logger.LogInfo("Check-out failed - not registered", userInfo, "User attempted check-out but not registered in tournament")
		return b.ReplyToMessage(update.Message.Chat.ID, update.Message.MessageID, "вы не записаны на турнир")
	}

	if currentPlayer.State == types.StateCheckedOut {
		b.Logger.LogInfo("Check-out failed - already checked out", userInfo, "User attempted check-out but already checked out")
		return b.ReplyToMessage(update.Message.Chat.ID, update.Message.MessageID, "вы уже отписались")
	}

	wasInTournament := currentPlayer.State == types.StateInTournament

	updatedPlayer := *currentPlayer
	updatedPlayer.State = types.StateCheckedOut
	updatedPlayer.CheckedOutTime = time.Now().UTC()

	if err := b.Tournament.EditPlayer(ctx, userID, updatedPlayer); err != nil {
		errorContext := utils.CreateErrorContext(&update, "checkout_player")
		utils.LogError(errorContext, err, "failed to check out player")
		b.Logger.LogError("Check-out failed", userInfo, "Failed to update player state to checked out", err)
		return b.ReplyToMessage(update.Message.Chat.ID, update.Message.MessageID, utils.FormatUserErrorMessage(errorContext.TraceID, "ошибка при отписке"))
	}

	log.Printf("user %d checked out from tournament", userID)
	b.Logger.LogSuccess("User checked out", userInfo, fmt.Sprintf("User %s checked out from tournament", currentPlayer.SavedName))

	if err := db.DecrementTimesPlayed(update.Message.From.ID); err != nil {
		log.Printf("failed to decrement times played for user %d: %v", userID, err)
		b.Logger.LogError("Times played decrement failed", userInfo, "Failed to decrement times played counter", err)
	}

	if wasInTournament {
		if err := PromoteQueuedPlayer(b, ctx); err != nil {
			log.Printf("failed to promote queued player: %v", err)
			b.Logger.LogError("Queue promotion failed", userInfo, "Failed to promote queued player after check-out", err)
		}
	}

	if err := UpdateAnnouncementMessage(b, update.Message.Chat.ID); err != nil {
		log.Printf("failed to update announcement message: %v", err)
		b.Logger.LogError("Announcement update failed", userInfo, "Failed to update tournament announcement message", err)
	}

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

func UpdateAnnouncementMessage(b *bot.Bot, chatID int64) error {
	announcementMessageID := b.Tournament.Metadata.AnnouncementMessageID
	if announcementMessageID == 0 {
		return nil
	}

	messageIntro := b.Tournament.Metadata.AnnouncementIntro
	if messageIntro == "" {
		messageIntro = "ТУРНИР НАЧАЛСЯ!!!"
	}

	message := BuildTournamentListMessage(b, messageIntro)

	return b.EditMessage(chatID, announcementMessageID, message)
}

func PromoteQueuedPlayer(b *bot.Bot, ctx context.Context) error {
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

	// Create a user info for the promoted player for logging
	promotedUserInfo := &logger.UserInfo{
		ID:        int64(firstQueuedPlayer.ID),
		Username:  firstQueuedPlayer.Username,
		FirstName: firstQueuedPlayer.SavedName,
		ChatType:  "main group",
	}
	b.Logger.LogSuccess("Player promoted from queue", promotedUserInfo, fmt.Sprintf("Player %s promoted from queue to tournament", firstQueuedPlayer.SavedName))

	return nil
}

func BuildTournamentListMessage(b *bot.Bot, messageIntro string) string {
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
