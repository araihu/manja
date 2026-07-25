package contracttest

import (
	"context"
	"testing"
)

type contextMarker struct{}

func markedContext(t testing.TB) context.Context {
	t.Helper()
	return context.WithValue(context.Background(), contextMarker{}, t.Name())
}

func requireSameContext(t testing.TB, want, got context.Context) {
	t.Helper()
	if got != want || got.Value(contextMarker{}) != t.Name() {
		t.Errorf("adapter replaced the caller context")
	}
}
