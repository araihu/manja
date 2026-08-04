package main

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestRuntimeCommandAcceptsOnlyRecoveryInputs(t *testing.T) {
	originalServe := serveRecovered
	t.Cleanup(func() { serveRecovered = originalServe })
	var got runtimeConfig
	serveRecovered = func(_ context.Context, cfg runtimeConfig) error {
		got = cfg
		return nil
	}
	var stderr bytes.Buffer
	code := run(context.Background(), []string{"-addr", ":9090", "-renderer-config", "/app/renderer.yaml", "-data-dir", "/app/data"}, &stderr)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("run code=%d stderr=%q", code, stderr.String())
	}
	want := runtimeConfig{Addr: ":9090", RendererConfig: "/app/renderer.yaml", DataDir: "/app/data"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime config = %#v, want %#v", got, want)
	}
}

func TestRuntimeCommandHasNoBuildOrSourceRefreshSurface(t *testing.T) {
	originalServe := serveRecovered
	t.Cleanup(func() { serveRecovered = originalServe })
	serveRecovered = func(context.Context, runtimeConfig) error { return errors.New("must not start") }
	for _, args := range [][]string{{"build"}, {"-spec", "source.json"}, {"-source", "git"}, {"-git-repo", "https://example.test/spec.git"}} {
		var stderr bytes.Buffer
		if code := run(context.Background(), args, &stderr); code != 2 {
			t.Errorf("run(%v) code=%d stderr=%q, want 2", args, code, stderr.String())
		}
		if strings.Contains(stderr.String(), "must not start") {
			t.Errorf("run(%v) reached server", args)
		}
	}
}
