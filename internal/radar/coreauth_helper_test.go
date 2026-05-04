package radar_test

import (
	"context"

	coreauth "github.com/ismd/linktheca/internal/core/auth"
)

func coreauthWithUser(ctx context.Context, userID int64, isAdmin bool) context.Context {
	return coreauth.WithUser(ctx, userID, isAdmin)
}
