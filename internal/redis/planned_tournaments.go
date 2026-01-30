package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sukalov/mshkbot/internal/types"
)

const plannedTournamentsKey = "planned_tournaments"

func GetPlannedTournaments(ctx context.Context) ([]types.PlannedTournament, error) {
	data, err := Client.Get(ctx, plannedTournamentsKey).Bytes()
	if err != nil {
		if err.Error() == "redis: nil" {
			return []types.PlannedTournament{}, nil
		}
		return nil, fmt.Errorf("failed to get planned tournaments from redis: %w", err)
	}

	var tournaments []types.PlannedTournament
	if err := json.Unmarshal(data, &tournaments); err != nil {
		return nil, fmt.Errorf("failed to unmarshal planned tournaments: %w", err)
	}

	return tournaments, nil
}

func SavePlannedTournament(ctx context.Context, tournament types.PlannedTournament) error {
	tournaments, err := GetPlannedTournaments(ctx)
	if err != nil {
		return err
	}

	// Check if tournament already exists (update case)
	found := false
	for i, t := range tournaments {
		if t.ID == tournament.ID {
			tournaments[i] = tournament
			found = true
			break
		}
	}

	if !found {
		tournaments = append(tournaments, tournament)
	}

	data, err := json.Marshal(tournaments)
	if err != nil {
		return fmt.Errorf("failed to marshal planned tournaments: %w", err)
	}

	if err := Client.Set(ctx, plannedTournamentsKey, data, 0).Err(); err != nil {
		return fmt.Errorf("failed to save planned tournaments to redis: %w", err)
	}

	return nil
}

func DeletePlannedTournament(ctx context.Context, id string) error {
	tournaments, err := GetPlannedTournaments(ctx)
	if err != nil {
		return err
	}

	var filtered []types.PlannedTournament
	for _, t := range tournaments {
		if t.ID != id {
			filtered = append(filtered, t)
		}
	}

	data, err := json.Marshal(filtered)
	if err != nil {
		return fmt.Errorf("failed to marshal planned tournaments: %w", err)
	}

	if err := Client.Set(ctx, plannedTournamentsKey, data, 0).Err(); err != nil {
		return fmt.Errorf("failed to save planned tournaments to redis: %w", err)
	}

	return nil
}

func GetPlannedTournamentByID(ctx context.Context, id string) (*types.PlannedTournament, error) {
	tournaments, err := GetPlannedTournaments(ctx)
	if err != nil {
		return nil, err
	}

	for _, t := range tournaments {
		if t.ID == id {
			return &t, nil
		}
	}

	return nil, fmt.Errorf("planned tournament not found: %s", id)
}

func GetTournamentsToStart(ctx context.Context) ([]types.PlannedTournament, error) {
	tournaments, err := GetPlannedTournaments(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	var toStart []types.PlannedTournament

	for _, t := range tournaments {
		if t.Status == types.StatusPlanned && !t.StartTime.After(now) {
			toStart = append(toStart, t)
		}
	}

	return toStart, nil
}

func HasTimeConflict(ctx context.Context, startTime, endTime time.Time, excludeID string) (bool, error) {
	tournaments, err := GetPlannedTournaments(ctx)
	if err != nil {
		return false, err
	}

	for _, t := range tournaments {
		if t.ID == excludeID {
			continue
		}

		// Skip completed or cancelled tournaments
		if t.Status == types.StatusCompleted || t.Status == types.StatusCancelled {
			continue
		}

		// Check for overlap
		if startTime.Before(t.EndTime) && endTime.After(t.StartTime) {
			return true, nil
		}
	}

	return false, nil
}
