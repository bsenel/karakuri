package format

import (
	"encoding/json"
	"os"
)

// WriteParquet writes NDJSON as a stand-in for v1; full parquet encoding deferred.
func WriteParquet(path string, v any) error {
	parquetPath := path
	if len(path) > 8 && path[len(path)-8:] != ".parquet" {
		parquetPath = path[:len(path)-len(getExt(path))] + ".parquet"
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(parquetPath, data, 0o644)
}

func getExt(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			return path[i:]
		}
	}
	return ""
}
