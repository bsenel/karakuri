package loop

import (
	"time"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/memory"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/core/twin"
)

type Step string

const (
	StepObserve Step = "observe"
	StepReason  Step = "reason"
	StepDecide  Step = "decide"
	StepAct     Step = "act"
	StepVerify  Step = "verify"
	StepLearn   Step = "learn"
)

type Request struct {
	Objective objective.Objective
	Twin      twin.DigitalTwin
	Agent     agent.Definition
	MaxIter   int // hard cap; default 50
}

type Result struct {
	LoopID       string                    `json:"loop_id"`
	ObjectiveID  objective.ObjectiveID     `json:"objective_id"`
	Status       objective.ObjectiveStatus `json:"status"`
	Iterations   []Iteration               `json:"iterations,omitempty"`
	CriteriaMet  float64                   `json:"criteria_met"`
	CheckpointID *string                   `json:"checkpoint_id,omitempty"`
	LearnedFacts []memory.Entry            `json:"learned_facts,omitempty"`
}

type Iteration struct {
	Number     int
	Step       Step
	Input      any
	Output     any
	TokensUsed int
	Duration   time.Duration
	Timestamp  time.Time
}

type WorldState struct {
	Observations []environment.Observation
	Version      string // composite SHA of all observation SHAs
	Timestamp    time.Time

	// Blind names the environments whose Observe returned an error, so that
	// "saw nothing" and "could not see" are distinguishable from outside.
	//
	// They used to be dropped with a bare `continue`, which made an
	// environment that went blind look exactly like one that looked and found
	// the world unchanged. The outer loop refused that conflation in Phase 20
	// — reconcile.Fingerprint.Blind, same name for the same reason — and the
	// inner loop never learned it.
	Blind []string
}

type Context struct {
	ObjectiveID objective.ObjectiveID
	TwinID      string
	Iteration   int
	PriorSteps  []Iteration
}

type Status struct {
	LoopID      string                `json:"loop_id"`
	ObjectiveID objective.ObjectiveID `json:"objective_id"`
	Step        Step                  `json:"step"`
	Iteration   int                   `json:"iteration"`
	CriteriaMet float64               `json:"criteria_met"`
	Paused      bool                  `json:"paused"`
}
