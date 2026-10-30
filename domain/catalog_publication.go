package domain

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// CatalogPublication describes the upstream publication represented by a
// document. It is explicitly supplied by the publisher, never inferred from
// info.version, the bundle filename, or the catalog build revision.
type CatalogPublication struct {
	API           string `json:"api" yaml:"api"`
	Audience      string `json:"audience,omitempty" yaml:"audience"`
	Channel       string `json:"channel" yaml:"channel"`
	Version       string `json:"version,omitempty" yaml:"version"`
	Latest        bool   `json:"latest,omitempty" yaml:"latest"`
	RepositoryURL string `json:"repositoryURL,omitempty" yaml:"repositoryURL"`
	Ref           string `json:"ref" yaml:"ref"`
	Commit        string `json:"commit" yaml:"commit"`
	SourcePath    string `json:"sourcePath" yaml:"sourcePath"`
}

var publicationVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)
var publicationCommitPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func ValidateCatalogPublication(p *CatalogPublication) error {
	if p == nil {
		return nil
	}
	if len(p.Version) > 128 || len(p.RepositoryURL) > 2048 {
		return fmt.Errorf("publication version or repository URL is too long")
	}
	if err := ValidateCatalogDocumentKey(p.API); err != nil {
		return fmt.Errorf("publication API: %w", err)
	}
	for name, value := range map[string]string{"audience": p.Audience, "ref": p.Ref, "source path": p.SourcePath} {
		if name == "audience" && value == "" {
			continue
		}
		if len(value) > 512 {
			return fmt.Errorf("publication %s is too long", name)
		}
		if err := ValidateCanonicalIdentity("publication "+name, value, false); err != nil {
			return err
		}
	}
	if err := validateCatalogSourcePath("publication source path", p.SourcePath); err != nil {
		return err
	}
	if !publicationCommitPattern.MatchString(p.Commit) {
		return fmt.Errorf("publication commit must be a full lowercase Git object ID")
	}
	if strings.ContainsAny(p.Ref, " ~^:?*[\\") || strings.Contains(p.Ref, "..") || strings.HasSuffix(p.Ref, "/") {
		return fmt.Errorf("invalid publication ref")
	}
	switch p.Channel {
	case "next":
		if !strings.HasPrefix(p.Ref, "refs/heads/") || len(p.Ref) == len("refs/heads/") || p.Version != "" || p.Latest {
			return fmt.Errorf("next publication requires a branch and cannot declare a release version or latest")
		}
	case "release":
		parts := publicationVersionPattern.FindStringSubmatch(p.Version)
		if parts == nil || !strings.HasPrefix(p.Ref, "refs/tags/") || len(p.Ref) == len("refs/tags/") {
			return fmt.Errorf("release publication requires a tag and a SemVer version")
		}
		for _, identifier := range strings.Split(parts[4], ".") {
			if numericPublicationIdentifier(identifier) && len(identifier) > 1 && identifier[0] == '0' {
				return fmt.Errorf("release prerelease identifiers cannot have leading zeroes")
			}
		}
		if p.Latest && parts[4] != "" {
			return fmt.Errorf("latest publication must be a stable release")
		}
	default:
		return fmt.Errorf("publication channel must be next or release")
	}
	if p.RepositoryURL != "" {
		u, err := url.Parse(p.RepositoryURL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("publication repositoryURL must be a public HTTPS URL without credentials, query or fragment")
		}
	}
	return nil
}

func CloneCatalogPublication(p *CatalogPublication) *CatalogPublication {
	if p == nil {
		return nil
	}
	copy := *p
	return &copy
}

// ValidateCatalogPublicationSet rejects contradictory identities and aliases.
func ValidateCatalogPublicationSet(publications []*CatalogPublication) error {
	seen := map[string]bool{}
	latest := map[string]*CatalogPublication{}
	for _, p := range publications {
		if err := ValidateCatalogPublication(p); err != nil {
			return err
		}
		if p == nil {
			continue
		}
		identity := p.API + "\x00" + p.Channel + "\x00" + p.Version
		if seen[identity] {
			return fmt.Errorf("duplicate publication for API %q", p.API)
		}
		seen[identity] = true
		if p.Latest {
			if latest[p.API] != nil {
				return fmt.Errorf("multiple latest publications for API %q", p.API)
			}
			latest[p.API] = p
		}
	}
	for _, p := range publications {
		if p != nil && p.Channel == "release" && !strings.Contains(strings.Split(p.Version, "+")[0], "-") {
			if l := latest[p.API]; l != nil && ComparePublicationVersions(p.Version, l.Version) > 0 {
				return fmt.Errorf("latest publication is not the highest stable version for API %q", p.API)
			}
		}
	}
	return nil
}

func (p CatalogPublication) Label(apiVersion string) string {
	if p.Channel == "next" {
		return "next · development · API " + apiVersion
	}
	label := "release " + p.Version
	if strings.Contains(strings.Split(p.Version, "+")[0], "-") {
		label = "pre-release " + p.Version
	}
	if p.Latest {
		label += " · latest stable"
	}
	return label
}

// ComparePublicationVersions compares SemVer precedence without narrowing
// numeric components to machine integers. Build metadata has no precedence.
func ComparePublicationVersions(a, b string) int {
	x, y := publicationVersionPattern.FindStringSubmatch(a), publicationVersionPattern.FindStringSubmatch(b)
	if x == nil || y == nil {
		return strings.Compare(a, b)
	}
	for i := 1; i <= 3; i++ {
		if c := comparePublicationNumber(x[i], y[i]); c != 0 {
			return c
		}
	}
	if x[4] == y[4] {
		return 0
	}
	if x[4] == "" {
		return 1
	}
	if y[4] == "" {
		return -1
	}
	xp, yp := strings.Split(x[4], "."), strings.Split(y[4], ".")
	for i := 0; i < len(xp) && i < len(yp); i++ {
		xn, yn := numericPublicationIdentifier(xp[i]), numericPublicationIdentifier(yp[i])
		if xn != yn {
			if xn {
				return -1
			}
			return 1
		}
		c := strings.Compare(xp[i], yp[i])
		if xn {
			c = comparePublicationNumber(xp[i], yp[i])
		}
		if c != 0 {
			return c
		}
	}
	if len(xp) < len(yp) {
		return -1
	}
	return 1
}
func numericPublicationIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
func comparePublicationNumber(a, b string) int {
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return 1
	}
	return strings.Compare(a, b)
}
