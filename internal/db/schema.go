// schema.go
package db

import (
	"time"

	"gorm.io/gorm"
)

// User represents a telegram user
type User struct {
	ChatID        int64      `gorm:"primaryKey;column:chat_id"`
	Username      string     `gorm:"column:username;index"`
	TgName        string     `gorm:"column:tg_name"`
	SavedName     string     `gorm:"column:saved_name"`
	Lichess       *string    `gorm:"column:lichess;unique"`
	ChessCom      *string    `gorm:"column:chesscom;unique"`
	BannedUntil   *time.Time `gorm:"column:banned_until"`
	NotGreenUntil *time.Time `gorm:"column:not_green_until"`
	AllowToGreen  bool       `gorm:"column:allow_to_green;default:false"`
	TimesPlayed   int        `gorm:"column:times_played;default:0"`
	State         State      `gorm:"column:state"`
	AddedAt       time.Time  `gorm:"column:added_at;autoCreateTime"`
}

type State string

const (
	StateCompleted        State = "completed"
	StateAskedLichess     State = "lichess_username_asked"
	StateAskedChessCom    State = "chesscomusername_asked"
	StateAskedSavedName   State = "asked_saved_name"
	StateEditingSavedName State = "editing_saved_name"
	StateEditingLichess   State = "editing_lichess"
	StateEditingChessCom  State = "editing_chesscom"
)

// TableName specifies the table name for User model
func (User) TableName() string {
	return "users"
}

// BeforeCreate hook - runs before creating a new user
func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.AddedAt.IsZero() {
		u.AddedAt = time.Now().UTC()
	}
	return nil
}

// add more models below as your project grows
// example:
// type Message struct {
//     ID        uint      `gorm:"primaryKey"`
//     UserID    int64     `gorm:"index;not null"`
//     Content   string    `gorm:"type:text"`
//     CreatedAt time.Time `gorm:"autoCreateTime"`
//     User      User      `gorm:"foreignKey:UserID;references:ChatID"`
// }
