package integration_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	karakuriauth "github.com/bsenel/karakuri/auth"
	karakuriquota "github.com/bsenel/karakuri/quota"
	"github.com/bsenel/karakuri/quota/cost"

	"github.com/bsenel/karakuri/internal/conformance"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/checkpoint"
	"github.com/bsenel/karakuri/internal/core/container"
	"github.com/bsenel/karakuri/internal/core/digest"
	"github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/memory"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/reconcile"
	"github.com/bsenel/karakuri/internal/core/twin"
	"github.com/bsenel/karakuri/internal/feature/artifact"
	"github.com/bsenel/karakuri/internal/feature/audit"
	"github.com/bsenel/karakuri/internal/feature/eval"
	"github.com/bsenel/karakuri/internal/feature/research"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools"
	"github.com/bsenel/karakuri/internal/platform/tools/mcp"
	internalquota "github.com/bsenel/karakuri/internal/quota"
)

// specTypes names the Go type behind each component schema that mirrors one.
// A schema listed here is checked field by field against its type; a schema
// for a response the handler builds as a map is not here, and is checked on
// the wire instead (see openapi_wire_test.go).
var specTypes = map[string]any{
	"AdapterStatus":           tools.AdapterStatus{},
	"Artifact":                artifact.Artifact{},
	"AuditEvent":              storage.ToolEvent{},
	"AuditExport":             audit.Export{},
	"AuthDecision":            karakuriauth.Decision{},
	"Autonomy":                objective.Autonomy{},
	"Budget":                  objective.Budget{},
	"Cadence":                 objective.Cadence{},
	"CalibrationReport":       eval.CalibrationReport{},
	"Capability":              capability.Capability{},
	"Checkpoint":              checkpoint.Checkpoint{},
	"CheckpointModifications": checkpoint.Modifications{},
	"ConformanceResult":       conformance.Result{},
	"Constraint":              objective.Constraint{},
	"Container":               container.Container{},
	"CostBucket":              cost.Bucket{},
	"Criterion":               objective.Criterion{},
	"Digest":                  digest.Digest{},
	"Grant":                   karakuriauth.Grant{},
	"LoopResult":              loop.Result{},
	"LoopStatus":              loop.Status{},
	"MCPInstanceHealth":       mcp.InstanceHealth{},
	"MemoryEntry":             memory.Entry{},
	"MemoryQuery":             memory.Query{},
	"MemoryRetentionPolicy":   memory.RetentionPolicy{},
	"Objective":               objective.Objective{},
	"ObjectiveTemplate":       objective.Template{},
	"Principal":               karakuriauth.Principal{},
	"QuotaOverride":           karakuriquota.Override{},
	"QuotaRequest":            karakuriquota.Request{},
	"QuotaTier":               internalquota.Tier{},
	"ReconcileOutcome":        reconcile.Outcome{},
	"ReconcileState":          reconcile.State{},
	"ReportSchedule":          digest.Schedule{},
	"ResearchResult":          research.Result{},
	"Role":                    karakuriauth.Role{},
	"RoleBinding":             karakuriauth.RoleBinding{},
	"TokenPair":               karakuriauth.TokenPair{},
	"Twin":                    twin.DigitalTwin{},
}

// The spec says what the server sends, so its schemas follow encoding/json:
// a nil slice, map or pointer is null, a struct carries exactly its exported
// json fields, a time.Time is an RFC 3339 string and a Duration a number of
// nanoseconds. A Go field added, renamed, retyped or removed without the spec
// following fails this test; OPENAPI_WRITE_SCHEMAS=<file> writes the schemas
// the types imply, to paste into docs/openapi.yaml.
func TestOpenAPISchemasMatchGoTypes(t *testing.T) {
	byType := map[reflect.Type]string{}
	for name, v := range specTypes {
		byType[reflect.TypeOf(v)] = name
	}
	want := map[string]any{}
	for name, v := range specTypes {
		want[name] = goSchema(reflect.TypeOf(v), byType, true)
	}

	if out := os.Getenv("OPENAPI_WRITE_SCHEMAS"); out != "" {
		b, err := yaml.Marshal(want)
		if err != nil {
			t.Fatalf("marshal schemas: %v", err)
		}
		if err := os.WriteFile(out, b, 0o600); err != nil {
			t.Fatalf("write schemas: %v", err)
		}
	}

	got := specSchemas(t)
	names := make([]string, 0, len(want))
	for name := range want {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		spec, ok := got[name]
		if !ok {
			t.Errorf("schema %s is missing from docs/openapi.yaml", name)
			continue
		}
		for _, d := range schemaDiff("", normalize(want[name]), normalize(spec)) {
			t.Errorf("schema %s: %s", name, d)
		}
	}
}

