package format

import (
	"encoding/csv"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"
)

// WriteCSV writes a slice of structs to path. The first row is a header
// derived from the struct field names so downstream tools (pandas, DuckDB,
// spreadsheet apps) can name columns without out-of-band schema.
func WriteCSV(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()

	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Slice {
		return w.Write([]string{fmt.Sprint(v)})
	}
	if val.Len() == 0 {
		return nil
	}

	first := val.Index(0)
	if first.Kind() == reflect.Struct {
		header := make([]string, first.NumField())
		for j := 0; j < first.NumField(); j++ {
			header[j] = strings.ToLower(first.Type().Field(j).Name)
		}
		if err := w.Write(header); err != nil {
			return err
		}
	}

	for i := 0; i < val.Len(); i++ {
		item := val.Index(i)
		if item.Kind() == reflect.Struct {
			var row []string
			for j := 0; j < item.NumField(); j++ {
				fv := item.Field(j).Interface()
				switch v := fv.(type) {
				case time.Time:
					row = append(row, v.Format(time.RFC3339))
				case map[string]string:
					parts := make([]string, 0, len(v))
					for k, val := range v {
						parts = append(parts, k+"="+val)
					}
					row = append(row, strings.Join(parts, ";"))
				default:
					row = append(row, fmt.Sprint(fv))
				}
			}
			if err := w.Write(row); err != nil {
				return err
			}
		}
	}
	return nil
}
