// oci-package assembles notices for the built runtime and verifies the complete
// scratch filesystem. It is a build tool and is never copied into the image.
package main

import (
	"bytes"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/araihu/manja/internal/distribution"
	"gopkg.in/yaml.v3"
)

const legalDir = "usr/share/licenses/manja"
const manifestPath = legalDir + "/inventory.json"
const runtimePath = "usr/local/bin/manja"

type component struct {
	Name     string   `json:"name"`
	Version  string   `json:"version"`
	Sum      string   `json:"sum,omitempty"`
	Licenses []string `json:"licenses"`
}

type inventory struct {
	SchemaVersion int                         `json:"schemaVersion"`
	GoVersion     string                      `json:"goVersion"`
	Components    []component                 `json:"components"`
	Files         []distribution.FileEvidence `json:"files"`
}

func main() {
	root := flag.String("root", "", "complete scratch filesystem root")
	source := flag.String("source", ".", "repository root (assemble only)")
	verify := flag.Bool("verify", false, "verify an already assembled filesystem")
	flag.Parse()
	if *root == "" || flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	var err error
	if *verify {
		err = verifyRoot(*root)
	} else {
		err = assemble(*source, *root)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "oci-package:", err)
		os.Exit(1)
	}
	fmt.Println("OCI filesystem and notices verified")
}

func assemble(source, root string) error {
	if _, err := os.Stat(filepath.Join(root, manifestPath)); !os.IsNotExist(err) {
		return fmt.Errorf("assemble requires a fresh root without an inventory")
	}
	info, err := buildinfo.ReadFile(filepath.Join(root, runtimePath))
	if err != nil {
		return err
	}
	result := inventory{SchemaVersion: 1, GoVersion: info.GoVersion}
	for _, name := range []string{"LICENSE", "NOTICE", "THIRD_PARTY_NOTICES.md"} {
		if err := copyFile(filepath.Join(source, name), filepath.Join(root, legalDir, name)); err != nil {
			return err
		}
	}
	for _, mod := range info.Deps {
		if mod.Replace != nil {
			return fmt.Errorf("replaced runtime module: %s", mod.Path)
		}
		data, err := exec.Command("go", "mod", "download", "-json", mod.Path+"@"+mod.Version).Output()
		if err != nil {
			return fmt.Errorf("resolve %s: %w", mod.Path, err)
		}
		var downloaded struct{ Dir, Sum string }
		if err := json.Unmarshal(data, &downloaded); err != nil {
			return err
		}
		if downloaded.Dir == "" {
			return fmt.Errorf("module directory missing: %s", mod.Path)
		}
		if downloaded.Sum != mod.Sum || mod.Sum == "" {
			return fmt.Errorf("module checksum mismatch: %s", mod.Path)
		}
		prefix := legalDir + "/go/" + mod.Path + "@" + mod.Version
		licenses, err := copyLegalTree(downloaded.Dir, root, prefix)
		if err != nil {
			return err
		}
		if len(licenses) == 0 {
			return fmt.Errorf("runtime module has no retained license: %s", mod.Path)
		}
		result.Components = append(result.Components, component{mod.Path, mod.Version, mod.Sum, licenses})
	}
	// -trimpath deliberately removes runtime.GOROOT from this build tool. Ask
	// the installed compiler for the source tree whose notices we distribute.
	goRoot, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return fmt.Errorf("resolve Go license root: %w", err)
	}
	toolchainRoot := strings.TrimSpace(string(goRoot))
	if !filepath.IsAbs(toolchainRoot) {
		return fmt.Errorf("Go license root must be absolute")
	}
	goLicenses, err := copyLegalTree(toolchainRoot, root, legalDir+"/go-toolchain")
	if err != nil {
		return err
	}
	result.Components = append(result.Components, component{Name: "Go", Version: info.GoVersion, Licenses: goLicenses})
	browser, err := copyBrowser(source, root)
	if err != nil {
		return err
	}
	result.Components = append(result.Components, browser...)
	if err := copyUpstream(source, root); err != nil {
		return err
	}
	for _, name := range []string{
		"muamba.yaml", "araihu-assets.json", "docs/legal/browser-bundles.md",
		"internal/renderer/testdata/kubernetes/LICENSE",
		"internal/renderer/testdata/kubernetes/catalog-source.json",
		"internal/renderer/testdata/kubernetes/receipt.json",
		"internal/renderer/testdata/kubernetes/stripe-openapi.provenance.json",
		"internal/adapters/openapi/testdata/github-v3-rest.provenance.json",
		"internal/web/static/simple-icons.provenance.json", "internal/web/static/twilio-mark.provenance.json",
	} {
		if err := copyFile(filepath.Join(source, name), filepath.Join(root, legalDir, "provenance", name)); err != nil {
			return err
		}
	}
	files, err := distribution.InspectRoot(root, distribution.RootOptions{})
	if err != nil {
		return err
	}
	result.Files = files.Files
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, manifestPath), append(data, '\n'), 0644); err != nil {
		return err
	}
	return verifyRoot(root)
}

