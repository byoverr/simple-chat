package httphandler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	chatv1 "github.com/byoverr/auth-proto/gen/go/chat/v1"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog"
)

type ChatUsecase interface {
	ListChats(ctx context.Context, userID string, limit int32, pageToken string) ([]*chatv1.ChatSummary, string, error)
	GetChat(ctx context.Context, userID, chatID string) (*chatv1.Chat, error)
	CreateChat(ctx context.Context, userID string, req *chatv1.CreateChatRequest) (*chatv1.Chat, error)
	JoinChat(ctx context.Context, userID, chatID string) error
	LeaveChat(ctx context.Context, userID, chatID string) error
	GetHistory(ctx context.Context, userID string, req *chatv1.GetHistoryRequest) ([]*chatv1.Message, bool, string, int64, error)
	SendMessage(ctx context.Context, userID string, in *chatv1.SendMessage) (*chatv1.Message, error)
	MarkRead(ctx context.Context, userID string, in *chatv1.Read) (string, int64, error)
}

type Handler struct {
	uc        ChatUsecase
	broker    *SSEBroker
	jwtSecret string
	log       zerolog.Logger
}

func NewHandler(uc ChatUsecase, broker *SSEBroker, jwtSecret string, log zerolog.Logger) http.Handler {
	h := &Handler{uc: uc, broker: broker, jwtSecret: jwtSecret, log: log}

	mux := http.NewServeMux()

	// CORS middleware wrapper
	mux.HandleFunc("GET /api/chats", h.auth(h.listChats))
	mux.HandleFunc("POST /api/chats", h.auth(h.createChat))
	mux.HandleFunc("GET /api/chats/{chatID}", h.auth(h.getChat))
	mux.HandleFunc("POST /api/chats/{chatID}/join", h.auth(h.joinChat))
	mux.HandleFunc("POST /api/chats/{chatID}/leave", h.auth(h.leaveChat))
	mux.HandleFunc("GET /api/chats/{chatID}/messages", h.auth(h.getHistory))
	mux.HandleFunc("POST /api/chats/{chatID}/messages", h.auth(h.sendMessage))
	mux.HandleFunc("POST /api/chats/{chatID}/read", h.auth(h.markRead))
	mux.HandleFunc("GET /api/chats/{chatID}/events", h.auth(h.sseEvents))

	return corsMiddleware(mux)
}

// --- CORS ---

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- Auth middleware ---

func (h *Handler) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		tokStr := strings.TrimPrefix(authHeader, "Bearer ")
		if tokStr == authHeader {
			jsonError(w, "bearer token required", http.StatusUnauthorized)
			return
		}

		tok, err := jwt.Parse(tokStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return []byte(h.jwtSecret), nil
		})
		if err != nil || !tok.Valid {
			jsonError(w, "invalid token", http.StatusUnauthorized)
			return
		}

		claims, ok := tok.Claims.(jwt.MapClaims)
		if !ok {
			jsonError(w, "invalid token claims", http.StatusUnauthorized)
			return
		}
		userID, _ := claims["sub"].(string)
		if userID == "" {
			jsonError(w, "invalid token: no sub", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), "user_id", userID)
		next(w, r.WithContext(ctx))
	}
}

func userIDFromCtx(r *http.Request) string {
	v, _ := r.Context().Value("user_id").(string)
	return v
}

// --- Handlers ---

func (h *Handler) listChats(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)

	summaries, nextToken, err := h.uc.ListChats(r.Context(), userID, 30, r.URL.Query().Get("page_token"))
	if err != nil {
		h.log.Error().Err(err).Msg("listChats")
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type chatSummaryJSON struct {
		ID           string `json:"id"`
		Type         int    `json:"type"`
		Title        string `json:"title"`
		MembersCount int64  `json:"members_count"`
		UnreadCount  int64  `json:"unread_count"`
		LastMessage  *struct {
			ID        string `json:"id"`
			SenderID  string `json:"sender_id"`
			Text      string `json:"text"`
			CreatedAt string `json:"created_at"`
		} `json:"last_message,omitempty"`
	}

	result := make([]chatSummaryJSON, 0, len(summaries))
	for _, s := range summaries {
		item := chatSummaryJSON{
			ID:           s.GetChat().GetId(),
			Type:         int(s.GetChat().GetType()),
			Title:        s.GetChat().GetTitle(),
			MembersCount: s.GetChat().GetMembersCount(),
			UnreadCount:  s.GetUnreadCount(),
		}
		if lm := s.GetLastMessage(); lm != nil {
			item.LastMessage = &struct {
				ID        string `json:"id"`
				SenderID  string `json:"sender_id"`
				Text      string `json:"text"`
				CreatedAt string `json:"created_at"`
			}{
				ID:        lm.GetMessageId(),
				SenderID:  lm.GetSenderId(),
				Text:      lm.GetTextPreview(),
				CreatedAt: lm.GetCreatedAt().AsTime().Format(time.RFC3339),
			}
		}
		result = append(result, item)
	}

	jsonOK(w, map[string]any{"chats": result, "next_page_token": nextToken})
}

func (h *Handler) getChat(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	chatID := r.PathValue("chatID")

	chat, err := h.uc.GetChat(r.Context(), userID, chatID)
	if err != nil {
		httpErr(w, err)
		return
	}
	jsonOK(w, chatToJSON(chat))
}

