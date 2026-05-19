package objective

import "github.com/bsenel/karakuri/internal/core/agent"

type Template struct {
	ID              string           `json:"id"`
	Title           string           `json:"title"`
	Description     string           `json:"description,omitempty"`
	Domain          string           `json:"domain"`
	SuccessCriteria []Criterion      `json:"success_criteria,omitempty"`
	Constraints     []Constraint     `json:"constraints,omitempty"`
	SuggestedAgents []agent.Definition `json:"suggested_agents,omitempty"`
}