// goSchema is the OpenAPI 3.0 schema encoding/json gives t. top is false for
// a field or element, where a named type becomes a reference.
func goSchema(t reflect.Type, named map[reflect.Type]string, top bool) map[string]any {
	nullable := false
	for t.Kind() == reflect.Pointer {
		t, nullable = t.Elem(), true
	}
	wrap := func(s map[string]any) map[string]any {
		if !nullable {
			return s
		}
		if _, isRef := s["$ref"]; isRef {
			// A $ref takes no siblings in OpenAPI 3.0.
			return map[string]any{"nullable": true, "allOf": []any{s}}
		}
		s["nullable"] = true
		return s
	}
	if name, ok := named[t]; ok && !top {
		return wrap(map[string]any{"$ref": "#/components/schemas/" + name})
	}
	switch t {
	case reflect.TypeOf(time.Time{}):
		return wrap(map[string]any{"type": "string", "format": "date-time"})
	case reflect.TypeOf(time.Duration(0)):
		return wrap(map[string]any{"type": "integer", "description": "nanoseconds"})
	case reflect.TypeOf(json.RawMessage{}):
		return map[string]any{}
	}
	switch t.Kind() {
	case reflect.String:
		return wrap(map[string]any{"type": "string"})
	case reflect.Bool:
		return wrap(map[string]any{"type": "boolean"})
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return wrap(map[string]any{"type": "integer"})
	case reflect.Float32, reflect.Float64:
		return wrap(map[string]any{"type": "number"})
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return map[string]any{"type": "string", "format": "byte", "nullable": true}
		}
		return map[string]any{"type": "array", "nullable": true, "items": goSchema(t.Elem(), named, false)}
	case reflect.Array:
		return wrap(map[string]any{"type": "array", "items": goSchema(t.Elem(), named, false)})
	case reflect.Map:
		return map[string]any{"type": "object", "nullable": true, "additionalProperties": goSchema(t.Elem(), named, false)}
	case reflect.Interface:
		return map[string]any{}
	case reflect.Struct:
		props := map[string]any{}
		for _, f := range jsonFields(t) {
			props[f.name] = goSchema(f.typ, named, false)
		}
		s := map[string]any{"type": "object", "additionalProperties": false}
		if len(props) > 0 {
			s["properties"] = props
		}
		return wrap(s)
	}
	return map[string]any{}
}

type jsonField struct {
	name string
	typ  reflect.Type
}

// jsonFields are the fields encoding/json writes for t, embedded structs
// flattened into their parent.
func jsonFields(t reflect.Type) []jsonField {
	var out []jsonField
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if f.Anonymous && name == "" {
			ft := f.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				out = append(out, jsonFields(ft)...)
				continue
			}
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		out = append(out, jsonField{name, f.Type})
	}
	return out
}

// specSchemas reads components.schemas from docs/openapi.yaml.
func specSchemas(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "openapi.yaml"))
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	var spec struct {
		Components struct {
			Schemas map[string]any `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	return spec.Components.Schemas
}

// normalize drops what documents a schema without constraining it, so the
// spec may carry descriptions and examples the Go type cannot. The keys of a
// properties map are field names, not keywords: a field called "title" or
// "description" is kept.
func normalize(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, val := range x {
			switch k {
			case "description", "example", "title":
				continue
			case "properties":
				fields, _ := val.(map[string]any)
				props := map[string]any{}
				for name, fs := range fields {
					props[name] = normalize(fs)
				}
				out[k] = props
				continue
			}
			out[k] = normalize(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = normalize(val)
		}
		return out
	}
	return v
}

// schemaDiff reports where want and got differ, one line per path.
func schemaDiff(path string, want, got any) []string {
	wm, wok := want.(map[string]any)
	gm, gok := got.(map[string]any)
	if wok && gok {
		keys := map[string]bool{}
		for k := range wm {
			keys[k] = true
		}
		for k := range gm {
			keys[k] = true
		}
		sorted := make([]string, 0, len(keys))
		for k := range keys {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		var out []string
		for _, k := range sorted {
			p := path + "/" + k
			wv, inW := wm[k]
			gv, inG := gm[k]
			switch {
			case !inG:
				out = append(out, fmt.Sprintf("%s: the Go type has %v, the spec has nothing", p, brief(wv)))
			case !inW:
				out = append(out, fmt.Sprintf("%s: the spec has %v, the Go type has nothing", p, brief(gv)))
			default:
				out = append(out, schemaDiff(p, wv, gv)...)
			}
		}
		return out
	}
	if !reflect.DeepEqual(want, got) {
		return []string{fmt.Sprintf("%s: the Go type gives %v, the spec says %v", path, brief(want), brief(got))}
	}
	return nil
}

func brief(v any) string {
	b, _ := json.Marshal(v)
	if len(b) > 120 {
		return string(b[:120]) + "..."
	}
	return string(b)
}