func (h *Handler) createChat(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)

	var body struct {
		Type      int      `json:"type"`
		Title     string   `json:"title"`
		MemberIDs []string `json:"member_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	chat, err := h.uc.CreateChat(r.Context(), userID, &chatv1.CreateChatRequest{
		Type:          chatv1.ChatType(body.Type),
		Title:         body.Title,
		MemberUserIds: body.MemberIDs,
	})
	if err != nil {
		httpErr(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	jsonOK(w, chatToJSON(chat))
}

func (h *Handler) joinChat(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	chatID := r.PathValue("chatID")

	if err := h.uc.JoinChat(r.Context(), userID, chatID); err != nil {
		httpErr(w, err)
		return
	}
	jsonOK(w, map[string]string{"status": "joined"})
}

func (h *Handler) leaveChat(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	chatID := r.PathValue("chatID")

	if err := h.uc.LeaveChat(r.Context(), userID, chatID); err != nil {
		httpErr(w, err)
		return
	}
	jsonOK(w, map[string]string{"status": "left"})
}

func (h *Handler) getHistory(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	chatID := r.PathValue("chatID")

	msgs, hasMore, nextID, _, err := h.uc.GetHistory(r.Context(), userID, &chatv1.GetHistoryRequest{
		ChatId: chatID,
		Limit:  50,
	})
	if err != nil {
		httpErr(w, err)
		return
	}

	jsonOK(w, map[string]any{
		"messages":               msgsToJSON(msgs),
		"has_more":               hasMore,
		"next_before_message_id": nextID,
	})
}

func (h *Handler) sendMessage(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	chatID := r.PathValue("chatID")

	var body struct {
		Text        string `json:"text"`
		ClientMsgID string `json:"client_msg_id"`
		ReplyToID   string `json:"reply_to_message_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.Text) == "" {
		jsonError(w, "text is required", http.StatusBadRequest)
		return
	}

	msg, err := h.uc.SendMessage(r.Context(), userID, &chatv1.SendMessage{
		ChatId:           chatID,
		ClientMsgId:      body.ClientMsgID,
		Text:             body.Text,
		ReplyToMessageId: body.ReplyToID,
	})
	if err != nil {
		httpErr(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
	jsonOK(w, msgToJSON(msg))
}

func (h *Handler) markRead(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromCtx(r)
	chatID := r.PathValue("chatID")

	var body struct {
		MessageID string `json:"message_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if _, _, err := h.uc.MarkRead(r.Context(), userID, &chatv1.Read{
		ChatId: chatID,
		UpTo:   &chatv1.Read_MessageId{MessageId: body.MessageID},
	}); err != nil {
		httpErr(w, err)
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

// sseEvents streams new messages as Server-Sent Events.
func (h *Handler) sseEvents(w http.ResponseWriter, r *http.Request) {
	chatID := r.PathValue("chatID")

	flusher, ok := w.(http.Flusher)
	if !ok {
		jsonError(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	ch := h.broker.Subscribe(chatID)
	defer h.broker.Unsubscribe(chatID, ch)

	// Send initial ping
	fmt.Fprintf(w, ": ping\n\n")
	flusher.Flush()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case msg, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(msgToJSON(msg))
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// --- JSON helpers ---

func chatToJSON(c *chatv1.Chat) map[string]any {
	return map[string]any{
		"id":            c.GetId(),
		"type":          int(c.GetType()),
		"title":         c.GetTitle(),
		"avatar_url":    c.GetAvatarUrl(),
		"created_by":    c.GetCreatedBy(),
		"created_at":    c.GetCreatedAt().AsTime().Format(time.RFC3339),
		"members_count": c.GetMembersCount(),
	}
}

func msgToJSON(m *chatv1.Message) map[string]any {
	out := map[string]any{
		"id":              m.GetId(),
		"chat_id":         m.GetChatId(),
		"sender_id":       m.GetSenderId(),
		"client_msg_id":   m.GetClientMsgId(),
		"text":            m.GetText(),
		"seq":             m.GetSeq(),
		"created_at":      m.GetCreatedAt().AsTime().Format(time.RFC3339),
		"reply_to_msg_id": m.GetReplyToMessageId(),
	}
	if m.GetEditedAt() != nil {
		out["edited_at"] = m.GetEditedAt().AsTime().Format(time.RFC3339)
	}
	return out
}

func msgsToJSON(msgs []*chatv1.Message) []map[string]any {
	out := make([]map[string]any, len(msgs))
	for i, m := range msgs {
		out[i] = msgToJSON(m)
	}
	return out
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func httpErr(w http.ResponseWriter, err error) {
	msg := err.Error()
	if strings.Contains(msg, "forbidden") {
		jsonError(w, "forbidden", http.StatusForbidden)
	} else if strings.Contains(msg, "not found") {
		jsonError(w, "not found", http.StatusNotFound)
	} else if strings.Contains(msg, "already exists") {
		jsonError(w, "already exists", http.StatusConflict)
	} else if strings.Contains(msg, "invalid") {
		jsonError(w, msg, http.StatusBadRequest)
	} else {
		jsonError(w, "internal error", http.StatusInternalServerError)
	}
}
