package webassets

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"

	"github.com/araihu/assets/assetmeta"
)

type RootMetadata struct {
	Bundles map[string]BundleMetadata `yaml:"bundles"`
}

type BundleMetadata struct {
	Packages []string `yaml:"packages"`
}

type PackageMetadata struct {
	PackageName string        `yaml:"package_name"`
	SPDX        string        `yaml:"spdx"`
	Homepage    string        `yaml:"homepage"`
	LicenseRef  assetmeta.Ref `yaml:"license_ref"`
}

type DownloadMetadata struct {
	Kind string `yaml:"kind"`
}

type Package struct {
	Resource    string
	Name        string
	Version     string
	SPDX        string
	Homepage    string
	ArchiveURL  string
	ArchiveHash string
	LicenseURL  string
	LicensePath string
	ArchiveRef  assetmeta.Ref
	LicenseRef  assetmeta.Ref
}

type Bundle struct {
	Name     string
	Packages []Package
}

func LoadRepositoryMetadata(repoRoot string) ([]Bundle, error) {
	file, err := os.Open(filepath.Join(repoRoot, "internal", "webassets", "vendor.overlay.yaml"))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return LoadMetadata(file)
}

func LoadMetadata(reader io.Reader) ([]Bundle, error) {
	inventory, err := acquisitionInventory()
	if err != nil {
		return nil, err
	}
	document, err := assetmeta.Load[RootMetadata, PackageMetadata, DownloadMetadata](reader, inventory)
	if err != nil {
		return nil, err
	}
	if len(document.Resources) != len(inventory.Resources()) {
		return nil, fmt.Errorf("resource metadata count = %d, want %d", len(document.Resources), len(inventory.Resources()))
	}

	packages := make(map[string]Package, len(document.Resources))
	npmNames := make(map[string]string, len(document.Resources))
	for _, resource := range inventory.Resources() {
		metadata, ok := document.Resources[resource.Name]
		if !ok {
			return nil, fmt.Errorf("resource %q: metadata is required", resource.Name)
		}
		if !validPackageName(metadata.Metadata.PackageName) {
			return nil, fmt.Errorf("resource %q: invalid package_name %q", resource.Name, metadata.Metadata.PackageName)
		}
		if previous := npmNames[metadata.Metadata.PackageName]; previous != "" {
			return nil, fmt.Errorf("resources %q and %q: duplicate package_name %q", previous, resource.Name, metadata.Metadata.PackageName)
		}
		npmNames[metadata.Metadata.PackageName] = resource.Name
		if metadata.Metadata.SPDX == "" {
			return nil, fmt.Errorf("resource %q: spdx is required", resource.Name)
		}
		parsed, parseErr := url.Parse(metadata.Metadata.Homepage)
		if parseErr != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return nil, fmt.Errorf("resource %q: homepage must be absolute HTTPS", resource.Name)
		}
		archiveRef := assetmeta.Ref{Resource: resource.Name, Download: "archive"}
		licenseRef := metadata.Metadata.LicenseRef
		if err := assetmeta.ValidateRefs(inventory, archiveRef, licenseRef); err != nil {
			return nil, fmt.Errorf("resource %q: %w", resource.Name, err)
		}
		if licenseRef != (assetmeta.Ref{Resource: resource.Name, Download: "license"}) {
			return nil, fmt.Errorf("resource %q: license_ref must select its retained license", resource.Name)
		}
		if len(metadata.Downloads) != 2 || metadata.Downloads["archive"].Metadata.Kind != "npm-archive" || metadata.Downloads["license"].Metadata.Kind != "license" {
			return nil, fmt.Errorf("resource %q: archive/license kind metadata mismatch", resource.Name)
		}
		archive, _ := document.Resolve(archiveRef)
		license, _ := document.Resolve(licenseRef)
		packages[resource.Name] = Package{
			Resource: resource.Name, Name: metadata.Metadata.PackageName, Version: resource.Version,
			SPDX: metadata.Metadata.SPDX, Homepage: metadata.Metadata.Homepage,
			ArchiveURL: archive.Download.URL, ArchiveHash: archive.Download.Hash,
			LicenseURL: license.Download.URL, LicensePath: license.Download.Path,
			ArchiveRef: archiveRef, LicenseRef: licenseRef,
		}
	}

	seen := make(map[string]string, len(packages))
	bundleNames := make([]string, 0, len(document.Metadata.Bundles))
	for name := range document.Metadata.Bundles {
		bundleNames = append(bundleNames, name)
	}
	sort.Strings(bundleNames)
	bundles := make([]Bundle, 0, len(bundleNames))
	for _, name := range bundleNames {
		metadata := document.Metadata.Bundles[name]
		if len(metadata.Packages) == 0 {
			return nil, fmt.Errorf("bundle %q: packages are required", name)
		}
		bundle := Bundle{Name: name, Packages: make([]Package, 0, len(metadata.Packages))}
		for _, resource := range metadata.Packages {
			pkg, ok := packages[resource]
			if !ok {
				return nil, fmt.Errorf("bundle %q: unknown package resource %q", name, resource)
			}
			if previous := seen[resource]; previous != "" {
				return nil, fmt.Errorf("package resource %q appears in bundles %q and %q", resource, previous, name)
			}
			seen[resource] = name
			bundle.Packages = append(bundle.Packages, pkg)
		}
		bundles = append(bundles, bundle)
	}
	if len(seen) != len(packages) {
		return nil, fmt.Errorf("bundle package count = %d, want %d", len(seen), len(packages))
	}
	return bundles, nil
}

func acquisitionInventory() (*assetmeta.Inventory, error) {
	resources := MuambaResources()
	converted := make([]assetmeta.Resource, 0, len(resources))
	for _, resource := range resources {
		item := assetmeta.Resource{Name: resource.Name, Version: resource.Version}
		for _, download := range resource.Downloads {
			item.Downloads = append(item.Downloads, assetmeta.Download{
				Name: download.Name, URL: download.URL, Path: download.Path,
				Integrity: download.Integrity, Hash: download.Hash,
			})
		}
		converted = append(converted, item)
	}
	return assetmeta.NewInventory(converted)
}

func allPackages(bundles []Bundle) []Package {
	var packages []Package
	for _, bundle := range bundles {
		packages = append(packages, bundle.Packages...)
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].Name < packages[j].Name })
	return packages
}
