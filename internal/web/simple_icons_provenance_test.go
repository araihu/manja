package web

import (
	"crypto/sha1" // #nosec G505 -- Git blob identity is defined as SHA-1.
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type simpleIconsProvenance struct {
	SchemaVersion int                      `json:"schemaVersion"`
	Repository    string                   `json:"repository"`
	Version       string                   `json:"version"`
	CommitSHA     string                   `json:"commitSha"`
	CommitTreeSHA string                   `json:"commitTreeSha"`
	GitHub        simpleIconProvenance     `json:"github"`
	Stripe        simpleIconProvenance     `json:"stripe"`
	License       simpleIconsLicenseSource `json:"license"`
}

type simpleIconProvenance struct {
	Name            string               `json:"name"`
	LocalPath       string               `json:"localPath"`
	UpstreamPath    string               `json:"upstreamPath"`
	UpstreamSize    int                  `json:"upstreamSize"`
	UpstreamBlobSHA string               `json:"upstreamGitBlobSha"`
	UpstreamSHA256  string               `json:"upstreamSha256"`
	LocalSize       int                  `json:"localSize"`
	LocalBlobSHA    string               `json:"localGitBlobSha"`
	LocalSHA256     string               `json:"localSha256"`
	Adaptation      simpleIconAdaptation `json:"adaptation"`
}

type simpleIconAdaptation struct {
	SourceViewBox       string                     `json:"sourceViewBox"`
	LocalViewBox        string                     `json:"localViewBox"`
	PathDataComparison  string                     `json:"pathDataComparison"`
	PathDataSHA256      string                     `json:"pathDataSha256"`
	LocalPathTransform  string                     `json:"localPathTransform"`
	LocalPathFill       string                     `json:"localPathFill"`
	LocalBackgroundMark simpleIconBackgroundCircle `json:"localBackgroundCircle"`
}

type simpleIconBackgroundCircle struct {
	CX   string `json:"cx"`
	CY   string `json:"cy"`
	R    string `json:"r"`
	Fill string `json:"fill"`
}

type simpleIconsLicenseSource struct {
	Name         string `json:"name"`
	SPDX         string `json:"spdx"`
	UpstreamPath string `json:"upstreamPath"`
	Size         int    `json:"size"`
	GitBlobSHA   string `json:"gitBlobSha"`
	SHA256       string `json:"sha256"`
}

type approvedSimpleIcon struct {
	Receipt  simpleIconProvenance
	PathData string
	Title    string
}

var approvedSimpleIconsProvenance = simpleIconsProvenance{
	SchemaVersion: 1,
	Repository:    "https://github.com/simple-icons/simple-icons",
	Version:       "16.28.0",
	CommitSHA:     "fc91ef03ec113d06627b2d47c1f9644ca202b6f9",
	CommitTreeSHA: "4c01339d8cafffdd7a6a59837b2fc0bbc5ad6e92",
	GitHub: simpleIconProvenance{
		Name:            "GitHub",
		LocalPath:       "internal/web/static/github-mark.svg",
		UpstreamPath:    "icons/github.svg",
		UpstreamSize:    822,
		UpstreamBlobSHA: "538ec5bf2a9a5724899daf728577cd0b8beaae90",
		UpstreamSHA256:  "3bf8cceead820aec50d4ee825a3fd02c5a1cd6665cc9cf4cbf3d9c8861a204bb",
		LocalSize:       1025,
		LocalBlobSHA:    "01c98d26266049dee2c5e609f21ebd6019871c90",
		LocalSHA256:     "792df8d190df379b0a81e35d95b6ca107653bd59b012e25e9b9f67f5c6adfb21",
		Adaptation: simpleIconAdaptation{
			SourceViewBox:      "0 0 24 24",
			LocalViewBox:       "0 0 64 64",
			PathDataComparison: "exact",
			PathDataSHA256:     "d82e21f6c9bfbfd889fed4b8d8604121be1d364ef75b7fe42cc9c0b8737ae529",
			LocalPathTransform: "translate(14 14) scale(1.5)",
			LocalPathFill:      "#fff",
			LocalBackgroundMark: simpleIconBackgroundCircle{
				CX:   "32",
				CY:   "32",
				R:    "32",
				Fill: "#24292f",
			},
		},
	},
	Stripe: simpleIconProvenance{
		Name:            "Stripe",
		LocalPath:       "internal/web/static/stripe-mark.svg",
		UpstreamPath:    "icons/stripe.svg",
		UpstreamSize:    588,
		UpstreamBlobSHA: "8ebadf74d367a5a9bd7deb45a53f1844fc08a095",
		UpstreamSHA256:  "130c6d957b8977f5eda2928267b9df531ca038a400a801765d263801bb1bd870",
		LocalSize:       791,
		LocalBlobSHA:    "60ed70bccc80a4855d082890d5b24f6a544a6602",
		LocalSHA256:     "1952f7e5560c4b818dd640c5ee17dc05623aec45747cce5396516c7537a37790",
		Adaptation: simpleIconAdaptation{
			SourceViewBox:      "0 0 24 24",
			LocalViewBox:       "0 0 64 64",
			PathDataComparison: "exact",
			PathDataSHA256:     "e3b2e90079bcdca94a620b4a871c4ad4f39a448b23b25066340531ce3d701d71",
			LocalPathTransform: "translate(14 14) scale(1.5)",
			LocalPathFill:      "#fff",
			LocalBackgroundMark: simpleIconBackgroundCircle{
				CX:   "32",
				CY:   "32",
				R:    "32",
				Fill: "#635bff",
			},
		},
	},
	License: simpleIconsLicenseSource{
		Name:         "CC0 1.0 Universal",
		SPDX:         "CC0-1.0",
		UpstreamPath: "LICENSE.md",
		Size:         6569,
		GitBlobSHA:   "70d4a7b6740c5d9b594ff2fc27d3ea7e89413185",
		SHA256:       "9046848b63a5c92bff14e4accca80bd987e0623b74adf9226ce5198d312b79d5",
	},
}

var approvedSimpleIcons = map[string]approvedSimpleIcon{
	"github": {
		Receipt:  approvedSimpleIconsProvenance.GitHub,
		PathData: "M12 .297c-6.63 0-12 5.373-12 12 0 5.303 3.438 9.8 8.205 11.385.6.113.82-.258.82-.577 0-.285-.01-1.04-.015-2.04-3.338.724-4.042-1.61-4.042-1.61C4.422 18.07 3.633 17.7 3.633 17.7c-1.087-.744.084-.729.084-.729 1.205.084 1.838 1.236 1.838 1.236 1.07 1.835 2.809 1.305 3.495.998.108-.776.417-1.305.76-1.605-2.665-.3-5.466-1.332-5.466-5.93 0-1.31.465-2.38 1.235-3.22-.135-.303-.54-1.523.105-3.176 0 0 1.005-.322 3.3 1.23.96-.267 1.98-.399 3-.405 1.02.006 2.04.138 3 .405 2.28-1.552 3.285-1.23 3.285-1.23.645 1.653.24 2.873.12 3.176.765.84 1.23 1.91 1.23 3.22 0 4.61-2.805 5.625-5.475 5.92.42.36.81 1.096.81 2.22 0 1.606-.015 2.896-.015 3.286 0 .315.21.69.825.57C20.565 22.092 24 17.592 24 12.297c0-6.627-5.373-12-12-12",
		Title:    "GitHub",
	},
	"stripe": {
		Receipt:  approvedSimpleIconsProvenance.Stripe,
		PathData: "M13.976 9.15c-2.172-.806-3.356-1.426-3.356-2.409 0-.831.683-1.305 1.901-1.305 2.227 0 4.515.858 6.09 1.631l.89-5.494C18.252.975 15.697 0 12.165 0 9.667 0 7.589.654 6.104 1.872 4.56 3.147 3.757 4.992 3.757 7.218c0 4.039 2.467 5.76 6.476 7.219 2.585.92 3.445 1.574 3.445 2.583 0 .98-.84 1.545-2.354 1.545-1.875 0-4.965-.921-6.99-2.109l-.9 5.555C5.175 22.99 8.385 24 11.714 24c2.641 0 4.843-.624 6.328-1.813 1.664-1.305 2.525-3.236 2.525-5.732 0-4.128-2.524-5.851-6.594-7.305h.003z",
		Title:    "Stripe",
	},
}

func TestSimpleIconsMarksMatchImmutableProvenanceReceipt(t *testing.T) {
	t.Parallel()

	receiptBytes, err := os.ReadFile(filepath.Join("static", "simple-icons.provenance.json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt simpleIconsProvenance
	if err := json.Unmarshal(receiptBytes, &receipt); err != nil {
		t.Fatal(err)
	}
	marks := readSimpleIconMarks(t)
	if err := validateSimpleIconsProvenance(receipt, marks); err != nil {
		t.Fatal(err)
	}
}

func TestSimpleIconsProvenanceRejectsControlledDrift(t *testing.T) {
	t.Parallel()

	approved := approvedSimpleIconsProvenance
	marks := readSimpleIconMarks(t)
	mutations := []struct {
		name   string
		mutate func(*simpleIconsProvenance)
	}{
		{name: "schema version", mutate: func(got *simpleIconsProvenance) { got.SchemaVersion++ }},
		{name: "repository", mutate: func(got *simpleIconsProvenance) { got.Repository += "-fork" }},
		{name: "version", mutate: func(got *simpleIconsProvenance) { got.Version = "16.28.1" }},
		{name: "commit SHA", mutate: func(got *simpleIconsProvenance) { got.CommitSHA = strings.Repeat("a", 40) }},
		{name: "commit tree SHA", mutate: func(got *simpleIconsProvenance) { got.CommitTreeSHA = strings.Repeat("b", 40) }},
		{name: "GitHub name", mutate: func(got *simpleIconsProvenance) { got.GitHub.Name += " changed" }},
		{name: "GitHub local path", mutate: func(got *simpleIconsProvenance) { got.GitHub.LocalPath += ".changed" }},
		{name: "GitHub upstream path", mutate: func(got *simpleIconsProvenance) { got.GitHub.UpstreamPath += ".changed" }},
		{name: "GitHub upstream size", mutate: func(got *simpleIconsProvenance) { got.GitHub.UpstreamSize++ }},
		{name: "GitHub upstream Git blob SHA", mutate: func(got *simpleIconsProvenance) { got.GitHub.UpstreamBlobSHA = strings.Repeat("c", 40) }},
		{name: "GitHub upstream SHA-256", mutate: func(got *simpleIconsProvenance) { got.GitHub.UpstreamSHA256 = strings.Repeat("d", 64) }},
		{name: "GitHub local size", mutate: func(got *simpleIconsProvenance) { got.GitHub.LocalSize++ }},
		{name: "GitHub local Git blob SHA", mutate: func(got *simpleIconsProvenance) { got.GitHub.LocalBlobSHA = strings.Repeat("e", 40) }},
		{name: "GitHub local SHA-256", mutate: func(got *simpleIconsProvenance) { got.GitHub.LocalSHA256 = strings.Repeat("f", 64) }},
		{name: "GitHub source viewBox", mutate: func(got *simpleIconsProvenance) { got.GitHub.Adaptation.SourceViewBox = "0 0 32 32" }},
		{name: "GitHub local viewBox", mutate: func(got *simpleIconsProvenance) { got.GitHub.Adaptation.LocalViewBox = "0 0 32 32" }},
		{name: "GitHub path comparison", mutate: func(got *simpleIconsProvenance) { got.GitHub.Adaptation.PathDataComparison = "changed" }},
		{name: "GitHub path SHA-256", mutate: func(got *simpleIconsProvenance) { got.GitHub.Adaptation.PathDataSHA256 = strings.Repeat("1", 64) }},
		{name: "GitHub transform", mutate: func(got *simpleIconsProvenance) { got.GitHub.Adaptation.LocalPathTransform += " changed" }},
		{name: "GitHub path fill", mutate: func(got *simpleIconsProvenance) { got.GitHub.Adaptation.LocalPathFill = "#000" }},
		{name: "GitHub circle cx", mutate: func(got *simpleIconsProvenance) { got.GitHub.Adaptation.LocalBackgroundMark.CX = "31" }},
		{name: "GitHub circle cy", mutate: func(got *simpleIconsProvenance) { got.GitHub.Adaptation.LocalBackgroundMark.CY = "31" }},
		{name: "GitHub circle radius", mutate: func(got *simpleIconsProvenance) { got.GitHub.Adaptation.LocalBackgroundMark.R = "31" }},
		{name: "GitHub circle fill", mutate: func(got *simpleIconsProvenance) { got.GitHub.Adaptation.LocalBackgroundMark.Fill = "#000" }},
		{name: "Stripe name", mutate: func(got *simpleIconsProvenance) { got.Stripe.Name += " changed" }},
		{name: "Stripe local path", mutate: func(got *simpleIconsProvenance) { got.Stripe.LocalPath += ".changed" }},
		{name: "Stripe upstream path", mutate: func(got *simpleIconsProvenance) { got.Stripe.UpstreamPath += ".changed" }},
		{name: "Stripe upstream size", mutate: func(got *simpleIconsProvenance) { got.Stripe.UpstreamSize++ }},
		{name: "Stripe upstream Git blob SHA", mutate: func(got *simpleIconsProvenance) { got.Stripe.UpstreamBlobSHA = strings.Repeat("2", 40) }},
		{name: "Stripe upstream SHA-256", mutate: func(got *simpleIconsProvenance) { got.Stripe.UpstreamSHA256 = strings.Repeat("3", 64) }},
		{name: "Stripe local size", mutate: func(got *simpleIconsProvenance) { got.Stripe.LocalSize++ }},
		{name: "Stripe local Git blob SHA", mutate: func(got *simpleIconsProvenance) { got.Stripe.LocalBlobSHA = strings.Repeat("4", 40) }},
		{name: "Stripe local SHA-256", mutate: func(got *simpleIconsProvenance) { got.Stripe.LocalSHA256 = strings.Repeat("5", 64) }},
		{name: "Stripe source viewBox", mutate: func(got *simpleIconsProvenance) { got.Stripe.Adaptation.SourceViewBox = "0 0 32 32" }},
		{name: "Stripe local viewBox", mutate: func(got *simpleIconsProvenance) { got.Stripe.Adaptation.LocalViewBox = "0 0 32 32" }},
		{name: "Stripe path comparison", mutate: func(got *simpleIconsProvenance) { got.Stripe.Adaptation.PathDataComparison = "changed" }},
		{name: "Stripe path SHA-256", mutate: func(got *simpleIconsProvenance) { got.Stripe.Adaptation.PathDataSHA256 = strings.Repeat("6", 64) }},
		{name: "Stripe transform", mutate: func(got *simpleIconsProvenance) { got.Stripe.Adaptation.LocalPathTransform += " changed" }},
		{name: "Stripe path fill", mutate: func(got *simpleIconsProvenance) { got.Stripe.Adaptation.LocalPathFill = "#000" }},
		{name: "Stripe circle cx", mutate: func(got *simpleIconsProvenance) { got.Stripe.Adaptation.LocalBackgroundMark.CX = "31" }},
		{name: "Stripe circle cy", mutate: func(got *simpleIconsProvenance) { got.Stripe.Adaptation.LocalBackgroundMark.CY = "31" }},
		{name: "Stripe circle radius", mutate: func(got *simpleIconsProvenance) { got.Stripe.Adaptation.LocalBackgroundMark.R = "31" }},
		{name: "Stripe circle fill", mutate: func(got *simpleIconsProvenance) { got.Stripe.Adaptation.LocalBackgroundMark.Fill = "#000" }},
		{name: "license name", mutate: func(got *simpleIconsProvenance) { got.License.Name = "changed" }},
		{name: "license SPDX", mutate: func(got *simpleIconsProvenance) { got.License.SPDX = "changed" }},
		{name: "license upstream path", mutate: func(got *simpleIconsProvenance) { got.License.UpstreamPath = "LICENSE" }},
		{name: "license size", mutate: func(got *simpleIconsProvenance) { got.License.Size++ }},
		{name: "license Git blob SHA", mutate: func(got *simpleIconsProvenance) { got.License.GitBlobSHA = strings.Repeat("7", 40) }},
		{name: "license SHA-256", mutate: func(got *simpleIconsProvenance) { got.License.SHA256 = strings.Repeat("8", 64) }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			changed := approved
			mutation.mutate(&changed)
			if err := validateSimpleIconsProvenance(changed, marks); err == nil {
				t.Fatal("controlled receipt drift was accepted")
			}
		})
	}

	t.Run("coordinated local bytes and receipt digest drift", func(t *testing.T) {
		changedMarks := cloneSimpleIconMarks(marks)
		changedBytes := append(append([]byte(nil), changedMarks[approved.GitHub.LocalPath]...), '\n')
		changedMarks[approved.GitHub.LocalPath] = changedBytes
		changed := approved
		changed.GitHub.LocalSize = len(changedBytes)
		changed.GitHub.LocalBlobSHA = simpleIconsGitBlobSHA(changedBytes)
		changed.GitHub.LocalSHA256 = simpleIconsSHA256Hex(changedBytes)
		if err := validateSimpleIconsProvenance(changed, changedMarks); err == nil {
			t.Fatal("coordinated local bytes and receipt digest drift was accepted")
		}
	})

	t.Run("coordinated local path and receipt evidence drift", func(t *testing.T) {
		changedMarks := cloneSimpleIconMarks(marks)
		changedPath := strings.Replace(approvedSimpleIcons["stripe"].PathData, "M13.976", "M13.975", 1)
		changedBytes := []byte(strings.Replace(string(changedMarks[approved.Stripe.LocalPath]), approvedSimpleIcons["stripe"].PathData, changedPath, 1))
		changedMarks[approved.Stripe.LocalPath] = changedBytes
		changed := approved
		changed.Stripe.LocalSize = len(changedBytes)
		changed.Stripe.LocalBlobSHA = simpleIconsGitBlobSHA(changedBytes)
		changed.Stripe.LocalSHA256 = simpleIconsSHA256Hex(changedBytes)
		changed.Stripe.Adaptation.PathDataSHA256 = simpleIconsSHA256Hex([]byte(changedPath))
		if err := validateSimpleIconsProvenance(changed, changedMarks); err == nil {
			t.Fatal("coordinated local path and receipt evidence drift was accepted")
		}
	})
}

func readSimpleIconMarks(t *testing.T) map[string][]byte {
	t.Helper()

	root := filepath.Join("..", "..")
	marks := make(map[string][]byte, 2)
	for _, icon := range []simpleIconProvenance{approvedSimpleIconsProvenance.GitHub, approvedSimpleIconsProvenance.Stripe} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(icon.LocalPath)))
		if err != nil {
			t.Fatal(err)
		}
		marks[icon.LocalPath] = data
	}
	return marks
}

