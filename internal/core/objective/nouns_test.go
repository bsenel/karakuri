package objective

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// softwareNouns are metric names that belong to the software pack. The core
// holds only Measurement, the state, the operators and the comparison; metric
// names live in packs (docs/roadmap.md, Phase 41 decision 2, 'Core stays free
// of software nouns', and step 4's "the import check shows no software noun
// in internal/core").
var softwareNouns = []string{"coverage", "p95_latency_ms", "bundle", "flaky"}

// TestCoreNamesNoSoftwareMetric scans every non-test Go file under
// internal/core for a software metric noun in an identifier or a string
// literal. Comments are prose and may mention one; code may not.
func TestCoreNamesNoSoftwareMetric(t *testing.T) {
	root := filepath.Join("..")
	fset := token.NewFileSet()
	scanned := 0

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		scanned++
		ast.Inspect(file, func(n ast.Node) bool {
			var text string
			switch n := n.(type) {
			case *ast.Ident:
				text = n.Name
			case *ast.BasicLit:
				if n.Kind != token.STRING {
					return true
				}
				text = n.Value
			default:
				return true
			}
			lower := strings.ToLower(text)
			for _, noun := range softwareNouns {
				if strings.Contains(lower, noun) {
					t.Errorf("%s: %q names the software metric %q; it belongs in a pack",
						fset.Position(n.Pos()), text, noun)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// A walk that found nothing would pass for the wrong reason.
	if scanned == 0 {
		t.Fatal("no Go file under internal/core was scanned")
	}
}
