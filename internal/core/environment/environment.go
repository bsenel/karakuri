package environment

import (
	"context"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
)

type EnvironmentID string

type Environment interface {
	ID() EnvironmentID
	Domain() string

	Observe(ctx context.Context, q ObservationQuery) (Observation, error)
	Act(ctx context.Context, a Action) (ActionResult, error)
	Subscribe(ctx context.Context, f EventFilter) (<-chan EnvironmentEvent, error)
	Snapshot(ctx context.Context) (EnvironmentSnapshot, error)
}

// Trust says who wrote what a payload carries.
//
// It is declared by the environment that built the payload, the way
// Capability.NeedsWorkspace and Factory.Serves are declared, and for the same
// reason: the environment is the only party that knows. The loop does not, and
// deriving it from an EnvID would be the name-suffix mistake ADR 019 exists to
// record — `software.env.communication` reads like prose and returns an adapter
// name when no channel is filtered, while `software.env.git` reads like
// infrastructure and carries pull-request titles a stranger typed.
//
// The distinction is about authorship, not about correctness or confidence. A
// commit SHA an operator's GitHub returns is TrustOperator even if it is wrong;
// a Slack message is TrustThirdParty even if a colleague wrote it. What the
// decision policy does with it is escalate — never rank prose by how suspicious
// it reads.
type Trust string

const (
	// TrustOperator is the zero value, and means the payload is
	// infrastructure an operator configured: a commit list, an adapter name,
	// a metric, a file this deployment was pointed at.
	//
	// The zero value is the trusted one, and that is a real cost worth stating
	// rather than hiding: a payload carrying somebody else's prose that forgets
	// to say so is trusted. The alternative — untrusted by default — would
	// escalate every plan in every deployment until each of the ~20 shipped
	// environments was labelled, which is the kind of default an operator
	// switches off. Whether a pack has labelled itself honestly is not
	// decidable from outside the pack (see docs/adr/021); conformance checks
	// that the label it is given changes the outcome, which is the part that
	// can be checked.
	TrustOperator Trust = ""

	// TrustThirdParty means the payload carries text somebody outside this
	// deployment wrote: a pull-request title, an issue body, a chat message, a
	// scraped page. A plan built while such material is in evidence escalates,
	// whatever autonomy its agent has earned.
	TrustThirdParty Trust = "third_party"
)

// IsThirdParty reports whether this payload carries somebody else's writing.
// Anything that is not explicitly TrustThirdParty is treated as the operator's
// own infrastructure — see the note on TrustOperator.
func (t Trust) IsThirdParty() bool { return t == TrustThirdParty }

type ObservationQuery struct {
	Filter map[string]any
	Limit  int
}

type Observation struct {
	EnvID     EnvironmentID
	State     map[string]any
	Version   string // SHA of this observation
	Timestamp time.Time

	// Trust says who wrote State. Zero value is the operator's own
	// infrastructure; an environment that returns a third party's prose says
	// so, and the loop escalates any plan drafted while it is in evidence.
	Trust Trust
}

type Action struct {
	CapabilityID capability.CapabilityID
	Params       map[string]any
}

type ActionResult struct {
	Success      bool
	StateDelta   map[string]any
	ArtifactSHAs []string // any VFS blobs produced
	Error        string

	// Trust says who wrote StateDelta, and it is on this type as well as on
	// Observation because the act path is where the wider surface actually is.
	// researchEnv scrapes pages and returns them here; observation carries only
	// whether its adapter is wired. A phase that marked observations alone
	// would have left the larger hole open.
	Trust Trust
}

type EventFilter struct {
	Kinds []string
}

type EnvironmentEvent struct {
	EnvID     EnvironmentID
	Kind      string
	Delta     map[string]any
	Timestamp time.Time
}

type EnvironmentSnapshot struct {
	SHA       string
	EnvID     EnvironmentID
	State     map[string]any
	Timestamp time.Time
}
