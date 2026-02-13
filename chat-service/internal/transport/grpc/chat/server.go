package chatgrpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	chatv1 "github.com/byoverr/auth-proto/gen/go/chat/v1"
	"github.com/byoverr/chat-service/internal/transport/grpc/interceptor"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type ChatUsecase interface {
	// Unary
	ListChats(ctx context.Context, userID string, limit int32, pageToken string) ([]*chatv1.ChatSummary, string, error)
	GetChat(ctx context.Context, userID, chatID string) (*chatv1.Chat, error)
	CreateChat(ctx context.Context, userID string, req *chatv1.CreateChatRequest) (*chatv1.Chat, error)
	JoinChat(ctx context.Context, userID, chatID string) error
	LeaveChat(ctx context.Context, userID, chatID string) error
	GetHistory(ctx context.Context, userID string, req *chatv1.GetHistoryRequest) ([]*chatv1.Message, bool, string, int64, error)
	GetLastMessage(ctx context.Context, userID, chatID string) (*chatv1.Message, error)

	// Для realtime
	SendMessage(ctx context.Context, userID string, in *chatv1.SendMessage) (*chatv1.Message, error)
	MarkRead(ctx context.Context, userID string, in *chatv1.Read) (upToMsgID string, upToSeq int64, err error)

	// Нужны чтобы подписать соединение на все чаты пользователя при Connect()
	ListUserChatIDs(ctx context.Context, userID string) ([]string, error)
}

type ChatServer struct {
	chatv1.UnimplementedChatServiceServer

	uc  ChatUsecase
	hub *Hub
	log zerolog.Logger
}

func NewChatServer(uc ChatUsecase, log zerolog.Logger) *ChatServer {
	return &ChatServer{
		uc:  uc,
		hub: NewHub(log),
		log: log,
	}
}

func (s *ChatServer) ListChats(ctx context.Context, req *chatv1.ListChatsRequest) (*chatv1.ListChatsResponse, error) {
	userID, err := requireUserID(ctx)
	if err != nil {
		return nil, err
	}

	items, next, err := s.uc.ListChats(ctx, userID, req.GetLimit(), req.GetPageToken())
	if err != nil {
		return nil, mapErr(err)
	}

	return &chatv1.ListChatsResponse{
		Items:         items,
		NextPageToken: next,
	}, nil
}

