package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"golang.org/x/crypto/bcrypt"

	"mal/internal/db"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrNotAuthenticated   = errors.New("not authenticated")
)

const bcryptCost = 12

type Service struct {
	db database.Querier
}

func NewService(db database.Querier) *Service {
	return &Service{db: db}
}

func generateToken(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func generateSessionToken() (string, error) {
	return generateToken(32)
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
	secure := os.Getenv("ENV") == "production" || os.Getenv("FORCE_SECURE_COOKIES") == "true"
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    sessionID,
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
	})
}

func (s *Service) Logout(ctx context.Context, sessionID string) error {
	return s.db.DeleteSession(ctx, sessionID)
}

func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    "",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Path:     "/",
	})
}
