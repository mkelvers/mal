package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
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
	ErrInvalidRecoveryKey = errors.New("invalid recovery details")
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

func generateRecoveryKey() (string, error) {
	return generateToken(24)
}

func hashRecoveryKey(recoveryKey string) string {
	sum := sha256.Sum256([]byte(recoveryKey))
	return hex.EncodeToString(sum[:])
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

func (s *Service) RegisterUser(ctx context.Context, username, password string) (*database.User, string, error) {
	if err := ValidatePassword(password); err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrInvalidPassword, err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return nil, "", fmt.Errorf("failed to hash password: %w", err)
	}

	recoveryKey, err := generateRecoveryKey()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate recovery key: %w", err)
	}

	id := uuid.New().String()
	user, err := s.db.CreateUser(ctx, database.CreateUserParams{
		ID:              id,
		Username:        username,
		PasswordHash:    string(hash),
		RecoveryKeyHash: hashRecoveryKey(recoveryKey),
	})
	if err != nil {
		// Assuming unique constraint failure for username
		return nil, "", ErrUserExists
	}

	return &user, recoveryKey, nil
}

func (s *Service) RecoverAccount(ctx context.Context, username, recoveryKey, newPassword string) (string, error) {
	if err := ValidatePassword(newPassword); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidPassword, err)
	}

	user, err := s.db.GetUserByUsernameAndRecoveryKeyHash(ctx, database.GetUserByUsernameAndRecoveryKeyHashParams{
		Username:        username,
		RecoveryKeyHash: hashRecoveryKey(recoveryKey),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrInvalidRecoveryKey
		}
		return "", fmt.Errorf("failed to lookup user for recovery: %w", err)
	}

	newPasswordHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash new password: %w", err)
	}

	newRecoveryKey, err := generateRecoveryKey()
	if err != nil {
		return "", fmt.Errorf("failed to generate new recovery key: %w", err)
	}

	err = s.db.UpdateUserPasswordAndRecoveryKeyHash(ctx, database.UpdateUserPasswordAndRecoveryKeyHashParams{
		ID:              user.ID,
		PasswordHash:    string(newPasswordHash),
		RecoveryKeyHash: hashRecoveryKey(newRecoveryKey),
	})
	if err != nil {
		return "", fmt.Errorf("failed to update recovered account: %w", err)
	}

	err = s.db.DeleteUserSessions(ctx, user.ID)
	if err != nil {
		return "", fmt.Errorf("failed to clear existing sessions: %w", err)
	}

	return newRecoveryKey, nil
}

func (s *Service) ChangePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	if err := ValidatePassword(newPassword); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidPassword, err)
	}

	user, err := s.db.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to lookup user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)); err != nil {
		return ErrInvalidCredentials
	}

	newPasswordHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcryptCost)
	if err != nil {
		return fmt.Errorf("failed to hash new password: %w", err)
	}

	err = s.db.UpdateUserPasswordAndRecoveryKeyHash(ctx, database.UpdateUserPasswordAndRecoveryKeyHashParams{
		ID:              user.ID,
		PasswordHash:    string(newPasswordHash),
		RecoveryKeyHash: user.RecoveryKeyHash,
	})
	if err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}

	return nil
}

func (s *Service) RegenerateRecoveryKey(ctx context.Context, userID, password string) (string, error) {
	user, err := s.db.GetUser(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("failed to lookup user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", ErrInvalidCredentials
	}

	newRecoveryKey, err := generateRecoveryKey()
	if err != nil {
		return "", fmt.Errorf("failed to generate new recovery key: %w", err)
	}

	err = s.db.UpdateUserPasswordAndRecoveryKeyHash(ctx, database.UpdateUserPasswordAndRecoveryKeyHashParams{
		ID:              user.ID,
		PasswordHash:    user.PasswordHash,
		RecoveryKeyHash: hashRecoveryKey(newRecoveryKey),
	})
	if err != nil {
		return "", fmt.Errorf("failed to rotate recovery key: %w", err)
	}

	return newRecoveryKey, nil
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
