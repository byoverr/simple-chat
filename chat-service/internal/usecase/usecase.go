package usecase

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	chatv1 "github.com/byoverr/auth-proto/gen/go/chat/v1"
	"github.com/byoverr/chat-service/internal/storage"
	"github.com/byoverr/chat-service/internal/storage/postgres"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Broker is the in-memory pub/sub broker for real-time SSE.
type Broker interface {
	Publish(chatID string, msg *chatv1.Message)
}

type Service struct {
	st     postgres.StoreInterface
	broker Broker
	log    zerolog.Logger
}

func NewService(log zerolog.Logger, st postgres.StoreInterface, broker Broker) *Service {
	return &Service{st: st, broker: broker, log: log}
}

func (s *Service) ListChats(ctx context.Context, userID string, limit int32, pageToken string) ([]*chatv1.ChatSummary, string, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	offset := 0
	if pageToken != "" {
		fmt.Sscanf(pageToken, "%d", &offset)
	}

	rows, err := s.st.Chats().ListForUser(ctx, userID, int(limit)+1, offset)
	if err != nil {
		return nil, "", err
	}

	hasMore := len(rows) > int(limit)
	if hasMore {
		rows = rows[:limit]
	}

	var nextToken string
	if hasMore {
		nextToken = fmt.Sprintf("%d", offset+int(limit))
	}

	summaries := make([]*chatv1.ChatSummary, 0, len(rows))
	for _, row := range rows {
		chat := chatRowToProto(row)

		readSeq, _ := s.st.Chats().GetReadState(ctx, row.ID, userID)
		unread, _ := s.st.Chats().GetUnreadCount(ctx, row.ID, userID, readSeq)

		var lastMsg *chatv1.MessagePreview
		if row.LastMessageID != nil {
			if msg, err := s.st.Messages().GetLastMessage(ctx, row.ID); err == nil {
				lastMsg = &chatv1.MessagePreview{
					MessageId:   msg.ID,
					SenderId:    msg.SenderID,
					TextPreview: truncate(msg.Text, 100),
					CreatedAt:   timestamppb.New(msg.CreatedAt),
					Seq:         msg.Seq,
				}
			}
		}

		summaries = append(summaries, &chatv1.ChatSummary{
			Chat:              chat,
			LastMessage:       lastMsg,
			UnreadCount:       unread,
			LastReadMessageId: fmt.Sprintf("%d", readSeq),
		})
	}

	return summaries, nextToken, nil
}

