package command

import "testing"

// A window with no spend comes back as null; on its own that reads like a
// failure, so the pretty format says there was nothing to total.
func TestCostReportSaysWhenNothingWasSpent(t *testing.T) {
	want := "No spend in this window. Widen --since or drop a filter; a running loop records what it spends\n"
	for _, body := range []string{"null", "[]\n"} {
		apiURL := startListServer(t, body)
		_, stderr, err := runKrkSplit("--api-url", apiURL, "cost", "report")
		if err != nil {
			t.Fatalf("krk cost report: %v", err)
		}
		if stderr != want {
			t.Errorf("krk cost report on %q wrote %q to stderr, want %q", body, stderr, want)
		}
	}
	apiURL := startListServer(t, "null")
	_, stderr, err := runKrkSplit("--api-url", apiURL, "--output", "json", "cost", "report")
	if err != nil {
		t.Fatalf("krk --output json cost report: %v", err)
	}
	if stderr != "" {
		t.Errorf("--output json wrote %q to stderr, want nothing", stderr)
	}
}
