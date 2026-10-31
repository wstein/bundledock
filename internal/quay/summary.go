package quay

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// maxSummaryRows keeps a section far below the 1 MiB limit of a step summary;
// the saved plan holds every row.
const maxSummaryRows = 100

// cell makes untrusted text inert in a Markdown table cell: no table breaks,
// code spans, escapes, links through HTML or raw HTML.
func cell(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ", "\\", "\\\\", "|", "\\|", "`", "'", "<", "&lt;", ">", "&gt;").Replace(s)
}

func rowsTable(header string, lines []string) string {
	var b strings.Builder
	b.WriteString("\n" + header + "\n|---|---|---|---|\n")
	for i, line := range lines {
		if i == maxSummaryRows {
			fmt.Fprintf(&b, "\n%d more rows are in the saved plan.\n", len(lines)-maxSummaryRows)
			break
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

// PlanSummary renders a plan as a GitHub step summary section. It holds only
// plan fields: repository, tag name, decision and the sanitized reason.
func PlanSummary(plan Plan) string {
	eligible := 0
	var lines []string
	for _, r := range plan.Rows {
		state := "retained"
		if r.Eligible {
			eligible++
			state = "eligible"
		}
		lines = append(lines, fmt.Sprintf("| %s | `%s` | %s | %s |", cell(r.Repository), cell(r.Name), state, cell(r.Reason)))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n### Staging cleanup (%s)\n\nStaging tags: %d; eligible: %d (dry run, nothing changed)\n", plan.Mode, len(plan.Rows), eligible)
	if len(lines) > 0 {
		b.WriteString(rowsTable("| Repository | Tag | Decision | Reason |", lines))
	}
	return b.String()
}

// SafeReason is the part of an error that may be shown in a summary: a
// verification failure or an HTTP status, never anything else.
func SafeReason(err error) string {
	var reason reasonError
	var status *StatusError
	switch {
	case errors.As(err, &reason):
		return string(reason)
	case errors.As(err, &status):
		return status.Error()
	}
	return "see the job log"
}

// ApplySummary renders the result of an apply as a step summary section.
// err is the error that stopped the run, if any.
func ApplySummary(mode string, target *FailedRun, actions []Action, err error) string {
	deleted := 0
	var lines []string
	for _, a := range actions {
		if a.Action == "deleted" {
			deleted++
		}
		lines = append(lines, fmt.Sprintf("| %s | `%s` | %s |  |", cell(a.Repository), cell(a.Name), cell(a.Action)))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n### Staging cleanup apply (%s)\n\n", mode)
	if target != nil {
		fmt.Fprintf(&b, "Run %s attempt %d.\n\n", target.RunID, target.Attempt)
	}
	fmt.Fprintf(&b, "Actions: %d; deleted: %d\n", len(actions), deleted)
	if err != nil {
		fmt.Fprintf(&b, "\nStopped: %s\n", SafeReason(err))
	}
	if len(lines) > 0 {
		b.WriteString(rowsTable("| Repository | Tag | Action | |", lines))
	}
	return b.String()
}

// AppendStepSummary appends a section to the file named by GITHUB_STEP_SUMMARY.
// It never truncates the file, and does nothing when path is empty.
func AppendStepSummary(path, section string) error {
	if path == "" {
		return nil
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(section); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
