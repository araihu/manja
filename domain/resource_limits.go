package domain

import "context"

type resourceLimitsKey struct{}

func WithResourceLimits(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, resourceLimitsKey{}, enabled)
}

func ResourceLimitsEnabled(ctx context.Context) bool {
	enabled, configured := ctx.Value(resourceLimitsKey{}).(bool)
	return !configured || enabled
}
