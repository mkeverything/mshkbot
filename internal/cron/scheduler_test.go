package cron

import (
	"testing"
	"time"

	"github.com/sukalov/mshkbot/internal/bot"
	"github.com/sukalov/mshkbot/internal/tournament"
	"github.com/sukalov/mshkbot/internal/types"
)

func TestIsActiveTournamentOverdue(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name     string
		metadata types.TournamentMetadata
		want     bool
	}{
		{
			name: "returns false when tournament does not exist",
			metadata: types.TournamentMetadata{
				Exists:  false,
				EndTime: now.Add(-time.Minute),
			},
			want: false,
		},
		{
			name: "returns false when end time is zero",
			metadata: types.TournamentMetadata{
				Exists:  true,
				EndTime: time.Time{},
			},
			want: false,
		},
		{
			name: "returns false when end time is in the future",
			metadata: types.TournamentMetadata{
				Exists:  true,
				EndTime: now.Add(time.Minute),
			},
			want: false,
		},
		{
			name: "returns true when end time is in the past",
			metadata: types.TournamentMetadata{
				Exists:  true,
				EndTime: now.Add(-time.Minute),
			},
			want: true,
		},
		{
			name: "returns true when end time equals now",
			metadata: types.TournamentMetadata{
				Exists:  true,
				EndTime: now,
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheduler := &Scheduler{
				bot: &bot.Bot{
					Tournament: &tournament.TournamentManager{
						Metadata: tt.metadata,
					},
				},
			}

			got := scheduler.isActiveTournamentOverdue(now)
			if got != tt.want {
				t.Fatalf("isActiveTournamentOverdue() = %v, want %v", got, tt.want)
			}
		})
	}
}
