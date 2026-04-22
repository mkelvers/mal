package admin

import (
	"context"

	"mal/internal/db"
)

type contextKey string

const userContextKey contextKey = "user"

const AdminEmail = "mikkelelvers@outlook.com"

func IsAdmin(user *database.User) bool {
	if user == nil {
		return false
	}
	return user.Username == AdminEmail
}

func GetUser(ctx context.Context) *database.User {
	user, ok := ctx.Value(userContextKey).(*database.User)
	if !ok {
		return nil
	}
	return user
}

func IsAdminFromContext(ctx context.Context) bool {
	return IsAdmin(GetUser(ctx))
}
