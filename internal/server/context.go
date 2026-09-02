package server

import "context"

func contextWithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, ctxUserID, userID)
}

func userIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxUserID).(string)
	return v
}

func contextWithProjectID(ctx context.Context, projectID string) context.Context {
	return context.WithValue(ctx, ctxProjectID, projectID)
}

func projectIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxProjectID).(string)
	return v
}