func cloneSimpleIconMarks(marks map[string][]byte) map[string][]byte {
	cloned := make(map[string][]byte, len(marks))
	for name, data := range marks {
		cloned[name] = append([]byte(nil), data...)
	}
	return cloned
}

func validateSimpleIconsProvenance(receipt simpleIconsProvenance, marks map[string][]byte) error {
	approved := approvedSimpleIconsProvenance
	if receipt.SchemaVersion != approved.SchemaVersion {
		return fmt.Errorf("receipt schema version = %d, want %d", receipt.SchemaVersion, approved.SchemaVersion)
	}
	if receipt.Repository != approved.Repository {
		return fmt.Errorf("receipt repository = %q, want %q", receipt.Repository, approved.Repository)
	}
	if receipt.Version != approved.Version {
		return fmt.Errorf("receipt version = %q, want %q", receipt.Version, approved.Version)
	}
	if receipt.CommitSHA != approved.CommitSHA {
		return fmt.Errorf("receipt commit SHA = %q, want %q", receipt.CommitSHA, approved.CommitSHA)
	}
	if receipt.CommitTreeSHA != approved.CommitTreeSHA {
		return fmt.Errorf("receipt commit tree SHA = %q, want %q", receipt.CommitTreeSHA, approved.CommitTreeSHA)
	}
	if err := validateSimpleIconReceipt("GitHub", receipt.GitHub, approved.GitHub); err != nil {
		return err
	}
	if err := validateSimpleIconReceipt("Stripe", receipt.Stripe, approved.Stripe); err != nil {
		return err
	}
	if receipt.License.Name != approved.License.Name {
		return fmt.Errorf("receipt license name = %q, want %q", receipt.License.Name, approved.License.Name)
	}
	if receipt.License.SPDX != approved.License.SPDX {
		return fmt.Errorf("receipt license SPDX = %q, want %q", receipt.License.SPDX, approved.License.SPDX)
	}
	if receipt.License.UpstreamPath != approved.License.UpstreamPath {
		return fmt.Errorf("receipt license path = %q, want %q", receipt.License.UpstreamPath, approved.License.UpstreamPath)
	}
	if receipt.License.Size != approved.License.Size {
		return fmt.Errorf("receipt license size = %d, want %d", receipt.License.Size, approved.License.Size)
	}
	if receipt.License.GitBlobSHA != approved.License.GitBlobSHA {
		return fmt.Errorf("receipt license Git blob SHA = %q, want %q", receipt.License.GitBlobSHA, approved.License.GitBlobSHA)
	}
	if receipt.License.SHA256 != approved.License.SHA256 {
		return fmt.Errorf("receipt license SHA-256 = %q, want %q", receipt.License.SHA256, approved.License.SHA256)
	}
	if err := validateSimpleIconMark(approvedSimpleIcons["github"], marks[approved.GitHub.LocalPath]); err != nil {
		return err
	}
	return validateSimpleIconMark(approvedSimpleIcons["stripe"], marks[approved.Stripe.LocalPath])
}

