package gateway

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Phase 39 step 1: a call that bypasses the gateway fails a test that
// searches for one. The test parses every non-test Go file under cmd/,
// internal/ and domains/ and fails, naming file:line, wherever one of the
// things only the gateway may do appears outside the places allowed for it.
//
// Each allow-list maps "relative/path.go:Func" (methods as "Recv.Method") to
// why that use is not a bypass. An entry that no longer matches anything also
// fails the test, so a stale entry cannot sit there waiting to hide a new
// bypass written in the same function name.

// gatewayDir is where every rule's subject is allowed without an entry.
const gatewayDir = "internal/feature/gateway/"

// The packages whose types the audit and cost rules look for, by import path
// rather than by local name, so an unrelated digest.Spend is not caught.
const (
	storagePath = "github.com/bsenel/karakuri/internal/platform/storage"
	quotaPath   = "github.com/bsenel/karakuri/internal/quota"
)

// bypassRule is one thing the gateway owns.
type bypassRule struct {
	name string
	// scope limits the rule to files under these prefixes; empty means all.
	scope []string
	// match reports whether n is the thing this rule looks for; imports maps
	// each local package name in the file to its import path.
	match func(n ast.Node, imports map[string]string) bool
	allow map[string]string
}

var bypassRules = []bypassRule{
	{
		// environment.Environment.Act(ctx, a): the call a loop action runs.
		name: "environment Act call",
		match: func(n ast.Node, imports map[string]string) bool {
			return isMethodCall(n, "Act", 2)
		},
		allow: map[string]string{
			"internal/feature/loop/act.go:stepAct":                   "the perform func stepAct hands to Gateway.Call; the Act call is inside that closure",
			"domains/software/remediation_env.go:remediationEnv.Act": "an environment's own Act delegating to its shell executor; the outer Act is the call the gateway already charged and audited",
			"domains/software/review_runs.go:runReviewCommand":       "runs the review's build inside the review capability's Act (composeReviewBody), which the gateway already charged and audited",
			"domains/software/mcp_server.go:MCPServer.CallTool":      "the software pack's server, which the gateway calls from the perform it routes to a pack (Gateway.servedBy, Phase 39 step 3); the gateway has already charged the call and writes its row",
		},
	},
	{
		// The capability quota charge: gateway.allowCapability calls
		// karakuriquota.Deps.TakeCapability before a call runs.
		name: "capability quota charge",
		match: func(n ast.Node, imports map[string]string) bool {
			return isMethodCall(n, "TakeCapability", -1) || isCallNamed(n, "allowCapability")
		},
	},
	{
		// The tool-call audit row: a storage.ToolEvent with no Kind (the
		// gateway's row) or of the execute kind. Rows of every other kind
		// (refusal, escalation, run_end, baseline, ...) are decisions about a
		// run, not the record of a tool call.
		name: "tool call audit row",
		match: func(n ast.Node, imports map[string]string) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok || !isSelector(lit.Type, imports, storagePath, "ToolEvent") {
				return false
			}
			kind, found := field(lit, "Kind")
			return !found || isSelectorName(kind, "ToolEventExecute")
		},
		allow: map[string]string{
			"internal/feature/report/deliver.go:Service.audit": "records a scheduled digest delivered to a channel (capability deliver.digest), not a tool call an agent made",
		},
	},
	{
		// The tool-call cost entry: a quota Spend counted in calls (or with no
		// unit, which the recorder would take as one). Token spend from a model
		// call is not on the tool path.
		name: "tool call cost entry",
		match: func(n ast.Node, imports map[string]string) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok || !isSelector(lit.Type, imports, quotaPath, "Spend") {
				return false
			}
			unit, found := field(lit, "UnitKind")
			return !found || isSelectorName(unit, "UnitCalls")
		},
	},
	{
		// A served MCP tool run: mcpTool.read(ctx, h, p, args). Only runTool may
		// call it, through Gateway.Call or, with no gateway wired, directly for
		// fixtures.
		name:  "served MCP tool dispatch",
		scope: []string{"internal/api/"},
		match: func(n ast.Node, imports map[string]string) bool {
			return isMethodCall(n, "read", 4)
		},
		allow: map[string]string{
			"internal/api/handler/mcp.go:MCPHandler.runTool": "the one dispatch: through Gateway.Call, or directly when no Gateway is wired (fixtures)",
		},
	},
}

