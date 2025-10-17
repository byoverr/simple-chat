package models

import "time"

type Event struct {
	ID        string                 `json:"id" bson:"_id"`
	Type      string                 `json:"type" bson:"type"`
	UserID    string                 `json:"user_id" bson:"user_id"`
	Payload   map[string]interface{} `json:"payload" bson:"payload"`
	CreatedAt time.Time              `json:"created_at" bson:"created_at"`
}

type Subscription struct {
	UserID      string          `json:"user_id" bson:"user_id"`
	Channels    []string        `json:"channels" bson:"channels"`
	Email       string          `json:"email" bson:"email"`
	Preferences map[string]bool `json:"preferences" bson:"preferences"`
}
