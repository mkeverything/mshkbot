package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	redisClient "github.com/go-redis/redis/v8"
	"github.com/sukalov/mshkbot/internal/types"
)

func SetList(ctx context.Context, list []types.Player) error {
	if err := ValidateConnection(); err != nil {
		return fmt.Errorf("redis connection validation failed: %w", err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	listJSON, err := json.Marshal(list)
	if err != nil {
		return fmt.Errorf("failed to marshal player list: %w", err)
	}

	if err := Client.Set(timeoutCtx, "tournament_list", listJSON, 0).Err(); err != nil {
		return fmt.Errorf("failed to set tournament list in redis (timeout): %w", err)
	}

	return nil
}

func GetList(ctx context.Context) ([]types.Player, error) {
	if err := ValidateConnection(); err != nil {
		return nil, fmt.Errorf("redis connection validation failed: %w", err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	data, err := Client.Get(timeoutCtx, "tournament_list").Bytes()
	if err != nil {
		if err == redisClient.Nil {
			return []types.Player{}, nil
		}
		return nil, fmt.Errorf("failed to get tournament list from redis (timeout): %w", err)
	}

	var list []types.Player
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("failed to unmarshal player list: %w", err)
	}

	return list, nil
}

func SetMetadata(ctx context.Context, metadata types.TournamentMetadata) error {
	if err := ValidateConnection(); err != nil {
		return fmt.Errorf("redis connection validation failed: %w", err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal tournament metadata: %w", err)
	}

	if err := Client.Set(timeoutCtx, "tournament_metadata", metadataJSON, 0).Err(); err != nil {
		return fmt.Errorf("failed to set tournament metadata in redis (timeout): %w", err)
	}

	return nil
}

func GetMetadata(ctx context.Context) (types.TournamentMetadata, error) {
	if err := ValidateConnection(); err != nil {
		return types.TournamentMetadata{}, fmt.Errorf("redis connection validation failed: %w", err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	data, err := Client.Get(timeoutCtx, "tournament_metadata").Bytes()
	if err != nil {
		if err == redisClient.Nil {
			return types.TournamentMetadata{}, nil
		}
		return types.TournamentMetadata{}, fmt.Errorf("failed to get tournament metadata from redis (timeout): %w", err)
	}

	var metadata types.TournamentMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return types.TournamentMetadata{}, fmt.Errorf("failed to unmarshal tournament metadata: %w", err)
	}

	return metadata, nil
}