func TestNoCallBypassesTheGateway(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()

	used := map[string]bool{}
	var hits []string
	for _, dir := range []string{"cmd", "internal", "domains"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				switch d.Name() {
				case "testdata", "vendor", "node_modules", "web":
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				return fmt.Errorf("parse %s: %w", rel, err)
			}
			hits = append(hits, scanFile(fset, rel, file, used)...)
			return nil
		})
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}

	sort.Strings(hits)
	for _, h := range hits {
		t.Errorf("bypasses the gateway: %s", h)
	}
	for _, r := range bypassRules {
		for key := range r.allow {
			if !used[r.name+"|"+key] {
				t.Errorf("stale allow-list entry for %s: %s matches nothing; remove it", r.name, key)
			}
		}
	}
}

// scanFile returns "file:line: rule in Func" for each disallowed match in
// file, and marks the allow-list entries it used.
func scanFile(fset *token.FileSet, rel string, file *ast.File, used map[string]bool) []string {
	if strings.HasPrefix(rel, gatewayDir) {
		return nil
	}
	imports := fileImports(file)
	var hits []string
	for _, decl := range file.Decls {
		fn := declName(decl)
		ast.Inspect(decl, func(n ast.Node) bool {
			if n == nil {
				return false
			}
			for _, r := range bypassRules {
				if !inScope(rel, r.scope) || !r.match(n, imports) {
					continue
				}
				key := rel + ":" + fn
				if _, ok := r.allow[key]; ok {
					used[r.name+"|"+key] = true
					continue
				}
				pos := fset.Position(n.Pos())
				hits = append(hits, fmt.Sprintf("%s:%d: %s in %s", rel, pos.Line, r.name, fn))
			}
			return true
		})
	}
	return hits
}

func inScope(rel string, scope []string) bool {
	if len(scope) == 0 {
		return true
	}
	for _, s := range scope {
		if strings.HasPrefix(rel, s) {
			return true
		}
	}
	return false
}

// declName is "Func", "Recv.Method", or "<package level>" for a var block.
func declName(decl ast.Decl) string {
	fd, ok := decl.(*ast.FuncDecl)
	if !ok {
		return "<package level>"
	}
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return fd.Name.Name
	}
	typ := fd.Recv.List[0].Type
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}
	if idx, ok := typ.(*ast.IndexExpr); ok {
		typ = idx.X
	}
	if id, ok := typ.(*ast.Ident); ok {
		return id.Name + "." + fd.Name.Name
	}
	return fd.Name.Name
}

// isMethodCall reports whether n is x.name(...) with argc arguments (any
// count when argc is negative).
func isMethodCall(n ast.Node, name string, argc int) bool {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	return argc < 0 || len(call.Args) == argc
}

// isCallNamed reports whether n calls name, bare or as a selector.
func isCallNamed(n ast.Node, name string) bool {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return false
	}
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name == name
	case *ast.SelectorExpr:
		return fun.Sel.Name == name
	}
	return false
}

// isSelector reports whether e is name from the package imported as path.
func isSelector(e ast.Expr, imports map[string]string, path, name string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && imports[id.Name] == path
}

// fileImports maps each local package name in file to its import path. An
// unnamed import is keyed by the path's last element, which is the package
// name for every package these rules look for.
func fileImports(file *ast.File) map[string]string {
	out := map[string]string{}
	for _, imp := range file.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		name := path[strings.LastIndex(path, "/")+1:]
		if imp.Name != nil {
			name = imp.Name.Name
		}
		out[name] = path
	}
	return out
}

// isSelectorName reports whether e is a bare name or x.name.
func isSelectorName(e ast.Expr, name string) bool {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name == name
	case *ast.SelectorExpr:
		return v.Sel.Name == name
	}
	return false
}

// field returns the value of a keyed field in lit.
func field(lit *ast.CompositeLit, name string) (ast.Expr, bool) {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if id, ok := kv.Key.(*ast.Ident); ok && id.Name == name {
			return kv.Value, true
		}
	}
	return nil, false
}

// moduleRoot walks up from the working directory to the one holding go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the working directory")
		}
		dir = parent
	}
}
