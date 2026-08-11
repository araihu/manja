package web

import (
	"bytes"
	"crypto/sha1" // #nosec G505 -- Git blob identity is defined as SHA-1.
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type manjaSocialProvenance struct {
	SchemaVersion       int                       `json:"schemaVersion"`
	IntroducedCommitSHA string                    `json:"introducedCommitSha"`
	IntroducedTreeSHA   string                    `json:"introducedTreeSha"`
	Source              manjaSocialSourceEvidence `json:"source"`
	Artifact            manjaSocialPNGArtifact    `json:"artifact"`
	Renderer            manjaSocialRenderer       `json:"renderer"`
}

type manjaSocialSourceEvidence struct {
	LocalPath  string `json:"localPath"`
	Size       int    `json:"size"`
	GitBlobSHA string `json:"gitBlobSha"`
	SHA256     string `json:"sha256"`
	MIMEType   string `json:"mimeType"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
}

type manjaSocialPNGArtifact struct {
	LocalPath       string `json:"localPath"`
	Size            int    `json:"size"`
	GitBlobSHA      string `json:"gitBlobSha"`
	SHA256          string `json:"sha256"`
	MIMEType        string `json:"mimeType"`
	Width           int    `json:"width"`
	Height          int    `json:"height"`
	BitDepth        int    `json:"bitDepth"`
	ColorType       int    `json:"colorType"`
	InterlaceMethod int    `json:"interlaceMethod"`
	UnderOneMiB     bool   `json:"underOneMiB"`
}

type manjaSocialRenderer struct {
	Tool                   string `json:"tool"`
	Version                string `json:"version"`
	ExecutableSHA256       string `json:"executableSha256"`
	Options                string `json:"options"`
	CheckCommand           string `json:"checkCommand"`
	OperatingSystem        string `json:"operatingSystem"`
	OperatingSystemVersion string `json:"operatingSystemVersion"`
	OperatingSystemBuild   string `json:"operatingSystemBuild"`
	Architecture           string `json:"architecture"`
	Locale                 string `json:"locale"`
	Libraries              string `json:"libraries"`
	RegularFontPath        string `json:"regularFontPath"`
	RegularFontSize        int    `json:"regularFontSize"`
	RegularFontSHA256      string `json:"regularFontSha256"`
	BoldFontPath           string `json:"boldFontPath"`
	BoldFontSize           int    `json:"boldFontSize"`
	BoldFontSHA256         string `json:"boldFontSha256"`
	ExactRuns              int    `json:"exactRuns"`
}

var approvedManjaSocialProvenance = manjaSocialProvenance{
	SchemaVersion:       1,
	IntroducedCommitSHA: "b2df3a5f0d67c6a04539f96c804b404a5236c1d4",
	IntroducedTreeSHA:   "12b20134b059f2fce38041597f021a36ecd7f61a",
	Source: manjaSocialSourceEvidence{
		LocalPath:  "internal/web/static/manja-social.svg",
		Size:       2198,
		GitBlobSHA: "5cb2fda632e511e4eeccec6412858f6f630bc6c9",
		SHA256:     "002b05823a870f28ff28d12fe0b793cee979418435bb4ff4c4a634affc7b2fe2",
		MIMEType:   "image/svg+xml",
		Width:      1280,
		Height:     640,
	},
	Artifact: manjaSocialPNGArtifact{
		LocalPath:       "internal/web/static/manja-social.png",
		Size:            21500,
		GitBlobSHA:      "9260d190361cceeef611f3a2178f14c613b0f533",
		SHA256:          "7234c9a20fc3a4a44364b8f9d544ddae5aba8c2b6a418b26ad5a930d2d0ab0bd",
		MIMEType:        "image/png",
		Width:           1280,
		Height:          640,
		BitDepth:        8,
		ColorType:       2,
		InterlaceMethod: 0,
		UnderOneMiB:     true,
	},
	Renderer: manjaSocialRenderer{
		Tool:                   "rsvg-convert",
		Version:                "2.62.1",
		ExecutableSHA256:       "f9a4aa5e7d66f8e61ebbc5f7df3f0966d7d9a5c43d1a691969092c86ebebeb2e",
		Options:                "--format=png --width=1280 --height=640",
		CheckCommand:           "LC_ALL=C.UTF-8 rsvg-convert --format=png --width=1280 --height=640 internal/web/static/manja-social.svg | cmp - internal/web/static/manja-social.png",
		OperatingSystem:        "macOS",
		OperatingSystemVersion: "26.5.2",
		OperatingSystemBuild:   "25F84",
		Architecture:           "arm64",
		Locale:                 "C.UTF-8",
		Libraries:              "cairo 1.18.4; fontconfig 2.18.2; freetype 2.14.3; glib 2.88.3; harfbuzz 14.3.0; libpng 1.6.58; pango 1.58.0",
		RegularFontPath:        "/System/Library/Fonts/Supplemental/Arial.ttf",
		RegularFontSize:        773236,
		RegularFontSHA256:      "525979822591a3447cfc49d943d6f7683508e25543407871c0ed8fed05fd2bd9",
		BoldFontPath:           "/System/Library/Fonts/Supplemental/Arial Bold.ttf",
		BoldFontSize:           750984,
		BoldFontSHA256:         "d72db21f9242aedd6b917d8549ad5921766b24d5f8d0becfda2ff4c620b3c2e0",
		ExactRuns:              2,
	},
}

func TestManjaSocialPreviewMatchesReproductionReceipt(t *testing.T) {
	t.Parallel()

	receiptBytes, err := os.ReadFile(filepath.Join("static", "manja-social.provenance.json"))
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := decodeManjaSocialProvenance(receiptBytes)
	if err != nil {
		t.Fatal(err)
	}
	source, artifact := readManjaSocialAssets(t)
	if err := validateManjaSocialProvenance(receipt, source, artifact); err != nil {
		t.Fatal(err)
	}
}

func decodeManjaSocialProvenance(data []byte) (manjaSocialProvenance, error) {
	var receipt manjaSocialProvenance
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil {
		return receipt, fmt.Errorf("decode receipt: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return receipt, fmt.Errorf("decode receipt: trailing JSON value")
		}
		return receipt, fmt.Errorf("decode receipt trailing data: %w", err)
	}
	return receipt, nil
}

func TestManjaSocialPreviewRejectsMalformedReceiptJSON(t *testing.T) {
	t.Parallel()

	baseline, err := os.ReadFile("static/manja-social.provenance.json")
	if err != nil {
		t.Fatal(err)
	}
	topLevelUnknown := bytes.Replace(
		baseline,
		[]byte("{"),
		[]byte("{\n  \"unexpected\": true,"),
		1,
	)
	nestedUnknown := bytes.Replace(
		baseline,
		[]byte("\"source\": {"),
		[]byte("\"source\": {\n    \"unexpected\": true,"),
		1,
	)
	trailingValue := append(append([]byte(nil), baseline...), []byte("\n{}\n")...)

	tests := []struct {
		name      string
		data      []byte
		wantValid bool
	}{
		{name: "committed baseline", data: baseline, wantValid: true},
		{name: "top-level unknown field", data: topLevelUnknown},
		{name: "nested unknown field", data: nestedUnknown},
		{name: "trailing second value", data: trailingValue},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodeManjaSocialProvenance(test.data)
			if test.wantValid && err != nil {
				t.Fatalf("committed receipt rejected: %v", err)
			}
			if !test.wantValid && err == nil {
				t.Fatal("malformed receipt accepted")
			}
		})
	}
}

func TestManjaSocialPreviewRejectsControlledDrift(t *testing.T) {
	t.Parallel()

	approved := approvedManjaSocialProvenance
	source, artifact := readManjaSocialAssets(t)
	mutations := []struct {
		name   string
		mutate func(*manjaSocialProvenance)
	}{
		{name: "schema version", mutate: func(got *manjaSocialProvenance) { got.SchemaVersion++ }},
		{name: "introduced commit", mutate: func(got *manjaSocialProvenance) { got.IntroducedCommitSHA = strings.Repeat("a", 40) }},
		{name: "introduced tree", mutate: func(got *manjaSocialProvenance) { got.IntroducedTreeSHA = strings.Repeat("b", 40) }},
		{name: "source path", mutate: func(got *manjaSocialProvenance) { got.Source.LocalPath += ".changed" }},
		{name: "source size", mutate: func(got *manjaSocialProvenance) { got.Source.Size++ }},
		{name: "source Git blob", mutate: func(got *manjaSocialProvenance) { got.Source.GitBlobSHA = strings.Repeat("c", 40) }},
		{name: "source SHA-256", mutate: func(got *manjaSocialProvenance) { got.Source.SHA256 = strings.Repeat("d", 64) }},
		{name: "source MIME", mutate: func(got *manjaSocialProvenance) { got.Source.MIMEType = "changed" }},
		{name: "source width", mutate: func(got *manjaSocialProvenance) { got.Source.Width++ }},
		{name: "source height", mutate: func(got *manjaSocialProvenance) { got.Source.Height++ }},
		{name: "artifact path", mutate: func(got *manjaSocialProvenance) { got.Artifact.LocalPath += ".changed" }},
		{name: "artifact size", mutate: func(got *manjaSocialProvenance) { got.Artifact.Size++ }},
		{name: "artifact Git blob", mutate: func(got *manjaSocialProvenance) { got.Artifact.GitBlobSHA = strings.Repeat("e", 40) }},
		{name: "artifact SHA-256", mutate: func(got *manjaSocialProvenance) { got.Artifact.SHA256 = strings.Repeat("f", 64) }},
		{name: "artifact MIME", mutate: func(got *manjaSocialProvenance) { got.Artifact.MIMEType = "changed" }},
		{name: "artifact width", mutate: func(got *manjaSocialProvenance) { got.Artifact.Width++ }},
		{name: "artifact height", mutate: func(got *manjaSocialProvenance) { got.Artifact.Height++ }},
		{name: "artifact bit depth", mutate: func(got *manjaSocialProvenance) { got.Artifact.BitDepth++ }},
		{name: "artifact color type", mutate: func(got *manjaSocialProvenance) { got.Artifact.ColorType++ }},
		{name: "artifact interlace", mutate: func(got *manjaSocialProvenance) { got.Artifact.InterlaceMethod++ }},
		{name: "artifact size gate", mutate: func(got *manjaSocialProvenance) { got.Artifact.UnderOneMiB = false }},
		{name: "renderer tool", mutate: func(got *manjaSocialProvenance) { got.Renderer.Tool = "changed" }},
		{name: "renderer version", mutate: func(got *manjaSocialProvenance) { got.Renderer.Version = "changed" }},
		{name: "renderer executable", mutate: func(got *manjaSocialProvenance) { got.Renderer.ExecutableSHA256 = strings.Repeat("1", 64) }},
		{name: "renderer options", mutate: func(got *manjaSocialProvenance) { got.Renderer.Options += " changed" }},
		{name: "renderer check command", mutate: func(got *manjaSocialProvenance) { got.Renderer.CheckCommand += " changed" }},
		{name: "renderer OS", mutate: func(got *manjaSocialProvenance) { got.Renderer.OperatingSystem = "changed" }},
		{name: "renderer OS version", mutate: func(got *manjaSocialProvenance) { got.Renderer.OperatingSystemVersion = "changed" }},
		{name: "renderer OS build", mutate: func(got *manjaSocialProvenance) { got.Renderer.OperatingSystemBuild = "changed" }},
		{name: "renderer architecture", mutate: func(got *manjaSocialProvenance) { got.Renderer.Architecture = "changed" }},
		{name: "renderer locale", mutate: func(got *manjaSocialProvenance) { got.Renderer.Locale = "changed" }},
		{name: "renderer libraries", mutate: func(got *manjaSocialProvenance) { got.Renderer.Libraries += "; changed" }},
		{name: "regular font path", mutate: func(got *manjaSocialProvenance) { got.Renderer.RegularFontPath += ".changed" }},
		{name: "regular font size", mutate: func(got *manjaSocialProvenance) { got.Renderer.RegularFontSize++ }},
		{name: "regular font SHA-256", mutate: func(got *manjaSocialProvenance) { got.Renderer.RegularFontSHA256 = strings.Repeat("2", 64) }},
		{name: "bold font path", mutate: func(got *manjaSocialProvenance) { got.Renderer.BoldFontPath += ".changed" }},
		{name: "bold font size", mutate: func(got *manjaSocialProvenance) { got.Renderer.BoldFontSize++ }},
		{name: "bold font SHA-256", mutate: func(got *manjaSocialProvenance) { got.Renderer.BoldFontSHA256 = strings.Repeat("3", 64) }},
		{name: "exact runs", mutate: func(got *manjaSocialProvenance) { got.Renderer.ExactRuns++ }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			changed := approved
			mutation.mutate(&changed)
			if err := validateManjaSocialProvenance(changed, source, artifact); err == nil {
				t.Fatal("controlled receipt drift was accepted")
			}
		})
	}

	t.Run("coordinated source and receipt drift", func(t *testing.T) {
		changedSource := append(append([]byte(nil), source...), '\n')
		changed := approved
		changed.Source.Size = len(changedSource)
		changed.Source.GitBlobSHA = manjaSocialGitBlobSHA(changedSource)
		changed.Source.SHA256 = manjaSocialSHA256Hex(changedSource)
		if err := validateManjaSocialProvenance(changed, changedSource, artifact); err == nil {
			t.Fatal("coordinated source and receipt drift was accepted")
		}
	})

	t.Run("coordinated PNG and receipt drift", func(t *testing.T) {
		changedArtifact := append(append([]byte(nil), artifact...), '\n')
		changed := approved
		changed.Artifact.Size = len(changedArtifact)
		changed.Artifact.GitBlobSHA = manjaSocialGitBlobSHA(changedArtifact)
		changed.Artifact.SHA256 = manjaSocialSHA256Hex(changedArtifact)
		if err := validateManjaSocialProvenance(changed, source, changedArtifact); err == nil {
			t.Fatal("coordinated PNG and receipt drift was accepted")
		}
	})
}

func readManjaSocialAssets(t *testing.T) ([]byte, []byte) {
	t.Helper()

	read := func(localPath string) []byte {
		data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(localPath)))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	return read(approvedManjaSocialProvenance.Source.LocalPath), read(approvedManjaSocialProvenance.Artifact.LocalPath)
}

func validateManjaSocialProvenance(receipt manjaSocialProvenance, source, artifact []byte) error {
	approved := approvedManjaSocialProvenance
	if receipt != approved {
		return fmt.Errorf("Manja social receipt = %#v, want fixed approved evidence %#v", receipt, approved)
	}
	if err := validateManjaSocialSource(approved.Source, source); err != nil {
		return err
	}
	if err := validateManjaSocialPNG(approved.Artifact, artifact); err != nil {
		return err
	}

	response := httptest.NewRecorder()
	NewCatalogAssetsHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/manja-assets/manja-social.png", nil))
	if response.Code != http.StatusOK {
		return fmt.Errorf("Manja social asset response = %d, want 200", response.Code)
	}
	if got := response.Header().Get("Content-Type"); got != approved.Artifact.MIMEType {
		return fmt.Errorf("Manja social asset content type = %q, want %q", got, approved.Artifact.MIMEType)
	}
	if !bytes.Equal(response.Body.Bytes(), artifact) {
		return fmt.Errorf("Manja social asset response differs from tracked PNG")
	}
	return nil
}

type manjaSocialSVG struct {
	XMLName xml.Name `xml:"svg"`
	Width   int      `xml:"width,attr"`
	Height  int      `xml:"height,attr"`
}

func validateManjaSocialSource(approved manjaSocialSourceEvidence, data []byte) error {
	if len(data) != approved.Size {
		return fmt.Errorf("Manja social SVG size = %d, want %d", len(data), approved.Size)
	}
	if got := manjaSocialGitBlobSHA(data); got != approved.GitBlobSHA {
		return fmt.Errorf("Manja social SVG Git blob = %s, want %s", got, approved.GitBlobSHA)
	}
	if got := manjaSocialSHA256Hex(data); got != approved.SHA256 {
		return fmt.Errorf("Manja social SVG SHA-256 = %s, want %s", got, approved.SHA256)
	}
	var root manjaSocialSVG
	if err := xml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("parse Manja social SVG: %w", err)
	}
	if root.XMLName.Local != "svg" || root.Width != approved.Width || root.Height != approved.Height {
		return fmt.Errorf("Manja social SVG root/dimensions = %s %dx%d, want svg %dx%d", root.XMLName.Local, root.Width, root.Height, approved.Width, approved.Height)
	}
	return nil
}

func validateManjaSocialPNG(approved manjaSocialPNGArtifact, data []byte) error {
	if len(data) != approved.Size || (len(data) < 1<<20) != approved.UnderOneMiB {
		return fmt.Errorf("Manja social PNG size/gate = %d/%t, want %d/%t", len(data), len(data) < 1<<20, approved.Size, approved.UnderOneMiB)
	}
	if got := manjaSocialGitBlobSHA(data); got != approved.GitBlobSHA {
		return fmt.Errorf("Manja social PNG Git blob = %s, want %s", got, approved.GitBlobSHA)
	}
	if got := manjaSocialSHA256Hex(data); got != approved.SHA256 {
		return fmt.Errorf("Manja social PNG SHA-256 = %s, want %s", got, approved.SHA256)
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("decode Manja social PNG: %w", err)
	}
	if config.Width != approved.Width || config.Height != approved.Height {
		return fmt.Errorf("Manja social PNG dimensions = %dx%d, want %dx%d", config.Width, config.Height, approved.Width, approved.Height)
	}
	if len(data) < 29 || string(data[12:16]) != "IHDR" {
		return fmt.Errorf("Manja social PNG lacks complete IHDR")
	}
	if width := int(binary.BigEndian.Uint32(data[16:20])); width != approved.Width {
		return fmt.Errorf("Manja social PNG IHDR width = %d, want %d", width, approved.Width)
	}
	if height := int(binary.BigEndian.Uint32(data[20:24])); height != approved.Height {
		return fmt.Errorf("Manja social PNG IHDR height = %d, want %d", height, approved.Height)
	}
	if int(data[24]) != approved.BitDepth || int(data[25]) != approved.ColorType || int(data[28]) != approved.InterlaceMethod {
		return fmt.Errorf("Manja social PNG IHDR encoding = bit-depth %d color-type %d interlace %d, want %d/%d/%d", data[24], data[25], data[28], approved.BitDepth, approved.ColorType, approved.InterlaceMethod)
	}
	return nil
}

func manjaSocialSHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func manjaSocialGitBlobSHA(data []byte) string {
	hash := sha1.New() // #nosec G401 -- Git blob identity is defined as SHA-1.
	_, _ = fmt.Fprintf(hash, "blob %d\x00", len(data))
	_, _ = hash.Write(data)
	return hex.EncodeToString(hash.Sum(nil))
}
