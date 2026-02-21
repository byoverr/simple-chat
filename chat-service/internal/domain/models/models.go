package models

import "time"

type ChatType int

const (
	ChatTypeDirect  ChatType = 1
	ChatTypeGroup   ChatType = 2
	ChatTypeChannel ChatType = 3
)

type Chat struct {
	ID            string
	Type          ChatType
	Title         string
	AvatarURL     *string
	CreatedBy     string
	CreatedAt     time.Time
	MembersCount  int64
	LastMessageID *string
	LastMessageAt *time.Time
	DirectKey     *string
}

type Message struct {
	ID           string
	ChatID       string
	SenderID     string
	ClientMsgID  string
	Text         string
	Seq          int64
	CreatedAt    time.Time
	EditedAt     *time.Time
	ReplyToMsgID *string
}

type Member struct {
	ChatID   string
	UserID   string
	Role     string
	JoinedAt time.Time
	LeftAt   *time.Time
}

type ChatState struct {
	ChatID      string
	UserID      string
	LastReadSeq int64
}
