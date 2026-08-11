package auth

import "github.com/bsenel/karakuri/auth"

// Built-in role names.
const (
	RoleAdmin       = "admin"
	RoleOperator    = "operator"
	RoleContributor = "contributor"
	RoleViewer      = "viewer"
	RoleAuditor     = "auditor"
)

// BuiltinRoles returns the five roles Karakuri ships with, composed by
// inheritance so each permission is stated once:
//
//	viewer    read-only across the operational surface
//	  ├─ auditor      + the authority-bounds audit log
//	  ├─ contributor  + creates work, and manages the twins it owns
//	  └─ operator     + everything that drives work: twins, objectives, loops,
//	                  checkpoints, artifacts, memory, research
//	admin     everything, including the auth model itself
//
// They are flagged System, so they cannot be edited or deleted through the API —
// an operator rewriting "admin" is the fastest way to lock everyone out.
//
// Note there are no explicit deny policies here. Deny exists for the case where
// a role would otherwise inherit an allow that must not apply to it, and none of
// these do: default-deny already covers everything a role does not name.
// Adding a deny to `operator` would be actively wrong, because `admin` would
// inherit it and lose the permission — deny beats allow regardless of which role
// is nearer. Deployment-specific denies belong on custom roles.
func BuiltinRoles() []auth.Role {
	viewer := auth.Role{
		Name:        RoleViewer,
		Description: "Read-only access to twins, objectives, loops, checkpoints, artifacts and memory.",
		System:      true,
		Policies: []auth.Policy{
			auth.Allow("viewer-twin-read", ActionTwinRead, "*"),
			auth.Allow("viewer-objective-read", ActionObjectiveRead, "*"),
			auth.Allow("viewer-loop-read", ActionLoopRead, "*"),
			auth.Allow("viewer-checkpoint-read", ActionCheckpointRead, "*"),
			auth.Allow("viewer-artifact-read", ActionArtifactRead, "*"),
			auth.Allow("viewer-memory-read", ActionMemoryRead, "*"),
			auth.Allow("viewer-domain-read", ActionDomainRead, "*"),
			// Seeing your own ceiling is not privileged: a caller who is being
			// throttled should be able to find out why without an operator.
			auth.Allow("viewer-quota-read", ActionQuotaRead, "*"),
		},
	}

	auditor := auth.Role{
		Name:        RoleAuditor,
		Description: "Everything a viewer can do, plus the authority-bounds audit log.",
		System:      true,
		Inherits:    []string{RoleViewer},
		Policies: []auth.Policy{
			auth.Allow("auditor-audit-read", ActionAuditRead, "*"),
		},
	}

	operator := auth.Role{
		Name:        RoleOperator,
		Description: "Drives work: creates twins and objectives, runs loops, resolves checkpoints.",
		System:      true,
		Inherits:    []string{RoleViewer},
		Policies: []auth.Policy{
			auth.Allow("operator-twin-create", ActionTwinCreate, "*"),
			auth.Allow("operator-twin-update", ActionTwinUpdate, "*"),
			auth.Allow("operator-twin-delete", ActionTwinDelete, "*"),
			auth.Allow("operator-twin-bind", ActionTwinBind, "*"),
			auth.Allow("operator-objective-create", ActionObjectiveCreate, "*"),
			auth.Allow("operator-objective-update", ActionObjectiveUpdate, "*"),
			auth.Allow("operator-loop-start", ActionLoopStart, "*"),
			auth.Allow("operator-loop-resume", ActionLoopResume, "*"),
			auth.Allow("operator-checkpoint-resolve", ActionCheckpointResolve, "*"),
			auth.Allow("operator-artifact-write", ActionArtifactWrite, "*"),
			auth.Allow("operator-memory-write", ActionMemoryWrite, "*"),
			auth.Allow("operator-memory-forget", ActionMemoryForget, "*"),
			auth.Allow("operator-research-run", ActionResearchRun, "*"),
		},
	}

	// contributor is where ownership earns its keep: it can create twins and
	// change the ones it created, but not anybody else's. Expressing that with
	// a condition rather than an `if` inside the handler means it shows up in
	// `krk auth policies list` and in the effective-permissions endpoint,
	// instead of being invisible until someone reads the code.
	//
	// Note this is strictly narrower than operator, which may change any twin.
	owned := auth.Condition{Kind: auth.CondOwnerEquals}
	contributor := auth.Role{
		Name:        RoleContributor,
		Description: "Creates work and manages the twins it owns, but not other people's.",
		System:      true,
		Inherits:    []string{RoleViewer},
		Policies: []auth.Policy{
			auth.Allow("contributor-twin-create", ActionTwinCreate, "*"),
			auth.Allow("contributor-twin-update", ActionTwinUpdate, "twin:*").When(owned),
			auth.Allow("contributor-twin-delete", ActionTwinDelete, "twin:*").When(owned),
			auth.Allow("contributor-twin-bind", ActionTwinBind, "twin:*").When(owned),
			auth.Allow("contributor-objective-create", ActionObjectiveCreate, "*"),
			auth.Allow("contributor-loop-start", ActionLoopStart, "*"),
		},
	}

	admin := auth.Role{
		Name:        RoleAdmin,
		Description: "Full control, including principals, roles and role bindings.",
		System:      true,
		Policies: []auth.Policy{
			// The wildcard rather than inheritance: an administrator should
			// pick up any action a future phase registers, without someone
			// having to remember to extend this list.
			auth.Allow("admin-all", "*", "*"),
		},
	}

	return []auth.Role{viewer, auditor, contributor, operator, admin}
}
