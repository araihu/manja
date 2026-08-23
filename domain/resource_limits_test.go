package domain

import (
	"context"
	"testing"
)

func TestResourceLimitsDefaultOnAndCanBeDisabled(t *testing.T) {
	if !ResourceLimitsEnabled(context.Background()) {
		t.Fatal("resource limits defaulted off without explicit runtime policy")
	}
	if ResourceLimitsEnabled(WithResourceLimits(context.Background(), false)) {
		t.Fatal("explicit unbounded policy was ignored")
	}
}
