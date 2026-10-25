package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the real buildinfo and retained notices, then corrupt the packaged
// artifact. This guards the publication boundary rather than a synthetic PASS.
func TestBuiltRuntimePackagingRejectsChangedImage(t *testing.T) {
	source, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	binary := filepath.Join(root, runtimePath)
	if err := os.MkdirAll(filepath.Dir(binary), 0755); err != nil {
		t.Fatal(err)
	}
	build := exec.Command("go", "build", "-tags=manja_runtime", "-trimpath", "-o", binary, "./cmd/manja-runtime")
	build.Dir = source
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOEXPERIMENT=jsonv2")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("runtime build: %v\n%s", err, output)
	}
	for _, name := range []string{"app/renderer/renderer.yaml", "app/renderer/default-allowlist.json", "etc/ssl/certs/ca-certificates.crt"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("test fixture\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := assemble(source, root); err != nil {
		t.Fatal(err)
	}
	if err := verifyRoot(root); err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(root, manifestPath))
	if err != nil {
		t.Fatal(err)
	}
	var packaged inventory
	if err := json.Unmarshal(manifest, &packaged); err != nil {
		t.Fatal(err)
	}
	if len(packaged.Components) < 2 {
		t.Fatal("runtime/browser inventory missing")
	}
	for _, change := range []string{"notice bytes", "notice missing", "unexpected file", "executable mode", "symbolic link", "dependency omitted"} {
		t.Run(change, func(t *testing.T) {
			path := filepath.Join(root, legalDir, "NOTICE")
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				os.Remove(filepath.Join(root, "unexpected"))
				os.Remove(path)
				os.WriteFile(path, original, 0644)
				os.Chmod(binary, 0755)
				os.WriteFile(filepath.Join(root, manifestPath), manifest, 0644)
			})
			switch change {
			case "notice bytes":
				err = os.WriteFile(path, []byte("changed"), 0644)
			case "notice missing":
				err = os.Remove(path)
			case "unexpected file":
				err = os.WriteFile(filepath.Join(root, "unexpected"), []byte("extra"), 0644)
			case "executable mode":
				err = os.Chmod(binary, 0644)
			case "symbolic link":
				if err = os.Remove(path); err == nil {
					err = os.Symlink("LICENSE", path)
				}
			case "dependency omitted":
				modified := packaged
				modified.Components = modified.Components[1:]
				data, marshalErr := json.Marshal(modified)
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				err = os.WriteFile(filepath.Join(root, manifestPath), data, 0644)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := verifyRoot(root); err == nil {
				t.Fatal("changed image passed verification")
			}
		})
	}
}

func TestUpstreamLicenseChecksumIsRequired(t *testing.T) {
	source := t.TempDir()
	path := filepath.Join(source, "licenses/oci")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "araihu-assets.json"), []byte(`{"assetsRevision":"test"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "LICENSE"), []byte("changed upstream"), 0644); err != nil {
		t.Fatal(err)
	}
	entries := []map[string]string{{"path": "LICENSE", "url": "https://example.org/LICENSE", "sha256": strings.Repeat("0", 64)}}
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "sources.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := copyUpstream(source, t.TempDir()); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("error=%v", err)
	}
}

func TestAssetNoticesMustMatchPinnedRevision(t *testing.T) {
	source := t.TempDir()
	path := filepath.Join(source, "licenses/oci")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "araihu-assets.json"), []byte(`{"assetsRevision":"current"}`), 0644); err != nil {
		t.Fatal(err)
	}
	entries := []map[string]string{{"path": "assets-NOTICE", "url": "https://raw.githubusercontent.com/araihu/assets/old/NOTICE", "sha256": strings.Repeat("0", 64)}}
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "sources.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := copyUpstream(source, t.TempDir()); err == nil || !strings.Contains(err.Error(), "pinned asset revision") {
		t.Fatalf("error=%v", err)
	}
}
