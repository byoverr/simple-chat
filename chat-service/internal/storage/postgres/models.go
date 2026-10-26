package postgres

import "time"

// ChatRow is an exported type used by repos and usecase.
type ChatRow struct {
	ID             string     `gorm:"column:id;primaryKey;type:uuid"`
	Type           int        `gorm:"column:type"`
	Title          string     `gorm:"column:title"`
	AvatarURL      *string    `gorm:"column:avatar_url"`
	CreatedBy      string     `gorm:"column:created_by;type:uuid"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
	MembersCount   int64      `gorm:"column:members_count"`
	LastMessageID  *string    `gorm:"column:last_message_id"`
	LastMessageSeq int64      `gorm:"column:last_message_seq"`
	LastMessageAt  *time.Time `gorm:"column:last_message_at"`
	DirectKey      *string    `gorm:"column:direct_key"`
}

func (ChatRow) TableName() string { return "chats" }

// MemberRow is an exported type for chat members.
type MemberRow struct {
	ChatID   string     `gorm:"column:chat_id;type:uuid"`
	UserID   string     `gorm:"column:user_id;type:uuid"`
	Role     string     `gorm:"column:role"`
	JoinedAt time.Time  `gorm:"column:joined_at"`
	LeftAt   *time.Time `gorm:"column:left_at"`
}

func (MemberRow) TableName() string { return "chat_members" }

// MessageRow is an exported type for chat messages.
type MessageRow struct {
	ID           string     `gorm:"column:id;primaryKey;type:uuid"`
	ChatID       string     `gorm:"column:chat_id;type:uuid"`
	SenderID     string     `gorm:"column:sender_id;type:uuid"`
	ClientMsgID  string     `gorm:"column:client_msg_id;type:uuid"`
	Text         string     `gorm:"column:text"`
	Seq          int64      `gorm:"column:seq"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
	EditedAt     *time.Time `gorm:"column:edited_at"`
	ReplyToMsgID *string    `gorm:"column:reply_to_message_id"`
}

func (MessageRow) TableName() string { return "messages" }

// ChatStateRow is an exported type for read state.
type ChatStateRow struct {
	ChatID      string    `gorm:"column:chat_id;type:uuid"`
	UserID      string    `gorm:"column:user_id;type:uuid"`
	LastReadSeq int64     `gorm:"column:last_read_seq"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (ChatStateRow) TableName() string { return "chat_state" }