func copyBrowser(source, root string) ([]component, error) {
	var manifest struct {
		Resources map[string]struct {
			Version   string `yaml:"version"`
			Downloads map[string]struct {
				Path string `yaml:"path"`
			} `yaml:"downloads"`
		} `yaml:"resources"`
	}
	data, err := os.ReadFile(filepath.Join(source, "muamba.yaml"))
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	var names []string
	for name := range manifest.Resources {
		names = append(names, name)
	}
	sort.Strings(names)
	var result []component
	for _, name := range names {
		resource := manifest.Resources[name]
		if !strings.HasPrefix(resource.Downloads["archive"].Path, "internal/webassets/vendor/") {
			continue
		}
		path := strings.ReplaceAll(resource.Downloads["license"].Path, "${version}", resource.Version)
		if !fs.ValidPath(path) || !strings.HasPrefix(path, "internal/webassets/vendor/") {
			return nil, fmt.Errorf("browser license missing: %s", name)
		}
		destination := legalDir + "/browser/" + strings.TrimPrefix(path, "internal/webassets/vendor/")
		if err := copyFile(filepath.Join(source, path), filepath.Join(root, destination)); err != nil {
			return nil, err
		}
		result = append(result, component{Name: name, Version: resource.Version, Licenses: []string{destination}})
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("browser inventory is empty")
	}
	return result, nil
}

