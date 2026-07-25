package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	core "github.com/araihu/manja/domain"
)

func TestRunCheckWritesConciseErrors(t *testing.T) {
	var stderr bytes.Buffer
	if code := writeCheckError(&stderr, errors.New("parse failed:\nline 2")); code != 2 {
		t.Fatalf("writeCheckError code = %d, want 2", code)
	}
	if got, want := stderr.String(), "manja check: parse failed: line 2\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestCheckArgsRejectInvalidRequiredAndExclusiveFlags(t *testing.T) {
	base := []string{
		"--config", reviewFixturePath("config", "testdata", "manja.yaml"),
		"--contract", "payments",
		"--target-file", reviewFixturePath("openapi", "testdata", "review", "target.yaml"),
		"--candidate-file", reviewFixturePath("openapi", "testdata", "review", "candidate.yaml"),
	}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "config", args: withoutCheckFlag(base, "--config"), want: "--config is required"},
		{name: "contract", args: withoutCheckFlag(base, "--contract"), want: "--contract is required"},
		{name: "target", args: withoutCheckFlag(base, "--target-file"), want: "target must set exactly one"},
		{name: "candidate", args: withoutCheckFlag(base, "--candidate-file"), want: "candidate must set exactly one"},
		{name: "target file and ref", args: append(append([]string{}, base...), "--target-ref", "main"), want: "target must set exactly one"},
		{name: "candidate file and ref", args: append(append([]string{}, base...), "--candidate-ref", "main"), want: "candidate must set exactly one"},
		{name: "release file and ref", args: append(append([]string{}, base...), "--release-file", "release.yaml", "--release-ref", "v1"), want: "release must set at most one"},
		{name: "format", args: append(append([]string{}, base...), "--format", "xml"), want: "--format must be one of json, text, markdown"},
		{name: "evaluation time", args: append(append([]string{}, base...), "--evaluated-at", "2026-07-25"), want: "--evaluated-at must be RFC3339"},
		{name: "unknown flag", args: append(append([]string{}, base...), "--unknown"), want: "flag provided but not defined"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(context.Background(), append([]string{"check"}, tt.args...), &stdout, &stderr)
			if code != 2 {
				t.Fatalf("run code = %d, want 2; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("stderr = %q, want containing %q", stderr.String(), tt.want)
			}
			if strings.Count(strings.TrimSpace(stderr.String()), "\n") != 0 {
				t.Fatalf("stderr is not concise: %q", stderr.String())
			}
		})
	}
}

func TestRunCheckReturnsZeroForPassingReviewAndPreservesExplicitEvaluationInstant(t *testing.T) {
	spec := reviewFixturePath("openapi", "testdata", "review", "target.yaml")
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{
		"check",
		"--config", reviewFixturePath("config", "testdata", "manja.yaml"),
		"--contract", "payments",
		"--target-file", spec,
		"--candidate-file", spec,
		"--release-file", spec,
		"--format", "json",
		"--evaluated-at", "2026-07-25T09:00:00-03:00",
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("run code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	var report core.ReviewReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	wantTime := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	if report.Verdict != core.VerdictPass || report.EngineVersion != "dev" || !report.EvaluatedAt.Equal(wantTime) {
		t.Fatalf("report = %#v", report)
	}
}

func TestRunCheckDefaultsEvaluationTimeToCurrentUTC(t *testing.T) {
	spec := reviewFixturePath("openapi", "testdata", "review", "target.yaml")
	before := time.Now().UTC()
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{
		"check",
		"--config", reviewFixturePath("config", "testdata", "manja.yaml"),
		"--contract", "payments",
		"--target-file", spec,
		"--candidate-file", spec,
		"--release-file", spec,
		"--format", "json",
	}, &stdout, &stderr)
	after := time.Now().UTC()

	if code != 0 {
		t.Fatalf("run code = %d, want 0; stderr=%q", code, stderr.String())
	}
	var report core.ReviewReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode stdout: %v\n%s", err, stdout.String())
	}
	if report.EvaluatedAt.Before(before) || report.EvaluatedAt.After(after) || report.EvaluatedAt.Location() != time.UTC {
		t.Fatalf("evaluated at = %s, want UTC time between %s and %s", report.EvaluatedAt, before, after)
	}
}

func TestRunCheckReturnsOneForCompletedPolicyFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), append([]string{"check"}, failingReviewArgs("text")...), &stdout, &stderr)

	if code != 1 {
		t.Fatalf("run code = %d, want 1; stderr=%q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Verdict: fail") ||
		!strings.Contains(stdout.String(), "Pull-request delta") ||
		!strings.Contains(stdout.String(), "Release impact") {
		t.Fatalf("stdout missing failing dual-baseline report:\n%s", stdout.String())
	}
}

func TestRunCheckReturnsTwoForConfigInputAndExecutionErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "config",
			args: []string{"--config", "missing-config.yaml", "--contract", "payments", "--target-file", "target.yaml", "--candidate-file", "candidate.yaml"},
			want: "open config",
		},
		{
			name: "input",
			args: []string{
				"--config", reviewFixturePath("config", "testdata", "manja.yaml"),
				"--contract", "payments",
				"--target-file", "missing-target.yaml",
				"--candidate-file", reviewFixturePath("openapi", "testdata", "review", "candidate.yaml"),
				"--release-file", reviewFixturePath("openapi", "testdata", "review", "release.yaml"),
			},
			want: "load target",
		},
		{
			name: "execution",
			args: []string{
				"--config", reviewFixturePath("config", "testdata", "manja.yaml"),
				"--contract", "payments",
				"--target-file", reviewFixturePath("openapi", "testdata", "review", "target.yaml"),
				"--candidate-file", reviewFixturePath("openapi", "testdata", "review", "candidate.yaml"),
			},
			want: "release baseline is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(context.Background(), append([]string{"check"}, tt.args...), &stdout, &stderr)
			if code != 2 {
				t.Fatalf("run code = %d, want 2; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("stderr = %q, want containing %q", stderr.String(), tt.want)
			}
			if strings.Count(strings.TrimSpace(stderr.String()), "\n") != 0 {
				t.Fatalf("stderr is not concise: %q", stderr.String())
			}
		})
	}
}

func failingReviewArgs(format string) []string {
	return []string{
		"--config", reviewFixturePath("config", "testdata", "manja.yaml"),
		"--contract", "payments",
		"--policy", "stable",
		"--target-file", reviewFixturePath("openapi", "testdata", "review", "target.yaml"),
		"--candidate-file", reviewFixturePath("openapi", "testdata", "review", "candidate.yaml"),
		"--release-file", reviewFixturePath("openapi", "testdata", "review", "release.yaml"),
		"--format", format,
		"--evaluated-at", "2026-07-25T12:00:00Z",
	}
}

func reviewFixturePath(parts ...string) string {
	return filepath.Join(append([]string{"..", "..", "internal", "adapters"}, parts...)...)
}

func withoutCheckFlag(args []string, flagName string) []string {
	result := make([]string, 0, len(args)-2)
	for index := 0; index < len(args); index++ {
		if args[index] == flagName {
			index++
			continue
		}
		result = append(result, args[index])
	}
	return result
}
