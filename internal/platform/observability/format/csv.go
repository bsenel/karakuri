package format

import (
	"encoding/csv"
	"fmt"
	"os"
	"reflect"
	"time"
)

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
	for i := 0; i < val.Len(); i++ {
		item := val.Index(i)
		if item.Kind() == reflect.Struct {
			var row []string
			for j := 0; j < item.NumField(); j++ {
				fv := item.Field(j).Interface()
				if t, ok := fv.(time.Time); ok {
					row = append(row, t.Format(time.RFC3339))
				} else {
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
