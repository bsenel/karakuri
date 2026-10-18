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
func WriteNDJSON(path string, v any) (err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) // #nosec G304 -- path is the operator's export destination
	if err != nil {
		return err
	}
	// Close is where a full disk shows up on a written file, so its error is
	// reported unless an earlier one already is.
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("ndjson: close %s: %w", path, cerr)
		}
	}()
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
