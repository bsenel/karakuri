package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCustomerAttributionStampsTheHeader(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(CustomerHeader)
	}))
	defer srv.Close()

	client := WithCustomerAttribution(srv.Client(), customerPrefix)

	req, _ := http.NewRequestWithContext(WithTwin(context.Background(), "t1"), http.MethodGet, srv.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	resp.Body.Close()

	if got != "twin:t1" {
		t.Errorf("%s = %q, want twin:t1", CustomerHeader, got)
	}
}

func TestCustomerAttributionSkipsCallsWithNoTwin(t *testing.T) {
	// Charging an unattributed call to some arbitrary customer would be worse
	// than leaving it unattributed.
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(CustomerHeader)
	}))
	defer srv.Close()

	client := WithCustomerAttribution(srv.Client(), customerPrefix)
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	resp.Body.Close()

	if got != "" {
		t.Errorf("%s = %q on a call with no twin", CustomerHeader, got)
	}
}

func TestWithTwinIgnoresAnEmptyID(t *testing.T) {
	ctx := WithTwin(context.Background(), "")
	if _, ok := TwinFromContext(ctx); ok {
		t.Error("an empty twin id was stored")
	}
}

func TestGatewayEnvSetsTheCustomerHeader(t *testing.T) {
	// The CLI providers are spawned per call, so the environment is the only
	// place a per-twin value can go.
	env := gatewayEnv(WithTwin(context.Background(), "t1"), []string{"PATH=/usr/bin"})

	want := "ANTHROPIC_CUSTOM_HEADERS=" + CustomerHeader + ": twin:t1"
	if !contains(env, want) {
		t.Errorf("env = %v, want it to contain %q", env, want)
	}
}

func TestGatewayEnvLeavesAnOperatorsOwnSettingAlone(t *testing.T) {
	// Somebody who has pointed the CLI somewhere deliberately should not have
	// it overridden.
	existing := "ANTHROPIC_CUSTOM_HEADERS=x-team: platform"
	env := gatewayEnv(WithTwin(context.Background(), "t1"), []string{existing})

	if len(env) != 1 || env[0] != existing {
		t.Errorf("env = %v, want the original left untouched", env)
	}
}

func TestGatewayEnvIsANoOpWithoutATwin(t *testing.T) {
	env := gatewayEnv(context.Background(), []string{"PATH=/usr/bin"})
	if len(env) != 1 {
		t.Errorf("env = %v, want it unchanged", env)
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