func validateSimpleIconReceipt(name string, got, want simpleIconProvenance) error {
	if got.Name != want.Name {
		return fmt.Errorf("%s receipt name = %q, want %q", name, got.Name, want.Name)
	}
	if got.LocalPath != want.LocalPath {
		return fmt.Errorf("%s receipt local path = %q, want %q", name, got.LocalPath, want.LocalPath)
	}
	if got.UpstreamPath != want.UpstreamPath {
		return fmt.Errorf("%s receipt upstream path = %q, want %q", name, got.UpstreamPath, want.UpstreamPath)
	}
	if got.UpstreamSize != want.UpstreamSize {
		return fmt.Errorf("%s receipt upstream size = %d, want %d", name, got.UpstreamSize, want.UpstreamSize)
	}
	if got.UpstreamBlobSHA != want.UpstreamBlobSHA {
		return fmt.Errorf("%s receipt upstream Git blob SHA = %q, want %q", name, got.UpstreamBlobSHA, want.UpstreamBlobSHA)
	}
	if got.UpstreamSHA256 != want.UpstreamSHA256 {
		return fmt.Errorf("%s receipt upstream SHA-256 = %q, want %q", name, got.UpstreamSHA256, want.UpstreamSHA256)
	}
	if got.LocalSize != want.LocalSize {
		return fmt.Errorf("%s receipt local size = %d, want %d", name, got.LocalSize, want.LocalSize)
	}
	if got.LocalBlobSHA != want.LocalBlobSHA {
		return fmt.Errorf("%s receipt local Git blob SHA = %q, want %q", name, got.LocalBlobSHA, want.LocalBlobSHA)
	}
	if got.LocalSHA256 != want.LocalSHA256 {
		return fmt.Errorf("%s receipt local SHA-256 = %q, want %q", name, got.LocalSHA256, want.LocalSHA256)
	}
	if got.Adaptation.SourceViewBox != want.Adaptation.SourceViewBox {
		return fmt.Errorf("%s receipt source viewBox = %q, want %q", name, got.Adaptation.SourceViewBox, want.Adaptation.SourceViewBox)
	}
	if got.Adaptation.LocalViewBox != want.Adaptation.LocalViewBox {
		return fmt.Errorf("%s receipt local viewBox = %q, want %q", name, got.Adaptation.LocalViewBox, want.Adaptation.LocalViewBox)
	}
	if got.Adaptation.PathDataComparison != want.Adaptation.PathDataComparison {
		return fmt.Errorf("%s receipt path-data comparison = %q, want %q", name, got.Adaptation.PathDataComparison, want.Adaptation.PathDataComparison)
	}
	if got.Adaptation.PathDataSHA256 != want.Adaptation.PathDataSHA256 {
		return fmt.Errorf("%s receipt path-data SHA-256 = %q, want %q", name, got.Adaptation.PathDataSHA256, want.Adaptation.PathDataSHA256)
	}
	if got.Adaptation.LocalPathTransform != want.Adaptation.LocalPathTransform {
		return fmt.Errorf("%s receipt local path transform = %q, want %q", name, got.Adaptation.LocalPathTransform, want.Adaptation.LocalPathTransform)
	}
	if got.Adaptation.LocalPathFill != want.Adaptation.LocalPathFill {
		return fmt.Errorf("%s receipt local path fill = %q, want %q", name, got.Adaptation.LocalPathFill, want.Adaptation.LocalPathFill)
	}
	if got.Adaptation.LocalBackgroundMark != want.Adaptation.LocalBackgroundMark {
		return fmt.Errorf("%s receipt local background circle = %#v, want %#v", name, got.Adaptation.LocalBackgroundMark, want.Adaptation.LocalBackgroundMark)
	}
	return nil
}

