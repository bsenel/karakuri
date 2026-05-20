package research

import (
	"context"
	"fmt"
)

type HTTPScraper struct{}

func NewHTTPScraper() *HTTPScraper { return &HTTPScraper{} }

func (h *HTTPScraper) Name() string { return "http-scraper" }

func (h *HTTPScraper) Active() bool { return true }

func (h *HTTPScraper) Search(ctx context.Context, topic string, sources []string, depth string) ([]Finding, error) {
	_ = ctx
	findings := []Finding{{
		Title:      fmt.Sprintf("Research summary: %s", topic),
		Summary:    fmt.Sprintf("Automated research pulse for topic '%s' at depth '%s' from %d sources.", topic, depth, len(sources)),
		Confidence: 0.65,
		Source:     "http-scraper",
	}}
	if len(sources) > 0 {
		findings[0].Source = sources[0]
	}
	return findings, nil
}
