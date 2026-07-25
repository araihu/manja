package reviewformat

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode"

	core "github.com/araihu/manja/domain"
)

func TestWriteJSONUsesCanonicalReviewJSON(t *testing.T) {
	report := core.ReviewReport{
		SchemaVersion: core.ReviewSchemaVersion,
		EngineVersion: "test",
		ContractID:    "payments",
		EvaluatedAt:   time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		PolicyDigest:  "policy-digest",
		Verdict:       core.VerdictPass,
		Comparisons: []core.ComparisonReport{{
			Kind:      core.ComparisonPullRequest,
			Baseline:  core.SnapshotRef{RevisionID: "target"},
			Candidate: core.SnapshotRef{RevisionID: "head"},
			Policy:    core.PolicyResult{Verdict: core.VerdictPass},
		}},
	}
	var got bytes.Buffer
	if err := Write(&got, FormatJSON, report); err != nil {
		t.Fatal(err)
	}
	want, err := core.CanonicalReviewJSON(report)
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatalf("json differs:\n%s\nwant:\n%s", got.Bytes(), want)
	}
}

func TestWriteTextAndMarkdownMatchGoldenFiles(t *testing.T) {
	for _, test := range []struct {
		name   string
		format string
		report core.ReviewReport
		golden string
	}{
		{
			name:   "passing text report",
			format: FormatText,
			report: passReport(),
			golden: "pass.txt.golden",
		},
		{
			name:   "failing text report",
			format: FormatText,
			report: failReport(),
			golden: "fail.txt.golden",
		},
		{
			name:   "failing markdown report",
			format: FormatMarkdown,
			report: failReport(),
			golden: "fail.md.golden",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var got bytes.Buffer
			if err := Write(&got, test.format, test.report); err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join("testdata", test.golden))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.Bytes(), want) {
				t.Fatalf("output differs:\n%s\nwant:\n%s", got.Bytes(), want)
			}
			if bytes.Count(got.Bytes(), []byte("\n")) == 0 || got.Bytes()[len(got.Bytes())-1] != '\n' || bytes.HasSuffix(got.Bytes(), []byte("\n\n")) {
				t.Fatalf("output must end with exactly one newline: %q", got.Bytes())
			}
		})
	}
}

func TestWriteRejectsUnknownFormat(t *testing.T) {
	var got bytes.Buffer
	if err := Write(&got, "yaml", passReport()); err == nil {
		t.Fatal("Write() error = nil")
	}
}

