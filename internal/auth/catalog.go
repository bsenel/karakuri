// Package auth wires the standalone github.com/bsenel/karakuri/auth module into
// Karakuri. The module knows nothing about twins, objectives or loops; this
// package is where those concepts meet it — the action catalog, the built-in
// roles, the route→permission table, and first-boot seeding.
//
// See ADR 007 for why the engine lives outside the main module.
package auth

import "github.com/bsenel/karakuri/auth"

// Every action the API exposes. Nothing is implicit: a policy naming an action
// that is not registered below is rejected when roles are seeded, so a typo in
// a role definition fails at boot rather than silently granting nothing.
const (
	ActionTwinCreate auth.Action = "twin:create"
	ActionTwinRead   auth.Action = "twin:read"
	ActionTwinUpdate auth.Action = "twin:update"
	ActionTwinDelete auth.Action = "twin:delete"
	ActionTwinBind   auth.Action = "twin:bind"

	ActionObjectiveCreate auth.Action = "objective:create"
	ActionObjectiveRead   auth.Action = "objective:read"
	ActionObjectiveUpdate auth.Action = "objective:update"

	// Standing objectives (Phase 20). Declaring one is a separate permission
	// from creating an ordinary objective, because it is a different kind of
	// grant: a one-shot objective spends what one run costs and stops, while a
	// standing one spends on a cadence indefinitely and can be given authority
	// to act unsupervised. Somebody trusted to file work is not automatically
	// trusted to start something that runs every hour until told otherwise.
	ActionObjectiveDeclare auth.Action = "objective:declare"

	// ActionObjectiveReconcile is asking for a reconcile now, outside the
	// cadence. It sits with the operator permissions rather than the viewer
	// ones: it starts a loop, and a loop costs money.
	ActionObjectiveReconcile auth.Action = "objective:reconcile"

	// ActionObjectivePause stops and restarts a standing objective. Separate
	// from declare because the people who should be able to stop a runaway are
	// not only the people who may start one — on call at 3am, the ability to
	// halt should be the easier permission to hold, not the harder.
	ActionObjectivePause auth.Action = "objective:pause"

	// Digests (Phase 21). Reading a schedule is one permission and writing
	// one is another, because a schedule is an instruction to send mail to a
	// named address on a recurring basis — which is a way to make Karakuri
	// message somebody who never asked to hear from it.
	ActionReportRead  auth.Action = "report:read"
	ActionReportWrite auth.Action = "report:write"

	ActionLoopStart  auth.Action = "loop:start"
	ActionLoopRead   auth.Action = "loop:read"
	ActionLoopResume auth.Action = "loop:resume"

	ActionCheckpointRead    auth.Action = "checkpoint:read"
	ActionCheckpointResolve auth.Action = "checkpoint:resolve"

	ActionArtifactRead  auth.Action = "artifact:read"
	ActionArtifactWrite auth.Action = "artifact:write"

	ActionMemoryRead   auth.Action = "memory:read"
	ActionMemoryWrite  auth.Action = "memory:write"
	ActionMemoryForget auth.Action = "memory:forget"

	ActionDomainRead  auth.Action = "domain:read"
	ActionResearchRun auth.Action = "research:run"
	ActionAuditRead   auth.Action = "audit:read"

	ActionAuthRead  auth.Action = "auth:read"
	ActionAuthWrite auth.Action = "auth:write"

	ActionQuotaRead  auth.Action = "quota:read"
	ActionQuotaAdmin auth.Action = "quota:admin"

	// ActionQuotaRequest is asking for more; ActionQuotaApprove is deciding.
	// They are separate because almost everybody should hold the first and
	// almost nobody the second — collapsing them into quota:admin would mean
	// the permission to request is the permission to grant.
	ActionQuotaRequest auth.Action = "quota:request"
	ActionQuotaApprove auth.Action = "quota:approve"

	// ActionCostRead is reading what was spent. Separate from quota:read
	// because a limit is operational and spend is commercial: plenty of
	// deployments want everybody to see the first and not the second.
	ActionCostRead auth.Action = "cost:read"

	// Containers are the tenancy tree (Phase 17). They are authorized like
	// anything else, which is what makes the hierarchy govern changes to
	// itself: creating a team under an org needs a grant covering that org, so
	// an acme administrator cannot create teams inside globex.
	ActionContainerRead  auth.Action = "container:read"
	ActionContainerWrite auth.Action = "container:write"

	// ActionContainerMove is separate from write because reparenting is the one
	// operation that touches two places at once — it needs a covering grant on
	// both the old and the new parent, or moving a team would be a way to walk
	// resources out of a tenant you hold and into one you do not.
	ActionContainerMove auth.Action = "container:move"
)

// NewCatalog returns the Karakuri action catalog.
func NewCatalog() *auth.Catalog {
	c := auth.NewCatalog()
	for action, description := range map[auth.Action]string{
		ActionTwinCreate: "create a digital twin",
		ActionTwinRead:   "read twins and their event stream",
		ActionTwinUpdate: "update a twin",
		ActionTwinDelete: "delete a twin",
		ActionTwinBind:   "change a twin's adapter bindings",

		ActionObjectiveCreate: "create an objective",
		ActionObjectiveRead:   "read objectives, templates and their event stream",
		ActionObjectiveUpdate: "change an objective's status",

		ActionObjectiveDeclare:   "declare an objective standing, with a cadence and an autonomy ceiling",
		ActionObjectiveReconcile: "reconcile a standing objective now, outside its cadence",
		ActionObjectivePause:     "pause and resume a standing objective",

		ActionReportRead:  "read digest schedules and preview a digest",
		ActionReportWrite: "declare, delete and send a digest schedule",

		ActionLoopStart:  "start a reasoning loop",
		ActionLoopRead:   "read loop status",
		ActionLoopResume: "resume a paused loop",

		ActionCheckpointRead:    "read pending checkpoints",
		ActionCheckpointResolve: "approve, modify or reject a checkpoint",

		ActionArtifactRead:  "read and diff artifacts",
		ActionArtifactWrite: "write an artifact",

		ActionMemoryRead:   "recall memory entries",
		ActionMemoryWrite:  "store a memory entry",
		ActionMemoryForget: "delete a memory entry",

		ActionDomainRead:  "list domain packs and run conformance checks",
		ActionResearchRun: "run a research query",
		ActionAuditRead:   "read the authority-bounds audit log",

		ActionAuthRead:  "read principals, roles and policies",
		ActionAuthWrite: "create principals and change role bindings",

		ActionQuotaRead:  "read quota configuration and current usage",
		ActionQuotaAdmin: "reset a twin's quota counters",

		ActionQuotaRequest: "ask for a limit to be raised",
		ActionQuotaApprove: "approve or reject a quota request",
		ActionCostRead:     "read what was spent",

		ActionContainerRead:  "read organisations, teams and projects",
		ActionContainerWrite: "create, rename and delete containers, and place resources in them",
		ActionContainerMove:  "reparent a container, which requires holding both ends",
	} {
		c.MustRegister(action, description)
	}
	return c
}
