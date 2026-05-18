package format

import (
	"encoding/json"
	"os"
)

func WriteNDJSON(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	switch records := v.(type) {
	case []any:
		for _, r := range records {
			if err := enc.Encode(r); err != nil {
				return err
			}
		}
	default:
		return enc.Encode(v)
	}
	return nil
}
