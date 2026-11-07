package diagcodes

import (
	"slices"
	"strings"

	"github.com/HazelnutParadise/phego/diag"
)

// Markdown renders the code table; docs/diagnostics/codes.md must equal it.
func Markdown() string {
	codes := make([]diag.Code, 0, len(Registry))
	for c := range Registry {
		codes = append(codes, c)
	}
	slices.Sort(codes)
	var b strings.Builder
	b.WriteString("# Phego diagnostic codes\n\n")
	b.WriteString("Generated from `internal/diagcodes/codes.go` by `go generate ./internal/diagcodes`; do not edit by hand.\n\n")
	b.WriteString("These codes cover Phego's own diagnostics only. PHP warnings, fatal errors and exception messages keep PHP 8.5.12's exact text and carry no PHG code.\n\n")
	b.WriteString("A published code is never renumbered or reused. `PHG1xxx` covers Engine configuration and run options, `PHG2xxx` bytecode loading and verification.\n")
	for _, c := range codes {
		e := Registry[c]
		b.WriteString("\n## " + string(c) + ": " + e.Title + "\n\n")
		if e.Retired {
			b.WriteString("Retired: no longer emitted.\n\n")
		}
		b.WriteString("- note: " + e.Note + "\n")
		b.WriteString("- help: " + e.Help + "\n")
	}
	return b.String()
}
