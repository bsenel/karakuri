package config

import "testing"

func TestLoad_ReviewRoundLimitParsed(t *testing.T) {
	if got := loadYAML(t, "review_round_limit: 1\n").ReviewRoundLimit; got != 1 {
		t.Errorf("ReviewRoundLimit = %d, want 1", got)
	}
}

func TestLoad_DefaultReviewRoundLimitIsThree(t *testing.T) {
	cfg, err := Load("default.yaml")
	if err != nil {
		t.Fatalf("Load default.yaml: %v", err)
	}
	if cfg.ReviewRoundLimit != 3 {
		t.Errorf("default.yaml review_round_limit = %d, want 3", cfg.ReviewRoundLimit)
	}
}
