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

func contextWithKeyRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, ctxKeyRole, role)
}

// keyRoleFromContext returns the role of the API key that authenticated this
// data-plane request (anon or service_role). Empty means the request was not
// API-key authenticated (e.g. operator JWT on dashboard routes).
func keyRoleFromContext(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyRole).(string)
	return v
}