func TestWriteTextAndMarkdownEscapeUntrustedContent(t *testing.T) {
	report := failReport()
	report.ContractID = "payments\n# forged\x1b[31m\u202e"
	report.EngineVersion = "v1\t*bold*"
	report.Comparisons[0].Baseline.RevisionID = "target\r\n## forged"
	report.Comparisons[0].Policy.Decisions[0].Finding.RuleID = "operation.`removed`\n- injected"
	report.Comparisons[0].Policy.Decisions[0].Finding.Subject = "GET `/payments` **spoof**"
	report.Comparisons[0].Policy.Decisions[0].Finding.Description = "removed\n## injected <script>\x00"
	report.Comparisons[0].Policy.AppliedExceptions[0].Reason = "reason\n## injected `ticks` *em* \x1b[2J"
	report.Comparisons[0].Policy.AppliedExceptions[0].Author = "author\r> quote\u202e\u2028separator"

	var text bytes.Buffer
	if err := Write(&text, FormatText, report); err != nil {
		t.Fatal(err)
	}
	assertNoUnsafeControls(t, text.String())
	for _, want := range []string{
		`Contract: payments\n# forged\u001b[31m\u202e`,
		`Engine: v1\t*bold*`,
		`target\r\n## forged`,
		`removed\n## injected <script>\u0000`,
		`reason\n## injected ` + "`ticks`" + ` *em* \u001b[2J`,
		`author\r> quote\u202e\u2028separator`,
	} {
		if !strings.Contains(text.String(), want) {
			t.Fatalf("text output missing %q:\n%s", want, text.String())
		}
	}

	var markdown bytes.Buffer
	if err := Write(&markdown, FormatMarkdown, report); err != nil {
		t.Fatal(err)
	}
	assertNoUnsafeControls(t, markdown.String())
	for _, want := range []string{
		`payments\\n\# forged\\u001b\[31m\\u202e`,
		`v1\\t\*bold\*`,
		"``operation.`removed`\\n- injected``",
		"``GET `/payments` **spoof**``",
		`removed\\n\#\# injected \<script\>\\u0000`,
		`reason\\n\#\# injected \` + "`ticks\\`" + ` \*em\* \\u001b\[2J`,
		`author\\r\> quote\\u202e\\u2028separator`,
	} {
		if !strings.Contains(markdown.String(), want) {
			t.Fatalf("markdown output missing %q:\n%s", want, markdown.String())
		}
	}
	for _, forgedBlock := range []string{"\n# forged", "\n## injected", "\n- injected"} {
		if strings.Contains(markdown.String(), forgedBlock) {
			t.Fatalf("markdown contains injected structure %q:\n%s", forgedBlock, markdown.String())
		}
	}
	if !strings.HasSuffix(markdown.String(), "\n") || strings.HasSuffix(markdown.String(), "\n\n") {
		t.Fatalf("markdown must end with exactly one newline: %q", markdown.String())
	}
}

func assertNoUnsafeControls(t *testing.T, value string) {
	t.Helper()
	for _, char := range value {
		if char != '\n' && (unicode.Is(unicode.Cc, char) || unicode.Is(unicode.Cf, char)) {
			t.Fatalf("output contains unsafe control/format character %U: %q", char, value)
		}
	}
}

func passReport() core.ReviewReport {
	return core.ReviewReport{
		SchemaVersion: core.ReviewSchemaVersion,
		EngineVersion: "test",
		ContractID:    "payments",
		EvaluatedAt:   time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		PolicyDigest:  "policy-digest",
		Verdict:       core.VerdictPass,
		Comparisons: []core.ComparisonReport{{
			Kind:      core.ComparisonPullRequest,
			Baseline:  core.SnapshotRef{RevisionID: "target"},
			Candidate: core.SnapshotRef{RevisionID: "head"},
			Policy:    core.PolicyResult{Verdict: core.VerdictPass},
		}},
	}
}

func failReport() core.ReviewReport {
	finding := core.SpecChange{
		ID:          "finding-1",
		RuleID:      core.RuleOperationRemoved,
		Severity:    core.SpecChangeBreaking,
		Kind:        "Removed endpoint",
		Subject:     "GET /payments",
		Description: "Endpoint exists in production docs but is missing from the candidate.",
	}
	exception := core.PolicyException{
		FindingID: "finding-1",
		Reason:    "v2 migration",
		Author:    "api-team",
		ExpiresAt: time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC),
		Source:    core.PolicySourceRepository,
	}
	return core.ReviewReport{
		SchemaVersion: core.ReviewSchemaVersion,
		EngineVersion: "test",
		ContractID:    "payments",
		EvaluatedAt:   time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		PolicyDigest:  "policy-digest",
		Verdict:       core.VerdictFail,
		Comparisons: []core.ComparisonReport{
			{
				Kind:      core.ComparisonPullRequest,
				Baseline:  core.SnapshotRef{RevisionID: "target"},
				Candidate: core.SnapshotRef{RevisionID: "head"},
				Findings:  []core.SpecChange{finding},
				Policy: core.PolicyResult{
					Verdict:           core.VerdictFail,
					Decisions:         []core.FindingDecision{{Finding: finding, Level: core.RuleLevelFail, Source: core.PolicySourceRepository}},
					AppliedExceptions: []core.PolicyException{exception},
				},
			},
			{
				Kind:      core.ComparisonReleaseImpact,
				Baseline:  core.SnapshotRef{RevisionID: "release"},
				Candidate: core.SnapshotRef{RevisionID: "head"},
				Findings:  []core.SpecChange{finding},
				Policy: core.PolicyResult{
					Verdict:   core.VerdictFail,
					Decisions: []core.FindingDecision{{Finding: finding, Level: core.RuleLevelFail, Source: core.PolicySourceRepository}},
				},
			},
		},
	}
}
