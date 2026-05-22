package format

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
)

// WriteNDJSON appends records to path, one JSON object per line. The file is
// opened with O_APPEND so successive Export calls accumulate into the same
// file until the LocalFileExporter rolls to a new sequence index on size.
//
// v is expected to be a slice of structs (or []any). A non-slice value is
// written as a single line. Each top-level slice element produces exactly one
// `\n`-terminated JSON line so downstream tools (DuckDB read_json_auto,
// `jq -c`, log shippers) can parse line-by-line without buffering.
func WriteNDJSON(path string, v any) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)

	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Slice {
		return enc.Encode(v)
	}
	for i := 0; i < val.Len(); i++ {
		if err := enc.Encode(val.Index(i).Interface()); err != nil {
			return fmt.Errorf("ndjson: encode element %d: %w", i, err)
		}
	}
	return nil
}
