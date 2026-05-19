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
	Objective  objective.Objective
	Twin       twin.DigitalTwin
	Agent      agent.Definition
	MaxIter    int  // hard cap; default 50
	WatchMode  bool // if true, loop continues on environment events
}

type Result struct {
	LoopID       string                  `json:"loop_id"`
	ObjectiveID  objective.ObjectiveID   `json:"objective_id"`
	Status       objective.ObjectiveStatus `json:"status"`
	Iterations   []Iteration             `json:"iterations,omitempty"`
	CriteriaMet  float64                 `json:"criteria_met"`
	CheckpointID *string                 `json:"checkpoint_id,omitempty"`
	LearnedFacts []memory.Entry          `json:"learned_facts,omitempty"`
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
