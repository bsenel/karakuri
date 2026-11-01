package command

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// successCriteria builds the success_criteria of POST /objectives: the
// file's criteria first, then one per --criterion. It reads syntax only; what
// the criteria say is the server's to check, so a nil result means "send no
// key" and the template's criteria apply.
func successCriteria(file string, flags []string) ([]any, error) {
	var out []any
	if file != "" {
		raw, err := os.ReadFile(file) // #nosec G304 -- file is the path the user passed to --criteria-file
		if err != nil {
			return nil, fmt.Errorf("--criteria-file: %w", err)
		}
		// A file holding "[]" stays an empty list, a request for no criteria.
		if err := yaml.Unmarshal(raw, &out); err != nil {
			return nil, fmt.Errorf("--criteria-file %s: %w", file, err)
		}
	}
	for _, flag := range flags {
		c, err := parseCriterion(flag)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// parseCriterion reads one --criterion, "key=value;key=value". A value is
// split at its first '=' only, so "op=>=" is op ">=". target and weight go out
// as numbers and required as a boolean when they read as one; otherwise they
// go as written and the server says what is wrong.
func parseCriterion(flag string) (map[string]any, error) {
	c := map[string]any{}
	for _, token := range strings.Split(flag, ";") {
		key, value, ok := strings.Cut(token, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("--criterion %q: token %q is not key=value", flag, token)
		}
		c[key] = criterionValue(key, value)
	}
	return c, nil
}

func criterionValue(key, value string) any {
	switch key {
	case "target", "weight":
		if n, err := strconv.ParseFloat(value, 64); err == nil {
			return n
		}
	case "required":
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return value
}
