// Command gen writes docs/diagnostics/codes.md from the code registry.
package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/HazelnutParadise/phego/internal/diagcodes"
)

func main() {
	path := filepath.Join("..", "..", "docs", "diagnostics", "codes.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(diagcodes.Markdown()), 0o644); err != nil {
		log.Fatal(err)
	}
}
