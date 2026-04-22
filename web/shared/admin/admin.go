package admin

import (
	"mal/internal/db"
)

const AdminEmail = "mikkelelvers@outlook.com"

func IsAdmin(user *database.User) bool {
	if user == nil {
		return false
	}
	return user.Username == AdminEmail
}

func IsAdminFromContext(ctx interface{ Value(key interface{}) interface{} }) bool {
	const userKey = "mal:user"
	user, _ := ctx.Value(userKey).(*database.User)
	return IsAdmin(user)
}