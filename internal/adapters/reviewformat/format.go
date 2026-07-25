// Package reviewformat writes deterministic human and machine review reports.
package reviewformat

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/araihu/manja/internal/core"
)

const (
	FormatJSON     = "json"
	FormatText     = "text"
	FormatMarkdown = "markdown"
)

// Write emits report in the requested deterministic format.
func Write(w io.Writer, format string, report core.ReviewReport) error {
	switch format {
	case FormatJSON:
		encoded, err := core.CanonicalReviewJSON(report)
		if err != nil {
			return fmt.Errorf("canonical review JSON: %w", err)
		}
		return writeBytes(w, append(encoded, '\n'))
	case FormatText:
		return writeBytes(w, []byte(formatText(report)))
	case FormatMarkdown:
		return writeBytes(w, []byte(formatMarkdown(report)))
	default:
		return fmt.Errorf("unknown review format %q", format)
	}
}

func formatText(report core.ReviewReport) string {
	var output strings.Builder
	fmt.Fprintf(&output, "Contract: %s\n", report.ContractID)
	fmt.Fprintf(&output, "Engine: %s\n", report.EngineVersion)
	fmt.Fprintf(&output, "Evaluated at: %s\n", formatTime(report.EvaluatedAt))
	fmt.Fprintf(&output, "Verdict: %s\n", report.Verdict)

	for _, comparison := range report.Comparisons {
		output.WriteByte('\n')
		fmt.Fprintf(&output, "%s (%s -> %s): %s\n", comparisonTitle(comparison.Kind), comparison.Baseline.RevisionID, comparison.Candidate.RevisionID, comparison.Policy.Verdict)
		writeTextFindings(&output, comparison.Policy.Decisions)
		writeTextExceptions(&output, comparison.Policy.AppliedExceptions)
	}

	output.WriteByte('\n')
	fmt.Fprintf(&output, "Effective policy digest: %s\n", report.PolicyDigest)
	return output.String()
}

func writeTextFindings(output *strings.Builder, decisions []core.FindingDecision) {
	if len(decisions) == 0 {
		output.WriteString("  Findings: none\n")
		return
	}
	output.WriteString("  Findings:\n")
	for _, decision := range decisions {
		fmt.Fprintf(output, "  - %s %s %s: %s\n", decision.Level, decision.Finding.RuleID, decision.Finding.Subject, decision.Finding.Description)
	}
}

func writeTextExceptions(output *strings.Builder, exceptions []core.PolicyException) {
	if len(exceptions) == 0 {
		output.WriteString("  Applied exceptions: none\n")
		return
	}
	output.WriteString("  Applied exceptions:\n")
	for _, exception := range exceptions {
		fmt.Fprintf(output, "  - reason: %s; author: %s; expires: %s\n", exception.Reason, exception.Author, formatTime(exception.ExpiresAt))
	}
}

func formatMarkdown(report core.ReviewReport) string {
	var output strings.Builder
	fmt.Fprintf(&output, "# Contract review: %s\n\n", report.ContractID)
	fmt.Fprintf(&output, "- Engine: %s\n", report.EngineVersion)
	fmt.Fprintf(&output, "- Evaluated at: %s\n", formatTime(report.EvaluatedAt))
	fmt.Fprintf(&output, "- Verdict: %s\n", report.Verdict)

	for _, comparison := range report.Comparisons {
		fmt.Fprintf(&output, "\n## %s\n\n", comparisonTitle(comparison.Kind))
		fmt.Fprintf(&output, "- Baseline: %s\n", comparison.Baseline.RevisionID)
		fmt.Fprintf(&output, "- Candidate: %s\n", comparison.Candidate.RevisionID)
		fmt.Fprintf(&output, "- Verdict: %s\n", comparison.Policy.Verdict)
		writeMarkdownFindings(&output, comparison.Policy.Decisions)
		writeMarkdownExceptions(&output, comparison.Policy.AppliedExceptions)
	}

	fmt.Fprintf(&output, "\n## Effective policy\n\n- Digest: %s\n", report.PolicyDigest)
	return output.String()
}

func writeMarkdownFindings(output *strings.Builder, decisions []core.FindingDecision) {
	if len(decisions) == 0 {
		output.WriteString("- Findings: none\n")
		return
	}
	output.WriteString("- Findings:\n")
	for _, decision := range decisions {
		fmt.Fprintf(output, "  - `%s` `%s` `%s`: %s\n", decision.Level, decision.Finding.RuleID, decision.Finding.Subject, decision.Finding.Description)
	}
}

func writeMarkdownExceptions(output *strings.Builder, exceptions []core.PolicyException) {
	if len(exceptions) == 0 {
		output.WriteString("- Applied exceptions: none\n")
		return
	}
	output.WriteString("- Applied exceptions:\n")
	for _, exception := range exceptions {
		fmt.Fprintf(output, "  - Reason: %s\n", exception.Reason)
		fmt.Fprintf(output, "    Author: %s\n", exception.Author)
		fmt.Fprintf(output, "    Expires: %s\n", formatTime(exception.ExpiresAt))
	}
}

func comparisonTitle(kind string) string {
	switch kind {
	case core.ComparisonPullRequest:
		return "Pull-request delta"
	case core.ComparisonReleaseImpact:
		return "Release impact"
	default:
		return kind
	}
}

func formatTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func writeBytes(w io.Writer, bytes []byte) error {
	written, err := w.Write(bytes)
	if err != nil {
		return err
	}
	if written != len(bytes) {
		return io.ErrShortWrite
	}
	return nil
}
