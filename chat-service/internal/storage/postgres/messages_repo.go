package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/byoverr/chat-service/internal/storage"
)

type MessagesRepo interface {
	Create(ctx context.Context, in CreateMessage) (MessageRow, error)
	GetByID(ctx context.Context, msgID string) (MessageRow, error)
	GetHistory(ctx context.Context, chatID string, beforeMsgID string, beforeSeq int64, limit int) ([]MessageRow, error)
	GetLastMessage(ctx context.Context, chatID string) (MessageRow, error)
}

type CreateMessage struct {
	ID           string
	ChatID       string
	SenderID     string
	ClientMsgID  string
	Text         string
	ReplyToMsgID *string
}

type messagesRepo struct {
	db *gorm.DB
}

func (r *messagesRepo) nextSeq(ctx context.Context, chatID string) (int64, error) {
	var maxSeq *int64
	err := r.db.WithContext(ctx).Model(&MessageRow{}).
		Where("chat_id = ?", chatID).
		Select("MAX(seq)").
		Scan(&maxSeq).Error
	if err != nil {
		return 0, err
	}
	if maxSeq == nil {
		return 1, nil
	}
	return *maxSeq + 1, nil
}

func (r *messagesRepo) Create(ctx context.Context, in CreateMessage) (MessageRow, error) {
	id := in.ID
	if id == "" {
		id = uuid.NewString()
	}

	seq, err := r.nextSeq(ctx, in.ChatID)
	if err != nil {
		return MessageRow{}, err
	}

	row := MessageRow{
		ID:           id,
		ChatID:       in.ChatID,
		SenderID:     in.SenderID,
		ClientMsgID:  in.ClientMsgID,
		Text:         in.Text,
		Seq:          seq,
		CreatedAt:    time.Now().UTC(),
		ReplyToMsgID: in.ReplyToMsgID,
	}

	err = r.db.WithContext(ctx).Create(&row).Error
	if err != nil {
		if isPgUniqueViolation(err) {
			// idempotent: return existing message
			var existing MessageRow
			if e2 := r.db.WithContext(ctx).
				Where("chat_id = ? AND client_msg_id = ?", in.ChatID, in.ClientMsgID).
				First(&existing).Error; e2 == nil {
				return existing, nil
			}
			return MessageRow{}, storage.ErrAlreadyExists
		}
		return MessageRow{}, err
	}

	return row, nil
}

func (r *messagesRepo) GetByID(ctx context.Context, msgID string) (MessageRow, error) {
	var row MessageRow
	err := r.db.WithContext(ctx).Where("id = ?", msgID).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return MessageRow{}, storage.ErrNotFound
		}
		return MessageRow{}, err
	}
	return row, nil
}

func (r *messagesRepo) GetHistory(ctx context.Context, chatID string, beforeMsgID string, beforeSeq int64, limit int) ([]MessageRow, error) {
	q := r.db.WithContext(ctx).Where("chat_id = ?", chatID)

	if beforeMsgID != "" {
		var ref MessageRow
		if err := r.db.WithContext(ctx).Select("seq").Where("id = ?", beforeMsgID).First(&ref).Error; err == nil {
			q = q.Where("seq < ?", ref.Seq)
		}
	} else if beforeSeq > 0 {
		q = q.Where("seq < ?", beforeSeq)
	}

	var rows []MessageRow
	err := q.Order("seq DESC").Limit(limit).Find(&rows).Error
	if err != nil {
		return nil, err
	}

	// reverse to chronological order
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	return rows, nil
}

func (r *messagesRepo) GetLastMessage(ctx context.Context, chatID string) (MessageRow, error) {
	var row MessageRow
	err := r.db.WithContext(ctx).
		Where("chat_id = ?", chatID).
		Order("seq DESC").
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return MessageRow{}, storage.ErrNotFound
		}
		return MessageRow{}, err
	}
	return row, nil
}