func (s *ChatServer) GetChat(ctx context.Context, req *chatv1.GetChatRequest) (*chatv1.Chat, error) {
	userID, err := requireUserID(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetChatId() == "" {
		return nil, status.Error(codes.InvalidArgument, "chat_id is required")
	}

	out, err := s.uc.GetChat(ctx, userID, req.GetChatId())
	if err != nil {
		return nil, mapErr(err)
	}
	return out, nil
}

func (s *ChatServer) CreateChat(ctx context.Context, req *chatv1.CreateChatRequest) (*chatv1.Chat, error) {
	userID, err := requireUserID(ctx)
	if err != nil {
		return nil, err
	}

	out, err := s.uc.CreateChat(ctx, userID, req)
	if err != nil {
		return nil, mapErr(err)
	}
	return out, nil
}

func (s *ChatServer) JoinChat(ctx context.Context, req *chatv1.JoinChatRequest) (*emptypb.Empty, error) {
	userID, err := requireUserID(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetChatId() == "" {
		return nil, status.Error(codes.InvalidArgument, "chat_id is required")
	}

	if err := s.uc.JoinChat(ctx, userID, req.GetChatId()); err != nil {
		return nil, mapErr(err)
	}

	// если у пользователя уже есть активные Connect() — подписываем их на чат
	s.hub.SubscribeUserToChat(userID, req.GetChatId())

	return &emptypb.Empty{}, nil
}

func (s *ChatServer) LeaveChat(ctx context.Context, req *chatv1.LeaveChatRequest) (*emptypb.Empty, error) {
	userID, err := requireUserID(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetChatId() == "" {
		return nil, status.Error(codes.InvalidArgument, "chat_id is required")
	}

	if err := s.uc.LeaveChat(ctx, userID, req.GetChatId()); err != nil {
		return nil, mapErr(err)
	}

	s.hub.UnsubscribeUserFromChat(userID, req.GetChatId())
	return &emptypb.Empty{}, nil
}

func (s *ChatServer) GetHistory(ctx context.Context, req *chatv1.GetHistoryRequest) (*chatv1.GetHistoryResponse, error) {
	userID, err := requireUserID(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetChatId() == "" {
		return nil, status.Error(codes.InvalidArgument, "chat_id is required")
	}
	if req.GetLimit() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "limit must be > 0")
	}

	msgs, hasMore, nextMsgID, nextSeq, err := s.uc.GetHistory(ctx, userID, req)
	if err != nil {
		return nil, mapErr(err)
	}

	return &chatv1.GetHistoryResponse{
		Messages:            msgs,
		HasMore:             hasMore,
		NextBeforeMessageId: nextMsgID,
		NextBeforeSeq:       nextSeq,
	}, nil
}

func (s *ChatServer) GetLastMessage(ctx context.Context, req *chatv1.GetLastMessageRequest) (*chatv1.GetLastMessageResponse, error) {
	userID, err := requireUserID(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetChatId() == "" {
		return nil, status.Error(codes.InvalidArgument, "chat_id is required")
	}

	m, err := s.uc.GetLastMessage(ctx, userID, req.GetChatId())
	if err != nil {
		return nil, mapErr(err)
	}
	return &chatv1.GetLastMessageResponse{Message: m}, nil
}

func (s *ChatServer) Connect(stream chatv1.ChatService_ConnectServer) error {
	ctx := stream.Context()

	userID, err := requireUserID(ctx)
	if err != nil {
		return err
	}

	// 1) register
	conn := s.hub.Register(userID, stream, 100)
	defer s.hub.Unregister(conn.ID)

	// 2) subscribe to all user chats
	chatIDs, err := s.uc.ListUserChatIDs(ctx, userID)
	if err != nil {
		return mapErr(err)
	}
	if err := s.hub.SubscribeConnToChats(conn.ID, chatIDs); err != nil {
		return mapErr(err)
	}

	// 3) run writer + reader in goroutines so мы можем выйти, даже если один из них "завис" в Recv/Send
	errCh := make(chan error, 2)

	// writer loop (server -> client)
	go func() {
		for {
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			case <-conn.Done():
				errCh <- nil
				return
			case ev := <-conn.SendCh():
				if err := stream.Send(ev); err != nil {
					// обычно это означает, что клиент отвалился
					conn.Close()
					errCh <- err
					return
				}
			}
		}
	}()

	// reader loop (client -> server)
	go func() {
		for {
			in, err := stream.Recv()
			if err != nil {
				// EOF/Cancelled считаем нормальным завершением
				if errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) {
					conn.Close()
					errCh <- nil
					return
				}
				conn.Close()
				errCh <- err
				return
			}

			switch p := in.Payload.(type) {

			case *chatv1.ClientEvent_Ping:
				_ = conn.Enqueue(&chatv1.ServerEvent{
					Payload: &chatv1.ServerEvent_Pong{
						Pong: &chatv1.Pong{TsUnixMs: time.Now().UnixMilli()},
					},
				}, DropIfFull)

			case *chatv1.ClientEvent_SendMessage:
				sm := p.SendMessage
				if sm.GetChatId() == "" || sm.GetClientMsgId() == "" || sm.GetText() == "" {
					_ = conn.Enqueue(errEvent("INVALID_ARGUMENT", "chat_id, client_msg_id, text are required"), DropIfFull)
					continue
				}

				msg, err := s.uc.SendMessage(ctx, userID, sm)
				if err != nil {
					_ = conn.Enqueue(errEvent("SEND_FAILED", grpcMsg(err)), DropIfFull)
					continue
				}

				ev := &chatv1.ServerEvent{
					Payload: &chatv1.ServerEvent_MessageCreated{
						MessageCreated: &chatv1.MessageCreated{Message: msg},
					},
				}
				// важное: если клиент медленный — можно отрубать
				s.hub.BroadcastToChat(sm.GetChatId(), ev, DisconnectIfFull)

			case *chatv1.ClientEvent_Typing:
				t := p.Typing
				if t.GetChatId() == "" {
					continue
				}
				ev := &chatv1.ServerEvent{
					Payload: &chatv1.ServerEvent_TypingEvent{
						TypingEvent: &chatv1.TypingEvent{
							ChatId:   t.GetChatId(),
							UserId:   userID,
							IsTyping: t.GetIsTyping(),
							At:       nowTS(),
						},
					},
				}
				// некритичное: можно дропать
				s.hub.BroadcastToChat(t.GetChatId(), ev, DropIfFull)

			case *chatv1.ClientEvent_Read:
				r := p.Read
				if r.GetChatId() == "" {
					continue
				}
				upToMsgID, upToSeq, err := s.uc.MarkRead(ctx, userID, r)
				if err != nil {
					_ = conn.Enqueue(errEvent("READ_FAILED", grpcMsg(err)), DropIfFull)
					continue
				}

				ev := &chatv1.ServerEvent{
					Payload: &chatv1.ServerEvent_ReadEvent{
						ReadEvent: &chatv1.ReadEvent{
							ChatId:        r.GetChatId(),
							UserId:        userID,
							UpToMessageId: upToMsgID,
							UpToSeq:       upToSeq,
							At:            nowTS(),
						},
					},
				}
				s.hub.BroadcastToChat(r.GetChatId(), ev, DropIfFull)

			case *chatv1.ClientEvent_Ack:
				continue

			default:
				_ = conn.Enqueue(errEvent("UNSUPPORTED", "unsupported event"), DropIfFull)
			}
		}
	}()

	// 4) wait first finish
	if err := <-errCh; err != nil {
		// часто тут будет "rpc error: code = Canceled" при нормальном отключении клиента
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
	return nil
}

// --- helpers ---

func requireUserID(ctx context.Context) (string, error) {
	uid, err := interceptor.UserIDFromContext(ctx)
	if err != nil {
		return "", err
	}
	return uid, nil
}

func grpcMsg(err error) string {
	// если usecase уже возвращает status.Error — не замыливаем
	if st, ok := status.FromError(err); ok {
		return st.Message()
	}
	return err.Error()
}

func errEvent(code, msg string) *chatv1.ServerEvent {
	return &chatv1.ServerEvent{
		Payload: &chatv1.ServerEvent_Error{
			Error: &chatv1.ServerError{Code: code, Message: msg},
		},
	}
}

func nowTS() *timestamppb.Timestamp {
	return timestamppb.Now()
}

// mapErr: подстрой под свои доменные ошибки (ErrNotFound/ErrForbidden/ErrAlreadyExists и т.д.)
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if st, ok := status.FromError(err); ok {
		return st.Err()
	}

	// TODO: когда заведёшь chat/service ошибки — подставь сюда errors.Is(...)
	// пример:
	// if errors.Is(err, chat.ErrNotFound) { return status.Error(codes.NotFound, "not found") }

	return status.Error(codes.Internal, fmt.Sprintf("internal error: %v", err))
}