func (s *Service) GetChat(ctx context.Context, userID, chatID string) (*chatv1.Chat, error) {
	ok, err := s.st.Chats().IsMember(ctx, chatID, userID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrForbidden
	}

	row, err := s.st.Chats().GetByID(ctx, chatID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return chatRowToProto(row), nil
}

func (s *Service) CreateChat(ctx context.Context, userID string, req *chatv1.CreateChatRequest) (*chatv1.Chat, error) {
	chatType := int(req.GetType())
	if chatType == 0 {
		chatType = 1 // default direct
	}

	members := req.GetMemberUserIds()
	if len(members) == 0 {
		members = []string{userID}
	}
	// ensure creator is always in the list
	hasCreator := false
	for _, m := range members {
		if m == userID {
			hasCreator = true
			break
		}
	}
	if !hasCreator {
		members = append(members, userID)
	}

	var directKey *string
	if chatType == 1 && len(members) == 2 {
		// sort to make key stable
		sorted := make([]string, len(members))
		copy(sorted, members)
		sort.Strings(sorted)
		dk := strings.Join(sorted, ":")
		directKey = &dk

		// check if direct chat already exists
		if existing, err := s.st.Chats().GetDirect(ctx, dk); err == nil {
			return chatRowToProto(existing), nil
		}
	}

	title := strings.TrimSpace(req.GetTitle())
	if chatType != 1 && title == "" {
		return nil, ErrInvalidArgument
	}

	row, err := s.st.Chats().Create(ctx, postgres.CreateChat{
		ID:        uuid.NewString(),
		Type:      chatType,
		Title:     title,
		CreatedBy: userID,
		DirectKey: directKey,
		Members:   members,
	})
	if err != nil {
		if errors.Is(err, storage.ErrAlreadyExists) {
			return nil, ErrAlreadyExists
		}
		return nil, err
	}

	return chatRowToProto(row), nil
}

func (s *Service) JoinChat(ctx context.Context, userID, chatID string) error {
	_, err := s.st.Chats().GetByID(ctx, chatID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	return s.st.Chats().AddMember(ctx, chatID, userID, "member")
}

func (s *Service) LeaveChat(ctx context.Context, userID, chatID string) error {
	err := s.st.Chats().RemoveMember(ctx, chatID, userID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (s *Service) GetHistory(ctx context.Context, userID string, req *chatv1.GetHistoryRequest) ([]*chatv1.Message, bool, string, int64, error) {
	chatID := req.GetChatId()

	ok, err := s.st.Chats().IsMember(ctx, chatID, userID)
	if err != nil {
		return nil, false, "", 0, err
	}
	if !ok {
		return nil, false, "", 0, ErrForbidden
	}

	limit := int(req.GetLimit())
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	var beforeMsgID string
	var beforeSeq int64

	switch c := req.GetCursor().(type) {
	case *chatv1.GetHistoryRequest_BeforeMessageId:
		beforeMsgID = c.BeforeMessageId
	case *chatv1.GetHistoryRequest_BeforeSeq:
		beforeSeq = c.BeforeSeq
	}

	rows, err := s.st.Messages().GetHistory(ctx, chatID, beforeMsgID, beforeSeq, limit+1)
	if err != nil {
		return nil, false, "", 0, err
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[1:] // drop oldest
	}

	var nextMsgID string
	var nextSeq int64
	if hasMore && len(rows) > 0 {
		nextMsgID = rows[0].ID
		nextSeq = rows[0].Seq
	}

	msgs := make([]*chatv1.Message, len(rows))
	for i, r := range rows {
		msgs[i] = msgRowToProto(r)
	}
	return msgs, hasMore, nextMsgID, nextSeq, nil
}

func (s *Service) GetLastMessage(ctx context.Context, userID, chatID string) (*chatv1.Message, error) {
	ok, err := s.st.Chats().IsMember(ctx, chatID, userID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrForbidden
	}

	row, err := s.st.Messages().GetLastMessage(ctx, chatID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return msgRowToProto(row), nil
}

func (s *Service) SendMessage(ctx context.Context, userID string, in *chatv1.SendMessage) (*chatv1.Message, error) {
	chatID := in.GetChatId()

	ok, err := s.st.Chats().IsMember(ctx, chatID, userID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrForbidden
	}

	clientMsgID := in.GetClientMsgId()
	if clientMsgID == "" {
		clientMsgID = uuid.NewString()
	}

	var replyTo *string
	if r := in.GetReplyToMessageId(); r != "" {
		replyTo = &r
	}

	row, err := s.st.Messages().Create(ctx, postgres.CreateMessage{
		ID:           uuid.NewString(),
		ChatID:       chatID,
		SenderID:     userID,
		ClientMsgID:  clientMsgID,
		Text:         in.GetText(),
		ReplyToMsgID: replyTo,
	})
	if err != nil {
		return nil, err
	}

	// Update chat's last message
	_ = s.st.Chats().UpdateLastMessage(ctx, chatID, row.ID, row.Seq, row.CreatedAt)

	msg := msgRowToProto(row)

	// Publish for SSE subscribers
	s.broker.Publish(chatID, msg)

	return msg, nil
}

func (s *Service) MarkRead(ctx context.Context, userID string, in *chatv1.Read) (string, int64, error) {
	chatID := in.GetChatId()

	var targetSeq int64
	switch u := in.GetUpTo().(type) {
	case *chatv1.Read_MessageId:
		if msg, err := s.st.Messages().GetByID(ctx, u.MessageId); err == nil {
			targetSeq = msg.Seq
		}
	case *chatv1.Read_Seq:
		targetSeq = u.Seq
	}

	if targetSeq <= 0 {
		return "", 0, nil
	}

	if err := s.st.Chats().UpsertReadState(ctx, chatID, userID, targetSeq); err != nil {
		return "", 0, err
	}

	return "", targetSeq, nil
}

func (s *Service) ListUserChatIDs(ctx context.Context, userID string) ([]string, error) {
	return s.st.Chats().ListUserChatIDs(ctx, userID)
}

// --- converters ---

func chatRowToProto(row postgres.ChatRow) *chatv1.Chat {
	var avatar string
	if row.AvatarURL != nil {
		avatar = *row.AvatarURL
	}
	return &chatv1.Chat{
		Id:           row.ID,
		Type:         chatv1.ChatType(row.Type),
		Title:        row.Title,
		AvatarUrl:    avatar,
		CreatedBy:    row.CreatedBy,
		CreatedAt:    timestamppb.New(row.CreatedAt),
		MembersCount: row.MembersCount,
	}
}

func msgRowToProto(row postgres.MessageRow) *chatv1.Message {
	msg := &chatv1.Message{
		Id:          row.ID,
		ChatId:      row.ChatID,
		SenderId:    row.SenderID,
		ClientMsgId: row.ClientMsgID,
		Text:        row.Text,
		Seq:         row.Seq,
		CreatedAt:   timestamppb.New(row.CreatedAt),
	}
	if row.EditedAt != nil {
		msg.EditedAt = timestamppb.New(*row.EditedAt)
	}
	if row.ReplyToMsgID != nil {
		msg.ReplyToMessageId = *row.ReplyToMsgID
	}
	return msg
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}
