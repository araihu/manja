package htmlartifact

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	artifact "github.com/araihu/manja/application/htmlartifact"
	storeprimitives "github.com/araihu/manja/internal/adapters/store"
)

func TestStoreCommitAndVerifyCacheHit(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := New(root)
	expected := expectationFixture(t)
	payload := bytes.Repeat([]byte("<article>pets</article>\n"), 4096)

	manifest, err := store.CommitHTML(ctx, "fragments/operations/get-pet.html", expected, renderBytes(payload))
	if err != nil {
		t.Fatal(err)
	}
	if manifest.BuildKey == artifact.BuildKey(manifest.Content.SHA256) {
		t.Fatal("pre-render build key was conflated with exact HTML digest")
	}
	verification, err := store.VerifyHTML(ctx, "fragments/operations/get-pet.html", expected)
	if err != nil {
		t.Fatal(err)
	}
	if !verification.Hit() || verification.Manifest != manifest {
		t.Fatalf("verification = %#v, want hit for %#v", verification, manifest)
	}

}

func TestStoreClassifiesCacheMissesAndCorruption(t *testing.T) {
	ctx := context.Background()
	expected := expectationFixture(t)
	path := "fragments/operations/get-pet.html"

	t.Run("no sidecar", func(t *testing.T) {
		store := New(t.TempDir())
		got, err := store.VerifyHTML(ctx, path, expected)
		if err != nil || got.Status != artifact.VerificationManifestMissing {
			t.Fatalf("verification = %#v, err %v", got, err)
		}
	})

	t.Run("build key mismatch avoids accepting stale HTML", func(t *testing.T) {
		store := New(t.TempDir())
		if _, err := store.CommitHTML(ctx, path, expected, renderBytes([]byte("old"))); err != nil {
			t.Fatal(err)
		}
		changed := expected
		changed.BuildKey = buildKeyForPayload(t, "changed")
		got, err := store.VerifyHTML(ctx, path, changed)
		if err != nil || got.Status != artifact.VerificationBuildKeyMismatch {
			t.Fatalf("verification = %#v, err %v", got, err)
		}
	})

	t.Run("fragment identity mismatch", func(t *testing.T) {
		store := New(t.TempDir())
		if _, err := store.CommitHTML(ctx, path, expected, renderBytes([]byte("old"))); err != nil {
			t.Fatal(err)
		}
		changed := expected
		changed.Fragment.Resource = "catalog/default/spec/pets/operation/createPet"
		got, err := store.VerifyHTML(ctx, path, changed)
		if err != nil || got.Status != artifact.VerificationIdentityMismatch {
			t.Fatalf("verification = %#v, err %v", got, err)
		}
	})

	t.Run("corrupt HTML", func(t *testing.T) {
		root := t.TempDir()
		store := New(root)
		if _, err := store.CommitHTML(ctx, path, expected, renderBytes([]byte("expected"))); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), []byte("corrupt"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := store.VerifyHTML(ctx, path, expected)
		if err != nil || got.Status != artifact.VerificationArtifactCorrupt {
			t.Fatalf("verification = %#v, err %v", got, err)
		}
	})

	t.Run("HTML missing after sidecar", func(t *testing.T) {
		root := t.TempDir()
		store := New(root)
		if _, err := store.CommitHTML(ctx, path, expected, renderBytes([]byte("expected"))); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(root, filepath.FromSlash(path))); err != nil {
			t.Fatal(err)
		}
		got, err := store.VerifyHTML(ctx, path, expected)
		if err != nil || got.Status != artifact.VerificationArtifactMissing {
			t.Fatalf("verification = %#v, err %v", got, err)
		}
	})

	t.Run("invalid sidecar", func(t *testing.T) {
		root := t.TempDir()
		store := New(root)
		resolved := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(resolved+sidecarSuffix, []byte("not-json"), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := store.VerifyHTML(ctx, path, expected)
		if err != nil || got.Status != artifact.VerificationManifestInvalid {
			t.Fatalf("verification = %#v, err %v", got, err)
		}
	})

	t.Run("oversized sidecar", func(t *testing.T) {
		root := t.TempDir()
		store := New(root)
		resolved := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(resolved+sidecarSuffix, bytes.Repeat([]byte("x"), maximumManifestSize+1), 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := store.VerifyHTML(ctx, path, expected)
		if err != nil || got.Status != artifact.VerificationManifestInvalid {
			t.Fatalf("verification = %#v, err %v", got, err)
		}
	})
}

func TestStoreRejectsNonCanonicalSidecars(t *testing.T) {
	ctx := context.Background()
	expected := expectationFixture(t)
	path := "fragments/operations/get-pet.html"
	tests := []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{
			name: "unknown root member",
			mutate: func(data []byte) []byte {
				return bytes.Replace(data, []byte(`"schemaVersion":1`), []byte(`"futureField":true,"schemaVersion":1`), 1)
			},
		},
		{
			name: "unknown nested member",
			mutate: func(data []byte) []byte {
				return bytes.Replace(data, []byte(`"length":8`), []byte(`"futureField":true,"length":8`), 1)
			},
		},
		{
			name: "duplicate root member",
			mutate: func(data []byte) []byte {
				return bytes.Replace(data, []byte(`"buildKey":`), []byte(`"buildKey":"fragment-build-sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","buildKey":`), 1)
			},
		},
		{
			name: "duplicate nested member",
			mutate: func(data []byte) []byte {
				return bytes.Replace(data, []byte(`"sha256":`), []byte(`"sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","sha256":`), 1)
			},
		},
		{
			name:   "trailing value",
			mutate: func(data []byte) []byte { return append(data, []byte("{}\n")...) },
		},
		{
			name: "noncanonical known member casing",
			mutate: func(data []byte) []byte {
				return bytes.Replace(data, []byte(`"buildKey"`), []byte(`"BuildKey"`), 1)
			},
		},
		{
			name: "unsupported schema version",
			mutate: func(data []byte) []byte {
				return bytes.Replace(data, []byte(`"schemaVersion":1`), []byte(`"schemaVersion":2`), 1)
			},
		},
		{
			name: "null content length",
			mutate: func(data []byte) []byte {
				return bytes.Replace(data, []byte(`"length":8`), []byte(`"length":null`), 1)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			store := New(root)
			if _, err := store.CommitHTML(ctx, path, expected, renderBytes([]byte("expected"))); err != nil {
				t.Fatal(err)
			}
			sidecarPath := filepath.Join(root, filepath.FromSlash(path)) + sidecarSuffix
			data, err := os.ReadFile(sidecarPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(sidecarPath, test.mutate(data), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := store.VerifyHTML(ctx, path, expected)
			if err != nil || got.Status != artifact.VerificationManifestInvalid {
				t.Fatalf("verification = %#v, err %v", got, err)
			}
		})
	}
}

func TestStoreStreamsRenderAndDoesNotPublishPartialOutput(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := New(root)
	expected := expectationFixture(t)
	path := "fragments/operations/get-pet.html"
	manifest, err := store.CommitHTML(ctx, path, expected, func(writer io.Writer) error {
		for _, chunk := range []string{"<article>", "pets", "</article>"} {
			if _, err := io.WriteString(writer, chunk); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("<article>pets</article>")
	if manifest.Content.Length != uint64(len(want)) || manifest.Content.SHA256 != artifact.PayloadSHA256(want) {
		t.Fatalf("streamed content identity = %#v", manifest.Content)
	}

	wantErr := errors.New("render failed")
	err = func() error {
		_, err := store.CommitHTML(ctx, path, expected, func(writer io.Writer) error {
			if _, err := io.WriteString(writer, "partial replacement"); err != nil {
				return err
			}
			return wantErr
		})
		return err
	}()
	if !errors.Is(err, wantErr) {
		t.Fatalf("failed render error = %v, want %v", err, wantErr)
	}
	verification, err := store.VerifyHTML(ctx, path, expected)
	if err != nil || !verification.Hit() || verification.Manifest != manifest {
		t.Fatalf("failed render changed committed pair: verification=%#v err=%v", verification, err)
	}
}

func TestStoreRejectsSymlinksAndNonRegularCandidates(t *testing.T) {
	ctx := context.Background()
	expected := expectationFixture(t)
	path := "fragments/operations/get-pet.html"

	t.Run("sidecar symlink", func(t *testing.T) {
		root := t.TempDir()
		store := New(root)
		if _, err := store.CommitHTML(ctx, path, expected, renderBytes([]byte("expected"))); err != nil {
			t.Fatal(err)
		}
		sidecar := filepath.Join(root, filepath.FromSlash(path)) + sidecarSuffix
		outside := filepath.Join(t.TempDir(), "outside.json")
		data, err := os.ReadFile(sidecar)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(outside, data, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(sidecar); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, sidecar); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		got, err := store.VerifyHTML(ctx, path, expected)
		if err != nil || got.Status != artifact.VerificationManifestInvalid {
			t.Fatalf("verification = %#v, err %v", got, err)
		}
	})

	t.Run("artifact symlink", func(t *testing.T) {
		root := t.TempDir()
		store := New(root)
		if _, err := store.CommitHTML(ctx, path, expected, renderBytes([]byte("expected"))); err != nil {
			t.Fatal(err)
		}
		html := filepath.Join(root, filepath.FromSlash(path))
		outside := filepath.Join(t.TempDir(), "outside.html")
		if err := os.WriteFile(outside, []byte("expected"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(html); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, html); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		got, err := store.VerifyHTML(ctx, path, expected)
		if err != nil || got.Status != artifact.VerificationArtifactCorrupt {
			t.Fatalf("verification = %#v, err %v", got, err)
		}
	})

	t.Run("parent symlink escape", func(t *testing.T) {
		root := t.TempDir()
		outside := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(root, "fragments")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		store := New(root)
		if _, err := store.CommitHTML(ctx, path, expected, renderBytes([]byte("expected"))); err == nil {
			t.Fatal("commit through a symlinked parent succeeded")
		}
		if _, err := os.Stat(filepath.Join(outside, "operations", "get-pet.html")); !os.IsNotExist(err) {
			t.Fatalf("escaped artifact was written: %v", err)
		}
	})

	t.Run("non-regular sidecar", func(t *testing.T) {
		root := t.TempDir()
		store := New(root)
		resolved := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(resolved+sidecarSuffix, 0o755); err != nil {
			t.Fatal(err)
		}
		got, err := store.VerifyHTML(ctx, path, expected)
		if err != nil || got.Status != artifact.VerificationManifestInvalid {
			t.Fatalf("verification = %#v, err %v", got, err)
		}
	})
}

func TestStoreCommitsSidecarLastAndRejectsInterruptedPair(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := New(root)
	expected := expectationFixture(t)
	path := "fragments/operations/get-pet.html"
	if _, err := store.CommitHTML(ctx, path, expected, renderBytes([]byte("old HTML"))); err != nil {
		t.Fatal(err)
	}

	writes := 0
	store.write = func(path string, mode fs.FileMode, write func(io.Writer) error) error {
		writes++
		if writes == 2 {
			return errors.New("simulated interruption")
		}
		return storeprimitives.DurableAtomicWriteFunc(path, mode, write)
	}
	changed := expected
	changed.BuildKey = buildKeyForPayload(t, "new payload")
	if _, err := store.CommitHTML(ctx, path, changed, renderBytes([]byte("new HTML"))); err == nil {
		t.Fatal("interrupted sidecar write succeeded")
	}
	if writes != 2 {
		t.Fatalf("writes = %d, want HTML then sidecar", writes)
	}

	store.write = storeprimitives.DurableAtomicWriteFunc
	verification, err := store.VerifyHTML(ctx, path, expected)
	if err != nil {
		t.Fatal(err)
	}
	if verification.Status != artifact.VerificationArtifactCorrupt {
		t.Fatalf("old sidecar/new HTML pair = %#v, want corruption miss", verification)
	}
	verification, err = store.VerifyHTML(ctx, path, changed)
	if err != nil {
		t.Fatal(err)
	}
	if verification.Status != artifact.VerificationBuildKeyMismatch {
		t.Fatalf("new expectation/old sidecar pair = %#v, want build-key miss", verification)
	}
}

func TestStoreRejectsUnsafeArtifactPaths(t *testing.T) {
	store := New(t.TempDir())
	expected := expectationFixture(t)
	for _, path := range []string{"", "..", "../escape.html", "/absolute.html", `fragments\\escape.html`, "fragments/../escape.html"} {
		if _, err := store.CommitHTML(context.Background(), path, expected, renderBytes([]byte("html"))); err == nil {
			t.Errorf("unsafe path %q was accepted", path)
		}
	}
}

func expectationFixture(t *testing.T) artifact.Expectation {
	t.Helper()
	fragment := artifact.FragmentIdentity{
		Format:   "manja-fragment-v1",
		Kind:     artifact.FragmentOperation,
		Resource: "catalog/default/spec/pets/operation/getPet",
	}
	key, err := artifact.NewBuildKey(artifact.BuildKeyInput{
		Fragment:               fragment,
		CanonicalPayloadSHA256: strings.Repeat("a", 64),
		ManjaVersion:           "v2.0.0",
		RendererFingerprint:    "templates-sha256:a",
		UIFingerprint:          "assets-sha256:a",
		CompilerIdentity:       "goshtoso@v0.2.6",
		NormalizerIdentity:     "openapi-normalizer-v3",
	})
	if err != nil {
		t.Fatal(err)
	}
	return artifact.Expectation{Fragment: fragment, BuildKey: key}
}

func buildKeyForPayload(t *testing.T, payload string) artifact.BuildKey {
	t.Helper()
	expected := expectationFixture(t)
	key, err := artifact.NewBuildKey(artifact.BuildKeyInput{
		Fragment:               expected.Fragment,
		CanonicalPayloadSHA256: artifact.PayloadSHA256([]byte(payload)),
		ManjaVersion:           "v2.0.0",
		RendererFingerprint:    "templates-sha256:a",
		UIFingerprint:          "assets-sha256:a",
		CompilerIdentity:       "goshtoso@v0.2.6",
		NormalizerIdentity:     "openapi-normalizer-v3",
	})
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func renderBytes(data []byte) artifact.Render {
	return func(writer io.Writer) error {
		_, err := writer.Write(data)
		return err
	}
}
