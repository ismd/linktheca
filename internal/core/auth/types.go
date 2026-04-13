package auth

import (
	"context"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	IsAdmin bool `json:"is_admin"`
	jwt.RegisteredClaims
}

type ctxKey int

const (
	userIDKey ctxKey = iota
	isAdminKey
)

func WithUser(ctx context.Context, userID int64, isAdmin bool) context.Context {
	ctx = context.WithValue(ctx, userIDKey, userID)
	ctx = context.WithValue(ctx, isAdminKey, isAdmin)
	return ctx
}

func UserID(ctx context.Context) int64 {
	v, ok := ctx.Value(userIDKey).(int64)
	if !ok {
		panic("auth: UserID called without auth middleware in chain")
	}

	return v
}

func IsAdmin(ctx context.Context) bool {
	v, _ := ctx.Value(isAdminKey).(bool)
	return v
}
