package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"mal/internal/database"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUserExists         = errors.New("username already exists")
	ErrNotAuthenticated   = errors.New("not authenticated")
)

type Service struct {
	db database.Querier
}

func NewService(db database.Querier) *Service {
	return &Service{db: db}
}

// generateSessionToken generates a secure random 32-byte token.
func generateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func (s *Service) RegisterUser(ctx context.Context, username, password string) (*database.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	id := uuid.New().String()
	user, err := s.db.CreateUser(ctx, database.CreateUserParams{
		ID:           id,
		Username:     username,
		PasswordHash: string(hash),
	})
	if err != nil {
		// Assuming unique constraint failure for username
		return nil, ErrUserExists
	}

	return &user, nil
}

func (s *Service) Login(ctx context.Context, username, password string) (*database.Session, error) {
	user, err := s.db.GetUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("failed to lookup user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	token, err := generateSessionToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate session token: %w", err)
	}

	expiresAt := time.Now().Add(30 * 24 * time.Hour) // 30 days
	session, err := s.db.CreateSession(ctx, database.CreateSessionParams{
		ID:        token,
		UserID:    user.ID,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return &session, nil
}

func (s *Service) Logout(ctx context.Context, sessionID string) error {
	return s.db.DeleteSession(ctx, sessionID)
}

func (s *Service) ValidateSession(ctx context.Context, sessionID string) (*database.User, error) {
	session, err := s.db.GetSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotAuthenticated
		}
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	if time.Now().After(session.ExpiresAt) {
		_ = s.db.DeleteSession(ctx, sessionID)
		return nil, ErrNotAuthenticated
	}

	user, err := s.db.GetUser(ctx, session.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user for session: %w", err)
	}

	return &user, nil
}

func SetSessionCookie(w http.ResponseWriter, sessionID string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    sessionID,
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   false, // False for local development without TLS
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})
}

func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    "",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   false, // False for local development without TLS
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})
}
