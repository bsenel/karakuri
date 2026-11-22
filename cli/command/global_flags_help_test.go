package command

import (
	"strings"
	"testing"
)

func TestGlobalFlagsHelp(t *testing.T) {
	flags := NewRoot().PersistentFlags()

	// "API base URL" does not say whose API, nor what the path must end in.
	api := flags.Lookup("api-url").Usage
	for _, want := range []string{"Karakuri server", "/api/v1"} {
		if !strings.Contains(api, want) {
			t.Errorf("--api-url usage %q does not mention %q", api, want)
		}
	}

	// A bare "json|pretty|quiet" names the values but not what each is for.
	out := flags.Lookup("output").Usage
	for _, want := range []string{"pretty (", "json (", "quiet ("} {
		if !strings.Contains(out, want) {
			t.Errorf("--output usage %q does not explain %q", out, want)
		}
	}
}
