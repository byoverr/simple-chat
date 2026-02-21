package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/byoverr/chat-service/internal/storage"
)

type ChatsRepo interface {
	Create(ctx context.Context, in CreateChat) (ChatRow, error)
	GetByID(ctx context.Context, chatID string) (ChatRow, error)
	GetDirect(ctx context.Context, directKey string) (ChatRow, error)
	ListForUser(ctx context.Context, userID string, limit int, offset int) ([]ChatRow, error)
	UpdateLastMessage(ctx context.Context, chatID string, msgID string, seq int64, at time.Time) error

	AddMember(ctx context.Context, chatID, userID, role string) error
	RemoveMember(ctx context.Context, chatID, userID string) error
	IsMember(ctx context.Context, chatID, userID string) (bool, error)
	ListUserChatIDs(ctx context.Context, userID string) ([]string, error)

	UpsertReadState(ctx context.Context, chatID, userID string, lastReadSeq int64) error
	GetReadState(ctx context.Context, chatID, userID string) (int64, error)
	GetUnreadCount(ctx context.Context, chatID, userID string, lastReadSeq int64) (int64, error)
}

type CreateChat struct {
	ID        string
	Type      int
	Title     string
	CreatedBy string
	DirectKey *string
	Members   []string
}

type chatsRepo struct {
	db *gorm.DB
}

func (r *chatsRepo) Create(ctx context.Context, in CreateChat) (ChatRow, error) {
	id := in.ID
	if id == "" {
		id = uuid.NewString()
	}

	row := ChatRow{
		ID:        id,
		Type:      in.Type,
		Title:     in.Title,
		CreatedBy: in.CreatedBy,
		DirectKey: in.DirectKey,
		CreatedAt: time.Now().UTC(),
	}

	err := r.db.WithContext(ctx).Create(&row).Error
	if err != nil {
		if isPgUniqueViolation(err) {
			return ChatRow{}, storage.ErrAlreadyExists
		}
		return ChatRow{}, err
	}

	now := time.Now().UTC()
	for _, uid := range in.Members {
		m := MemberRow{
			ChatID:   id,
			UserID:   uid,
			Role:     "member",
			JoinedAt: now,
		}
		if uid == in.CreatedBy {
			m.Role = "admin"
		}
		if err := r.db.WithContext(ctx).Create(&m).Error; err != nil {
			if !isPgUniqueViolation(err) {
				return ChatRow{}, err
			}
		}
	}

	var count int64
	r.db.WithContext(ctx).Model(&MemberRow{}).Where("chat_id = ? AND left_at IS NULL", id).Count(&count)
	r.db.WithContext(ctx).Model(&ChatRow{}).Where("id = ?", id).Update("members_count", count)
	row.MembersCount = count

	return row, nil
}

func (r *chatsRepo) GetByID(ctx context.Context, chatID string) (ChatRow, error) {
	var row ChatRow
	err := r.db.WithContext(ctx).Where("id = ?", chatID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ChatRow{}, storage.ErrNotFound
		}
		return ChatRow{}, err
	}
	return row, nil
}

func (r *chatsRepo) GetDirect(ctx context.Context, directKey string) (ChatRow, error) {
	var row ChatRow
	err := r.db.WithContext(ctx).Where("direct_key = ?", directKey).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ChatRow{}, storage.ErrNotFound
		}
		return ChatRow{}, err
	}
	return row, nil
}

func (r *chatsRepo) ListForUser(ctx context.Context, userID string, limit, offset int) ([]ChatRow, error) {
	var rows []ChatRow
	err := r.db.WithContext(ctx).
		Joins("JOIN chat_members ON chat_members.chat_id = chats.id AND chat_members.user_id = ? AND chat_members.left_at IS NULL", userID).
		Order("chats.last_message_at DESC NULLS LAST, chats.created_at DESC").
		Limit(limit).Offset(offset).
		Find(&rows).Error
	return rows, err
}

func (r *chatsRepo) UpdateLastMessage(ctx context.Context, chatID, msgID string, seq int64, at time.Time) error {
	return r.db.WithContext(ctx).Model(&ChatRow{}).Where("id = ?", chatID).Updates(map[string]any{
		"last_message_id":  msgID,
		"last_message_seq": seq,
		"last_message_at":  at,
	}).Error
}

func (r *chatsRepo) AddMember(ctx context.Context, chatID, userID, role string) error {
	m := MemberRow{
		ChatID:   chatID,
		UserID:   userID,
		Role:     role,
		JoinedAt: time.Now().UTC(),
	}
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "chat_id"}, {Name: "user_id"}},
			DoUpdates: clause.Assignments(map[string]any{"left_at": nil, "joined_at": time.Now().UTC()}),
		}).
		Create(&m).Error
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Exec(
		"UPDATE chats SET members_count = (SELECT COUNT(*) FROM chat_members WHERE chat_id = ? AND left_at IS NULL) WHERE id = ?",
		chatID, chatID).Error
}

func (r *chatsRepo) RemoveMember(ctx context.Context, chatID, userID string) error {
	now := time.Now().UTC()
	res := r.db.WithContext(ctx).Model(&MemberRow{}).
		Where("chat_id = ? AND user_id = ? AND left_at IS NULL", chatID, userID).
		Update("left_at", now)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return storage.ErrNotFound
	}
	return r.db.WithContext(ctx).Exec(
		"UPDATE chats SET members_count = (SELECT COUNT(*) FROM chat_members WHERE chat_id = ? AND left_at IS NULL) WHERE id = ?",
		chatID, chatID).Error
}

func (r *chatsRepo) IsMember(ctx context.Context, chatID, userID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&MemberRow{}).
		Where("chat_id = ? AND user_id = ? AND left_at IS NULL", chatID, userID).
		Count(&count).Error
	return count > 0, err
}

func (r *chatsRepo) ListUserChatIDs(ctx context.Context, userID string) ([]string, error) {
	var ids []string
	err := r.db.WithContext(ctx).
		Model(&MemberRow{}).
		Select("chat_id").
		Where("user_id = ? AND left_at IS NULL", userID).
		Pluck("chat_id", &ids).Error
	return ids, err
}

func (r *chatsRepo) UpsertReadState(ctx context.Context, chatID, userID string, lastReadSeq int64) error {
	return r.db.WithContext(ctx).Exec(`
		INSERT INTO chat_state (chat_id, user_id, last_read_seq, updated_at)
		VALUES (?, ?, ?, NOW())
		ON CONFLICT (chat_id, user_id) DO UPDATE
		SET last_read_seq = GREATEST(chat_state.last_read_seq, EXCLUDED.last_read_seq),
		    updated_at = NOW()
	`, chatID, userID, lastReadSeq).Error
}

func (r *chatsRepo) GetReadState(ctx context.Context, chatID, userID string) (int64, error) {
	var row ChatStateRow
	err := r.db.WithContext(ctx).
		Where("chat_id = ? AND user_id = ?", chatID, userID).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return row.LastReadSeq, nil
}

func (r *chatsRepo) GetUnreadCount(ctx context.Context, chatID, userID string, lastReadSeq int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&MessageRow{}).
		Where("chat_id = ? AND seq > ?", chatID, lastReadSeq).
		Count(&count).Error
	return count, err
}

func isPgUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
