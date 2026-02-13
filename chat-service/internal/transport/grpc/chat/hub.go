package chatgrpc

import (
	"errors"
	"sync"

	chatv1 "github.com/byoverr/auth-proto/gen/go/chat/v1" // поправь импорт под себя
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type idSet map[string]struct{}

func (s idSet) add(id string) { s[id] = struct{}{} }
func (s idSet) del(id string) { delete(s, id) }
func (s idSet) len() int      { return len(s) }

func ensureSet(m map[string]idSet, key string) idSet {
	s := m[key]
	if s == nil {
		s = idSet{}
		m[key] = s
	}
	return s
}

type StreamSender interface {
	Send(*chatv1.ServerEvent) error
}

type BackpressurePolicy int

const (
	DropIfFull BackpressurePolicy = iota
	DisconnectIfFull
)

var (
	ErrConnClosed    = errors.New("conn closed")
	ErrDropped       = errors.New("dropped (buffer full)")
	ErrSlowConsumer  = errors.New("slow consumer (buffer full)")
	ErrUnknownConnID = errors.New("unknown conn id")
)

type Conn struct {
	ID     string
	UserID string

	sendCh chan *chatv1.ServerEvent
	sender StreamSender

	doneOnce sync.Once
	done     chan struct{}
}

func newConn(userID string, sender StreamSender, bufSize int) *Conn {
	if bufSize <= 0 {
		bufSize = 64
	}
	return &Conn{
		ID:     uuid.NewString(),
		UserID: userID,
		sendCh: make(chan *chatv1.ServerEvent, bufSize),
		sender: sender,
		done:   make(chan struct{}),
	}
}

func (c *Conn) Done() <-chan struct{} { return c.done }

// Enqueue — безопасная неблокирующая постановка события в буфер.
// Важно: мы НЕ закрываем sendCh, чтобы не словить panic "send on closed channel".
func (c *Conn) Enqueue(ev *chatv1.ServerEvent, p BackpressurePolicy) error {
	select {
	case <-c.done:
		return ErrConnClosed
	default:
	}

	switch p {
	case DropIfFull:
		select {
		case c.sendCh <- ev:
			return nil
		default:
			return ErrDropped
		}
	case DisconnectIfFull:
		select {
		case c.sendCh <- ev:
			return nil
		default:
			c.Close()
			return ErrSlowConsumer
		}
	default:
		// по умолчанию мягко
		select {
		case c.sendCh <- ev:
			return nil
		default:
			return ErrDropped
		}
	}
}

func (c *Conn) Close() {
	c.doneOnce.Do(func() { close(c.done) })
}

// Connect-loop должен читать отсюда и писать в gRPC stream одним writer’ом.
func (c *Conn) SendCh() <-chan *chatv1.ServerEvent { return c.sendCh }

type Hub struct {
	logger zerolog.Logger
	mu     sync.RWMutex

	connsByID map[string]*Conn // connID -> conn
	userConns map[string]idSet // userID -> set(connID)
	chatConns map[string]idSet // chatID -> set(connID)
	connChats map[string]idSet // connID -> set(chatID)
}

func NewHub(logger zerolog.Logger) *Hub {
	return &Hub{
		logger:    logger,
		connsByID: make(map[string]*Conn),
		userConns: make(map[string]idSet),
		chatConns: make(map[string]idSet),
		connChats: make(map[string]idSet),
	}
}

func (h *Hub) Register(userID string, sender StreamSender, bufSize int) *Conn {
	conn := newConn(userID, sender, bufSize)

	h.mu.Lock()
	defer h.mu.Unlock()

	h.connsByID[conn.ID] = conn
	ensureSet(h.userConns, userID).add(conn.ID)
	// connChats будет заполняться при подписках
	return conn
}

func (h *Hub) Unregister(connID string) {
	var conn *Conn

	h.mu.Lock()
	conn = h.connsByID[connID]
	if conn == nil {
		h.mu.Unlock()
		return
	}

	// 1) убрать из connsByID
	delete(h.connsByID, connID)

	// 2) убрать из userConns
	if s := h.userConns[conn.UserID]; s != nil {
		s.del(connID)
		if s.len() == 0 {
			delete(h.userConns, conn.UserID)
		}
	}

	// 3) убрать из всех чатов (chatConns) по индексу connChats
	if chats := h.connChats[connID]; chats != nil {
		for chatID := range chats {
			if cs := h.chatConns[chatID]; cs != nil {
				cs.del(connID)
				if cs.len() == 0 {
					delete(h.chatConns, chatID)
				}
			}
		}
		delete(h.connChats, connID)
	}

	h.mu.Unlock()

	// закрывать/останавливать conn — уже без лока
	conn.Close()
}

func (h *Hub) SubscribeUserToChat(userID, chatID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	conns := h.userConns[userID]
	if conns == nil {
		return
	}
	for connID := range conns {
		_ = h.subscribeConnToChatLocked(connID, chatID)
	}
}

func (h *Hub) UnsubscribeUserFromChat(userID, chatID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	conns := h.userConns[userID]
	if conns == nil {
		return
	}
	for connID := range conns {
		_ = h.unsubscribeConnFromChatLocked(connID, chatID)
	}
}

func (h *Hub) SubscribeConnToChat(connID, chatID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.subscribeConnToChatLocked(connID, chatID)
}

func (h *Hub) UnsubscribeConnFromChat(connID, chatID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.unsubscribeConnFromChatLocked(connID, chatID)
}

func (h *Hub) SubscribeConnToChats(connID string, chatIDs []string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.connsByID[connID] == nil {
		return ErrUnknownConnID
	}
	for _, chatID := range chatIDs {
		_ = h.subscribeConnToChatLocked(connID, chatID)
	}
	return nil
}

func (h *Hub) subscribeConnToChatLocked(connID, chatID string) error {
	if h.connsByID[connID] == nil {
		return ErrUnknownConnID
	}

	ensureSet(h.chatConns, chatID).add(connID)
	ensureSet(h.connChats, connID).add(chatID)
	return nil
}

func (h *Hub) unsubscribeConnFromChatLocked(connID, chatID string) error {
	if h.connsByID[connID] == nil {
		return ErrUnknownConnID
	}

	if cs := h.chatConns[chatID]; cs != nil {
		cs.del(connID)
		if cs.len() == 0 {
			delete(h.chatConns, chatID)
		}
	}

	if chats := h.connChats[connID]; chats != nil {
		chats.del(chatID)
		if chats.len() == 0 {
			delete(h.connChats, connID)
		}
	}
	return nil
}

// BroadcastToChat: берём снапшот conn’ов под RLock, отправляем уже без лока.
// Если DisconnectIfFull сработал — удаляем такие коннекты из Hub.
func (h *Hub) BroadcastToChat(chatID string, ev *chatv1.ServerEvent, p BackpressurePolicy) (sent int, dropped int) {
	var conns []*Conn

	h.mu.RLock()
	subs := h.chatConns[chatID]
	for connID := range subs {
		if c := h.connsByID[connID]; c != nil {
			conns = append(conns, c)
		}
	}
	h.mu.RUnlock()

	var kick []string
	for _, c := range conns {
		err := c.Enqueue(ev, p)
		switch err {
		case nil:
			sent++
		case ErrDropped, ErrConnClosed:
			dropped++
		case ErrSlowConsumer:
			dropped++
			kick = append(kick, c.ID)
		default:
			dropped++
		}
	}

	for _, id := range kick {
		h.Unregister(id)
	}
	return sent, dropped
}
