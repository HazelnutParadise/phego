package diagcodes

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestCodesDocumentMatchesRegistry(t *testing.T) {
	got, err := os.ReadFile(filepath.Join("..", "..", "docs", "diagnostics", "codes.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != Markdown() {
		t.Fatal("docs/diagnostics/codes.md is stale; run go generate ./internal/diagcodes")
	}
}

var codeShape = regexp.MustCompile(`^PHG[1-9][0-9]{3}$`)

func TestRegistryEntriesComplete(t *testing.T) {
	for c, e := range Registry {
		if !codeShape.MatchString(string(c)) {
			t.Errorf("%s is not PHG followed by four digits", c)
		}
		if e.Title == "" || e.Note == "" || e.Help == "" || strings.Contains(e.Title, "\n") {
			t.Errorf("%s needs a one-line title, a note and a help line", c)
		}
	}
}

// Production code must use the named constants, so every emitted code is in
// the registry and a typo cannot create an undocumented one.
func TestCodeLiteralsOnlyInRegistry(t *testing.T) {
	root := filepath.Join("..", "..")
	lit := regexp.MustCompile(`PHG[0-9]{4}`)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "testdata") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
			filepath.Base(path) == "codes.go" && filepath.Base(filepath.Dir(path)) == "diagcodes" {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if b, ok := n.(*ast.BasicLit); ok && b.Kind == token.STRING {
				if s, err := strconv.Unquote(b.Value); err == nil && lit.MatchString(s) {
					t.Errorf("%s: code literal %q outside internal/diagcodes/codes.go", path, s)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
