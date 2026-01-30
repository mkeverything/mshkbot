package bot

import (
	"sync"
	"time"
)

type AdminProcessType string

const (
	ProcessTypeSuspension     AdminProcessType = "suspension"
	ProcessTypeBan            AdminProcessType = "ban"
	ProcessTypeUnban          AdminProcessType = "unban"
	ProcessTypeAdmitToGreen   AdminProcessType = "admit_to_green"
	ProcessTypeAllowToGreen   AdminProcessType = "allow_to_green"
	ProcessTypeCreateEvent    AdminProcessType = "create_event"
	ProcessTypeStartCustom    AdminProcessType = "start_custom"
	ProcessTypePlanTournament AdminProcessType = "plan_tournament"
)

type AdminProcess struct {
	Type                AdminProcessType
	AdminID             int64
	Duration            string
	CreatedAt           time.Time
	EventConfig         *EventConfig
	CustomConfig        *EventConfig
	PlanTournamentState *PlanTournamentState
	CurrentStep         string
}

type PlanTournamentState struct {
	Tournament *PlannedTournamentConfig
}

type PlannedTournamentConfig struct {
	ID            string
	Name          string
	Date          string
	StartTime     string
	EndDate       string
	EndTime       string
	Limit         int
	LichessLimit  int
	ChesscomLimit int
	Intro         string
}

type EventConfig struct {
	Limit         int
	LichessLimit  int
	ChesscomLimit int
	Intro         string
}

type AdminProcessStore struct {
	processes map[int64]*AdminProcess
	mu        sync.RWMutex
}

func NewAdminProcessStore() *AdminProcessStore {
	return &AdminProcessStore{
		processes: make(map[int64]*AdminProcess),
	}
}

func (s *AdminProcessStore) Set(adminID int64, processType AdminProcessType, duration string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processes[adminID] = &AdminProcess{
		Type:      processType,
		AdminID:   adminID,
		Duration:  duration,
		CreatedAt: time.Now(),
	}
}

func (s *AdminProcessStore) SetWithConfig(adminID int64, processType AdminProcessType, step string, config *EventConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processes[adminID] = &AdminProcess{
		Type:        processType,
		AdminID:     adminID,
		CreatedAt:   time.Now(),
		CurrentStep: step,
	}

	if processType == ProcessTypeCreateEvent {
		s.processes[adminID].EventConfig = config
	} else if processType == ProcessTypeStartCustom {
		s.processes[adminID].CustomConfig = config
	}
}

func (s *AdminProcessStore) SetPlanTournamentState(adminID int64, step string, state *PlanTournamentState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processes[adminID] = &AdminProcess{
		Type:                ProcessTypePlanTournament,
		AdminID:             adminID,
		CreatedAt:           time.Now(),
		CurrentStep:         step,
		PlanTournamentState: state,
	}
}

func (s *AdminProcessStore) Get(adminID int64) (*AdminProcess, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, exists := s.processes[adminID]
	return p, exists
}

func (s *AdminProcessStore) UpdateStep(adminID int64, step string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, exists := s.processes[adminID]; exists {
		p.CurrentStep = step
	}
}

func (s *AdminProcessStore) Clear(adminID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.processes, adminID)
}

func (s *AdminProcessStore) CleanupExpired(maxAge time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for id, p := range s.processes {
		if now.Sub(p.CreatedAt) > maxAge {
			delete(s.processes, id)
		}
	}
}
