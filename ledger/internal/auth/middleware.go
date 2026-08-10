package auth

import (
	"context"
	"strings"
)

type contextKey string

const userContextKey contextKey = "authenticated_user_id"

func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userContextKey, userID)
}

func UserIDFromContext(ctx context.Context) (string, bool) {
	value, ok := ctx.Value(userContextKey).(string)
	return value, ok
}

func ExtractBearerToken(header string) string {
	if header == "" {
		return ""
	}

	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}

	return parts[1]
}
