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
}

const (
	StateInTournament = "in_tournament"
	StateQueued       = "queued"
	StateCheckedOut   = "checked_out"
)

const SiteLichess = "lichess"
const SiteChesscom = "chesscom"

type TournamentMetadata struct {
	Limit                 int    `json:"limit"`
	LichessRatingLimit    int    `json:"lichess_rating_limit"`
	ChesscomRatingLimit   int    `json:"chesscom_rating_limit"`
	AnnouncementMessageID int    `json:"announcement_message_id"`
	AnnouncementIntro     string `json:"announcement_intro"`
	Exists                bool   `json:"exists"`
}