func copyUpstream(source, root string) error {
	var entries []struct{ Path, URL, SHA256 string }
	data, err := os.ReadFile(filepath.Join(source, "licenses/oci/sources.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}
	var assets struct {
		AssetsRevision string `json:"assetsRevision"`
	}
	data, err = os.ReadFile(filepath.Join(source, "araihu-assets.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &assets); err != nil {
		return err
	}
	for _, entry := range entries {
		if !fs.ValidPath(entry.Path) || !strings.HasPrefix(entry.URL, "https://") {
			return fmt.Errorf("invalid upstream source")
		}
		if entry.Path == "assets-LICENSE" || entry.Path == "assets-NOTICE" {
			url := "https://raw.githubusercontent.com/araihu/assets/" + assets.AssetsRevision + "/" + strings.TrimPrefix(entry.Path, "assets-")
			if entry.URL != url {
				return fmt.Errorf("asset license source differs from pinned asset revision")
			}
		}
		path := filepath.Join(source, "licenses/oci", entry.Path)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hash := sha256.Sum256(data)
		if hex.EncodeToString(hash[:]) != entry.SHA256 {
			return fmt.Errorf("upstream license checksum mismatch: %s", entry.Path)
		}
		if err := copyFile(path, filepath.Join(root, legalDir, "upstream", entry.Path)); err != nil {
			return err
		}
	}
	return copyFile(filepath.Join(source, "licenses/oci/sources.json"), filepath.Join(root, legalDir, "upstream/sources.json"))
}

func copyLegalTree(source, root, prefix string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		name := strings.ToLower(entry.Name())
		if strings.HasSuffix(name, ".go") {
			return nil
		}
		if !strings.HasPrefix(name, "license") && !strings.HasPrefix(name, "notice") && !strings.HasPrefix(name, "copying") && !strings.Contains(name, "_license") && name != "patents" && !strings.HasPrefix(name, "ofl") {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("non-regular legal file: %s", path)
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.ToSlash(filepath.Join(prefix, rel))
		if err := copyFile(path, filepath.Join(root, destination)); err != nil {
			return err
		}
		paths = append(paths, destination)
		return nil
	})
	return paths, err
}

func copyFile(source, destination string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("empty notice: %s", source)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	return os.WriteFile(destination, data, 0644)
}

func verifyRoot(root string) error {
	data, err := os.ReadFile(filepath.Join(root, manifestPath))
	if err != nil {
		return err
	}
	var expected inventory
	if err := json.Unmarshal(data, &expected); err != nil {
		return err
	}
	if expected.SchemaVersion != 1 || len(expected.Components) == 0 {
		return fmt.Errorf("invalid runtime inventory")
	}
	actual, err := distribution.InspectRoot(root, distribution.RootOptions{})
	if err != nil {
		return err
	}
	var files []distribution.FileEvidence
	present := map[string]distribution.FileEvidence{}
	for _, file := range actual.Files {
		present[file.Path] = file
		if file.Path != manifestPath {
			files = append(files, file)
		}
	}
	a, _ := json.Marshal(files)
	b, _ := json.Marshal(expected.Files)
	if !bytes.Equal(a, b) {
		return fmt.Errorf("filesystem differs from packaged inventory")
	}
	for _, name := range []string{
		runtimePath, "etc/ssl/certs/ca-certificates.crt",
		"app/renderer/renderer.yaml", "app/renderer/default-allowlist.json",
		legalDir + "/LICENSE", legalDir + "/NOTICE", legalDir + "/THIRD_PARTY_NOTICES.md",
		legalDir + "/upstream/github-LICENSE.md", legalDir + "/upstream/stripe-LICENSE",
		legalDir + "/upstream/assets-LICENSE", legalDir + "/upstream/assets-NOTICE",
		legalDir + "/upstream/simple-icons-16-LICENSE.md", legalDir + "/upstream/simple-icons-9-LICENSE.md",
		legalDir + "/upstream/twemoji-LICENSE-GRAPHICS", legalDir + "/upstream/ca-certificates-MPL-2.0.txt",
		legalDir + "/provenance/internal/renderer/testdata/kubernetes/LICENSE",
	} {
		if present[name].Size <= 0 {
			return fmt.Errorf("required image material missing: %s", name)
		}
	}
	license, err := os.ReadFile(filepath.Join(root, legalDir, "LICENSE"))
	if err != nil {
		return err
	}
	if !bytes.Contains(license, []byte("Copyright (c) 2026 Arai Hû")) || !bytes.Contains(license, []byte("Permission is hereby granted, free of charge")) {
		return fmt.Errorf("project MIT license missing")
	}
	for _, c := range expected.Components {
		if len(c.Licenses) == 0 {
			return fmt.Errorf("component without license: %s", c.Name)
		}
		for _, path := range c.Licenses {
			if present[path].Size <= 0 {
				return fmt.Errorf("component license missing: %s", path)
			}
		}
	}
	info, err := buildinfo.ReadFile(filepath.Join(root, runtimePath))
	if err != nil {
		return err
	}
	if info.Path != "github.com/araihu/manja/cmd/manja-runtime" || info.GoVersion != expected.GoVersion {
		return fmt.Errorf("unexpected runtime build identity")
	}
	static := false
	for _, setting := range info.Settings {
		if setting.Key == "CGO_ENABLED" && setting.Value == "0" {
			static = true
		}
	}
	if !static {
		return fmt.Errorf("runtime must be built with CGO_ENABLED=0 for scratch")
	}
	for _, mod := range info.Deps {
		found := false
		for _, c := range expected.Components {
			if c.Name == mod.Path && c.Version == mod.Version && c.Sum == mod.Sum && mod.Replace == nil {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("runtime dependency absent from notices: %s", mod.Path)
		}
	}
	return nil
}
