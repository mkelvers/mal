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
	"unicode"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"mal/internal/database"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrUserExists         = errors.New("username already exists")
	ErrNotAuthenticated   = errors.New("not authenticated")
	ErrInvalidPassword    = errors.New("password does not meet security requirements")
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

func ValidatePassword(password string) error {
	if len(password) < 12 {
		return fmt.Errorf("password must be at least 12 characters long")
	}

	var hasUpper, hasLower, hasNumber, hasSpecial bool
	for _, c := range password {
		switch {
		case unicode.IsNumber(c):
			hasNumber = true
		case unicode.IsUpper(c):
			hasUpper = true
		case unicode.IsLower(c):
			hasLower = true
		case unicode.IsPunct(c) || unicode.IsSymbol(c) || !unicode.IsLetter(c) && !unicode.IsNumber(c):
			hasSpecial = true
		}
	}

	if !hasUpper || !hasLower || !hasNumber || !hasSpecial {
		return fmt.Errorf("password must contain at least one uppercase letter, one lowercase letter, one number, and one special character")
	}
	return nil
}

func (s *Service) RegisterUser(ctx context.Context, username, password string) (*database.User, error) {
	if err := ValidatePassword(password); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPassword, err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12) // higher cost
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
	isProd := os.Getenv("ENV") == "production"
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    sessionID,
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   isProd,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
	})
}

func ClearSessionCookie(w http.ResponseWriter) {
	isProd := os.Getenv("ENV") == "production"
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    "",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   isProd,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
	})
}