type simpleIconSVG struct {
	XMLName xml.Name `xml:"svg"`
	ViewBox string   `xml:"viewBox,attr"`
	Title   struct {
		Text string `xml:",chardata"`
	} `xml:"title"`
	Circle struct {
		CX   string `xml:"cx,attr"`
		CY   string `xml:"cy,attr"`
		R    string `xml:"r,attr"`
		Fill string `xml:"fill,attr"`
	} `xml:"circle"`
	Path struct {
		Fill      string `xml:"fill,attr"`
		Transform string `xml:"transform,attr"`
		D         string `xml:"d,attr"`
	} `xml:"path"`
}

func validateSimpleIconMark(approved approvedSimpleIcon, data []byte) error {
	want := approved.Receipt
	if len(data) != want.LocalSize {
		return fmt.Errorf("%s local size = %d, want %d", want.Name, len(data), want.LocalSize)
	}
	if got := simpleIconsGitBlobSHA(data); got != want.LocalBlobSHA {
		return fmt.Errorf("%s local Git blob SHA = %s, want %s", want.Name, got, want.LocalBlobSHA)
	}
	if got := simpleIconsSHA256Hex(data); got != want.LocalSHA256 {
		return fmt.Errorf("%s local SHA-256 = %s, want %s", want.Name, got, want.LocalSHA256)
	}
	var mark simpleIconSVG
	if err := xml.Unmarshal(data, &mark); err != nil {
		return fmt.Errorf("parse %s mark: %w", want.Name, err)
	}
	if mark.XMLName.Local != "svg" || mark.ViewBox != want.Adaptation.LocalViewBox {
		return fmt.Errorf("%s local SVG root/viewBox = %q/%q, want svg/%q", want.Name, mark.XMLName.Local, mark.ViewBox, want.Adaptation.LocalViewBox)
	}
	if strings.TrimSpace(mark.Title.Text) != approved.Title {
		return fmt.Errorf("%s local title = %q, want %q", want.Name, strings.TrimSpace(mark.Title.Text), approved.Title)
	}
	gotCircle := simpleIconBackgroundCircle{CX: mark.Circle.CX, CY: mark.Circle.CY, R: mark.Circle.R, Fill: mark.Circle.Fill}
	if gotCircle != want.Adaptation.LocalBackgroundMark {
		return fmt.Errorf("%s local background circle = %#v, want %#v", want.Name, gotCircle, want.Adaptation.LocalBackgroundMark)
	}
	if mark.Path.Fill != want.Adaptation.LocalPathFill || mark.Path.Transform != want.Adaptation.LocalPathTransform {
		return fmt.Errorf("%s local path fill/transform = %q/%q, want %q/%q", want.Name, mark.Path.Fill, mark.Path.Transform, want.Adaptation.LocalPathFill, want.Adaptation.LocalPathTransform)
	}
	if mark.Path.D != approved.PathData {
		return fmt.Errorf("%s local path data differs from fixed approved upstream path", want.Name)
	}
	if got := simpleIconsSHA256Hex([]byte(mark.Path.D)); got != want.Adaptation.PathDataSHA256 {
		return fmt.Errorf("%s local path-data SHA-256 = %s, want %s", want.Name, got, want.Adaptation.PathDataSHA256)
	}
	return nil
}

func simpleIconsSHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func simpleIconsGitBlobSHA(data []byte) string {
	hash := sha1.New() // #nosec G401 -- Git blob identity is defined as SHA-1.
	_, _ = fmt.Fprintf(hash, "blob %d\x00", len(data))
	_, _ = hash.Write(data)
	return hex.EncodeToString(hash.Sum(nil))
}
