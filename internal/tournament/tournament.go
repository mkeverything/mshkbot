package tournament

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/sukalov/mshkbot/internal/redis"
	"github.com/sukalov/mshkbot/internal/types"
	"github.com/sukalov/mshkbot/internal/utils"
)

type TournamentManager struct {
	mu       sync.RWMutex
	List     []types.Player
	Metadata types.TournamentMetadata
}

type ByTimeAdded []types.Player

func (a ByTimeAdded) Len() int           { return len(a) }
func (a ByTimeAdded) Less(i, j int) bool { return a[i].TimeAdded.Before(a[j].TimeAdded) }
func (a ByTimeAdded) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

func (tm *TournamentManager) Init() error {
	ctx := context.Background()
	fmt.Println("initializing tournament")
	tm.mu.Lock()
	defer tm.mu.Unlock()
	list, err := redis.GetList(ctx)
	if err != nil {
		return err
	}
	metadata, err := redis.GetMetadata(ctx)
	if err != nil {
		return err
	}
	tm.List = list
	tm.Metadata = metadata
	if !tm.Metadata.Exists && len(tm.List) > 0 {
		fmt.Println("tournament does not exist but list is not empty, clearing list")
		if err := tm.removeTournament(ctx); err != nil {
			return err
		}
	}
	fmt.Println("tournament initialized")
	return nil
}

func (tm *TournamentManager) AddPlayer(ctx context.Context, player types.Player) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.List = append(tm.List, player)
	if err := redis.SetList(ctx, tm.List); err != nil {
		utils.LogTournamentError("add_player", int64(player.ID), err)
		return err
	}
	return nil
}

func (tm *TournamentManager) CreateTournament(ctx context.Context, limit int, lichessRatingLimit int, chesscomRatingLimit int, announcementIntro string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.Metadata.Exists {
		return fmt.Errorf("tournament already exists")
	}
	tm.Metadata = types.TournamentMetadata{
		Limit:               limit,
		LichessRatingLimit:  lichessRatingLimit,
		ChesscomRatingLimit: chesscomRatingLimit,
		AnnouncementIntro:   announcementIntro,
		Exists:              true,
	}
	if err := redis.SetMetadata(ctx, tm.Metadata); err != nil {
		utils.LogTournamentError("create_tournament", 0, err)
		return err
	}
	return nil
}

func (tm *TournamentManager) RemoveTournament(ctx context.Context) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if !tm.Metadata.Exists {
		return fmt.Errorf("tournament does not exist")
	}
	tm.Metadata = types.TournamentMetadata{
		Limit:                 0,
		LichessRatingLimit:    0,
		ChesscomRatingLimit:   0,
		AnnouncementMessageID: 0,
		AnnouncementIntro:     "",
		Exists:                false,
	}
	if err := tm.clearList(ctx); err != nil {
		utils.LogTournamentError("clear_list", 0, err)
		return err
	}
	if err := redis.SetMetadata(ctx, tm.Metadata); err != nil {
		utils.LogTournamentError("save_metadata", 0, err)
		return err
	}
	return nil
}

func (tm *TournamentManager) removeTournament(ctx context.Context) error {
	tm.Metadata = types.TournamentMetadata{
		Limit:                 0,
		LichessRatingLimit:    0,
		ChesscomRatingLimit:   0,
		AnnouncementMessageID: 0,
		AnnouncementIntro:     "",
		Exists:                false,
	}
	if err := tm.clearList(ctx); err != nil {
		utils.LogTournamentError("clear_list", 0, err)
		return err
	}
	if err := redis.SetMetadata(ctx, tm.Metadata); err != nil {
		utils.LogTournamentError("save_metadata", 0, err)
		return err
	}
	return nil
}

func (tm *TournamentManager) clearList(ctx context.Context) error {
	tm.List = []types.Player{}
	if err := redis.SetList(ctx, tm.List); err != nil {
		utils.LogTournamentError("clear_list", 0, err)
	}
	return nil
}

func (tm *TournamentManager) EditPlayer(ctx context.Context, playerID int, updatedPlayer types.Player) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	for i, player := range tm.List {
		if player.ID == playerID {
			tm.List[i] = updatedPlayer
			if err := redis.SetList(ctx, tm.List); err != nil {
				utils.LogTournamentError("update_list", 0, err)
				return err
			}
			return nil
		}
	}

	return fmt.Errorf("player with ID %d not found in list", playerID)
}

func (tm *TournamentManager) RemovePlayer(ctx context.Context, playerID int) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	for i, player := range tm.List {
		if player.ID == playerID {
			tm.List = append(tm.List[:i], tm.List[i+1:]...)
			if err := redis.SetList(ctx, tm.List); err != nil {
				utils.LogTournamentError("update_list", 0, err)
				return err
			}
			return nil
		}
	}

	return fmt.Errorf("player with ID %d not found in list", playerID)
}

func (tm *TournamentManager) Sync(ctx context.Context) error {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	if err := redis.SetList(ctx, tm.List); err != nil {
		utils.LogTournamentError("sync_list", 0, err)
		return err
	}
	if err := redis.SetMetadata(ctx, tm.Metadata); err != nil {
		utils.LogTournamentError("sync_metadata", 0, err)
		return err
	}
	return nil
}

func (tm *TournamentManager) GetTournamentJSON() (string, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	data := struct {
		List     []types.Player           `json:"players"`
		Metadata types.TournamentMetadata `json:"metadata"`
	}{
		List:     tm.List,
		Metadata: tm.Metadata,
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal tournament state: %w", err)
	}

	return string(jsonData), nil
}

func (tm *TournamentManager) SetLimit(ctx context.Context, limit int) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.Metadata.Limit = limit
	if err := redis.SetMetadata(ctx, tm.Metadata); err != nil {
		utils.LogTournamentError("update_metadata", 0, err)
		return err
	}
	return nil
}

func (tm *TournamentManager) SetLichessRatingLimit(ctx context.Context, ratingLimit int) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.Metadata.LichessRatingLimit = ratingLimit
	if err := redis.SetMetadata(ctx, tm.Metadata); err != nil {
		utils.LogTournamentError("update_metadata", 0, err)
		return err
	}
	return nil
}

func (tm *TournamentManager) SetChesscomRatingLimit(ctx context.Context, ratingLimit int) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.Metadata.ChesscomRatingLimit = ratingLimit
	if err := redis.SetMetadata(ctx, tm.Metadata); err != nil {
		utils.LogTournamentError("update_metadata", 0, err)
		return err
	}
	return nil
}

func (tm *TournamentManager) SetAnnouncementMessageID(ctx context.Context, messageID int) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.Metadata.AnnouncementMessageID = messageID
	if err := redis.SetMetadata(ctx, tm.Metadata); err != nil {
		utils.LogTournamentError("update_metadata", 0, err)
		return err
	}
	return nil
}
