package types

import (
	"time"
)

type PeakRating struct {
	Site         string `json:"site"`
	BlitzPeak    int    `json:"blitz_peak"`
	SiteUsername string `json:"site_username"`
}

type Player struct {
	ID               int         `json:"id"`
	Username         string      `json:"username"`
	SavedName        string      `json:"saved_name"`
	TimeAdded        time.Time   `json:"time_added"`
	State            string      `json:"state"`
	CheckedOutTime   time.Time   `json:"checked_out_time,omitempty"`
	PeakRating       *PeakRating `json:"peak_rating,omitempty"`
	CheckinMessageID int         `json:"checkin_message_id,omitempty"`
	CheckinChatID    int64       `json:"checkin_chat_id,omitempty"`
	AllowToGreen     bool        `json:"allow_to_green,omitempty"`
}

const (
	StateInTournament = "in_tournament"
	StateQueued       = "queued"
	StateCheckedOut   = "checked_out"
)

const CheckoutCooldownDuration = 30 * time.Minute

const SiteLichess = "lichess"
const SiteChesscom = "chesscom"

type TournamentMetadata struct {
	Limit                 int       `json:"limit"`
	LichessRatingLimit    int       `json:"lichess_rating_limit"`
	ChesscomRatingLimit   int       `json:"chesscom_rating_limit"`
	AnnouncementMessageID int       `json:"announcement_message_id"`
	AnnouncementIntro     string    `json:"announcement_intro"`
	EndTime               time.Time `json:"end_time,omitempty"`
	Exists                bool      `json:"exists"`
	PlannedID             string    `json:"planned_id,omitempty"`
}

type PlannedTournament struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	StartTime         time.Time `json:"start_time"`
	EndTime           time.Time `json:"end_time"`
	Limit             int       `json:"limit"`
	LichessLimit      int       `json:"lichess_limit"`
	ChesscomLimit     int       `json:"chesscom_limit"`
	Intro             string    `json:"intro"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	StartedAt         time.Time `json:"started_at,omitempty"`
	AnnouncementMsgID int       `json:"announcement_msg_id,omitempty"`
}

const (
	StatusPlanned   = "planned"
	StatusActive    = "active"
	StatusCompleted = "completed"
	StatusCancelled = "cancelled"
)
