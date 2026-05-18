package research

import "context"

type Source struct {
	Name string
	URL  string
}

type Finding struct {
	Title      string
	Summary    string
	Confidence float64
	Source     string
}

type ResearchAdapter interface {
	Search(ctx context.Context, topic string, sources []string, depth string) ([]Finding, error)
	Active() bool
}
