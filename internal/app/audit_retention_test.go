package app

import (
	"context"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/feature/audit"
)

func TestAuditRetentionResolvesAndAccepts(t *testing.T) {
	tests := []struct {
		name string
		rc   config.AuditRetentionConfig
		want audit.Retention
	}{
		{"absent floor resolves to the declared one", config.AuditRetentionConfig{}, audit.Retention{FloorDays: 183, Days: 0}},
		{"days equal to the default floor", config.AuditRetentionConfig{Days: 183}, audit.Retention{FloorDays: 183, Days: 183}},
		{"raised floor, days above it", config.AuditRetentionConfig{FloorDays: 365, Days: 400}, audit.Retention{FloorDays: 365, Days: 400}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := auditRetention(tt.rc)
			if err != nil {
				t.Fatalf("auditRetention(%+v) error = %v, want nil", tt.rc, err)
			}
			if got != tt.want {
				t.Fatalf("auditRetention(%+v) = %+v, want %+v", tt.rc, got, tt.want)
			}
		})
	}
}

// A retention below the floor refuses startup, and the error names the
// setting, the floor in force and the offending value.
func TestAuditRetentionRefusesBelowFloor(t *testing.T) {
	tests := []struct {
		name   string
		rc     config.AuditRetentionConfig
		wantIn []string
	}{
		{"floor lowered below the declared one", config.AuditRetentionConfig{FloorDays: 100}, []string{"audit.retention", "183", "100"}},
		{"days below the default floor", config.AuditRetentionConfig{Days: 30}, []string{"audit.retention", "183", "30"}},
		{"days below a raised floor", config.AuditRetentionConfig{FloorDays: 365, Days: 200}, []string{"audit.retention", "365", "200"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := auditRetention(tt.rc)
			if err == nil {
				t.Fatalf("auditRetention(%+v) = nil error, want one naming %v", tt.rc, tt.wantIn)
			}
			for _, want := range tt.wantIn {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not name %s", err, want)
				}
			}
		})
	}
}

// The shipped defaults must start: never prune, at the declared floor.
func TestAuditRetentionShippedDefaults(t *testing.T) {
	// A set token keeps Load from shelling out to gh.
	t.Setenv("GITHUB_TOKEN", "preexisting-token")
	cfg, err := config.Load("../../config/default.yaml")
	if err != nil {
		t.Fatalf("Load default.yaml: %v", err)
	}
	got, err := auditRetention(cfg.Audit.Retention)
	if err != nil {
		t.Fatalf("auditRetention(default.yaml) error = %v, want nil", err)
	}
	if want := (audit.Retention{FloorDays: 183, Days: 0}); got != want {
		t.Fatalf("auditRetention(default.yaml) = %+v, want %+v", got, want)
	}
}

func TestStartAuditRetentionNeverPruneStartsNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := audit.Retention{FloorDays: 183, Days: 0}
	if startAuditRetention(ctx, audit.NewService(nil, r), r) {
		t.Fatal("startAuditRetention with Days = 0 started a sweep, want none")
	}
}

func TestStartAuditRetentionStartsSweep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := audit.Retention{FloorDays: 183, Days: 200}
	if !startAuditRetention(ctx, audit.NewService(nil, r), r) {
		t.Fatal("startAuditRetention with Days = 200 started no sweep, want one")
	}
}
