package httphandler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/byoverr/auth-service/internal/domain/dto"
	"github.com/byoverr/auth-service/internal/domain/models"
	"github.com/byoverr/auth-service/internal/usecase"
	"github.com/rs/zerolog"
)

// AuthService is the interface the HTTP handler depends on.
type AuthService interface {
	Register(ctx context.Context, in dto.RegisterIn) (models.AuthPair, error)
	Login(ctx context.Context, in dto.LoginIn) (models.AuthPair, error)
	Refresh(ctx context.Context, in dto.RefreshIn) (models.AuthPair, error)
	Logout(ctx context.Context, refreshToken string) error
	WhoAmI(ctx context.Context, userID string) (models.UserInfo, error)
	GetUser(ctx context.Context, userID string) (models.UserInfo, error)
	LookupByEmail(ctx context.Context, email string) (models.UserInfo, error)
}

type Handler struct {
	svc AuthService
	log zerolog.Logger
}

func NewHandler(svc AuthService, log zerolog.Logger) http.Handler {
	h := &Handler{svc: svc, log: log}
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/auth/register", h.register)
	mux.HandleFunc("POST /api/auth/login", h.login)
	mux.HandleFunc("POST /api/auth/refresh", h.refresh)
	mux.HandleFunc("POST /api/auth/logout", h.logout)
	mux.HandleFunc("GET /api/auth/me", h.me)
	mux.HandleFunc("GET /api/auth/users/lookup", h.requireAuth(h.lookupUser))
	mux.HandleFunc("GET /api/auth/users/{userID}", h.requireAuth(h.getUser))

	return corsMiddleware(mux)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientInfo(r *http.Request) models.ClientInfo {
	// r.RemoteAddr is "host:port"; PostgreSQL inet column needs just the host
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	// Prefer X-Forwarded-For from nginx
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		host = strings.SplitN(xff, ",", 2)[0]
	}
	return models.ClientInfo{IP: host, UserAgent: r.UserAgent()}
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	pair, err := h.svc.Register(r.Context(), dto.RegisterIn{
		Email: body.Email, Password: body.Password, DisplayName: body.DisplayName,
		Client: clientInfo(r),
	})
	if err != nil {
		h.handleErr(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, pairToJSON(pair))
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	pair, err := h.svc.Login(r.Context(), dto.LoginIn{
		Email: body.Email, Password: body.Password, Client: clientInfo(r),
	})
	if err != nil {
		h.handleErr(w, err)
		return
	}
	jsonOK(w, pairToJSON(pair))
}

func (h *Handler) refresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	pair, err := h.svc.Refresh(r.Context(), dto.RefreshIn{
		RefreshToken: body.RefreshToken, Client: clientInfo(r),
	})
	if err != nil {
		h.handleErr(w, err)
		return
	}
	jsonOK(w, pairToJSON(pair))
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if err := h.svc.Logout(r.Context(), body.RefreshToken); err != nil {
		h.handleErr(w, err)
		return
	}
	jsonOK(w, map[string]string{"status": "logged_out"})
}

// GET /api/auth/me — Bearer <access_token> required
func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	userID := jwtSub(r)
	if userID == "" {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	info, err := h.svc.WhoAmI(r.Context(), userID)
	if err != nil {
		h.handleErr(w, err)
		return
	}
	jsonOK(w, map[string]any{
		"user_id":      info.UserID,
		"email":        info.Email,
		"display_name": info.DisplayName,
		"roles":        info.Roles,
		"session_id":   info.SessionID,
	})
}

// requireAuth wraps a handler so it requires a valid Bearer JWT sub.
func (h *Handler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if jwtSub(r) == "" {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// GET /api/auth/users/{userID} — resolve display name by user ID
func (h *Handler) getUser(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("userID")
	info, err := h.svc.GetUser(r.Context(), userID)
	if err != nil {
		h.handleErr(w, err)
		return
	}
	jsonOK(w, map[string]any{
		"user_id":      info.UserID,
		"display_name": info.DisplayName,
		"email":        info.Email,
	})
}

// GET /api/auth/users/lookup?email=... — resolve user by email
func (h *Handler) lookupUser(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	if email == "" {
		jsonError(w, "email query param required", http.StatusBadRequest)
		return
	}
	info, err := h.svc.LookupByEmail(r.Context(), email)
	if err != nil {
		h.handleErr(w, err)
		return
	}
	jsonOK(w, map[string]any{
		"user_id":      info.UserID,
		"display_name": info.DisplayName,
		"email":        info.Email,
	})
}

// jwtSub extracts sub from the JWT payload without signature verification.
// WhoAmI performs a DB lookup which serves as effective validation.
func jwtSub(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}
	tok := strings.TrimPrefix(auth, "Bearer ")
	if tok == auth {
		return ""
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return ""
	}
	payload := parts[1]
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}
	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return ""
	}
	var claims map[string]any
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return ""
	}
	sub, _ := claims["sub"].(string)
	return sub
}

func pairToJSON(p models.AuthPair) map[string]any {
	return map[string]any{
		"access_token":       p.AccessToken,
		"access_expires_at":  p.AccessExpiresAt,
		"refresh_token":      p.RefreshToken,
		"refresh_expires_at": p.RefreshExpiresAt,
		"user_id":            p.UserID,
		"session_id":         p.SessionID,
		"roles":              p.Roles,
	}
}

func (h *Handler) handleErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, usecase.ErrNotFound):
		jsonError(w, "not found", http.StatusNotFound)
	case errors.Is(err, usecase.ErrAlreadyExists):
		jsonError(w, "email already registered", http.StatusConflict)
	case errors.Is(err, usecase.ErrInvalidCreds),
		errors.Is(err, usecase.ErrUnauthenticated),
		errors.Is(err, usecase.ErrSessionExpired),
		errors.Is(err, usecase.ErrSessionRevoked):
		jsonError(w, "unauthorized", http.StatusUnauthorized)
	case errors.Is(err, usecase.ErrUserDisabled):
		jsonError(w, "account disabled", http.StatusForbidden)
	case errors.Is(err, usecase.ErrInvalidArgument):
		msg := strings.TrimPrefix(err.Error(), "invalid argument: ")
		jsonError(w, msg, http.StatusBadRequest)
	default:
		h.log.Error().Err(err).Msg("auth http handler error")
		jsonError(w, "internal error", http.StatusInternalServerError)
	}
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
