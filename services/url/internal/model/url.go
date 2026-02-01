package model

import "time"

const (
	ContextUserIDKey = "user_id"
	ContextEmailKey  = "email"
)

type URL struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Address   string    `json:"address"`
	Status    string    `json:"status"`     // "up" or "down"
	CheckedAt time.Time `json:"checked_at"` // last checked time
}

const (
	StatusUnknown string = "unknown"
	StatusUP      string = "up"
	StatusDown    string = "down"
)
