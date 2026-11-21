// Package software implements the Karakuri Software Development domain pack.
package software

import (
	"context"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/domain"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools"
)

type Pack struct {
	tools *tools.Registry
	// issuer mints the credential a delegation holds for Karakuri's own MCP
	// tools; nil until AttachDelegationIssuer.
	issuer delegationIssuer
	// ownAuthors says whose pull request comments are the deployment's own.
	ownAuthors config.OwnAuthorsConfig
	// reviewRoundLimit is the declared review round limit; 0 means none.
	reviewRoundLimit int
	// store is the audit log the record of Karakuri's own comments is read
	// from (Phase 40 step 12a); nil until WithStorage.
	store storage.StorageAdapter
}

// WithStorage gives the git environment the audit log it reads the comments
// Karakuri posted from (Phase 40 step 12a).
func (p *Pack) WithStorage(s storage.StorageAdapter) *Pack {
	p.store = s
	return p
}

// WithReviewRoundLimit sets how many answered third-party comment rounds a
// stream pull request takes before the stream waits for the owner (Phase 40
// step 12).
func (p *Pack) WithReviewRoundLimit(n int) *Pack {
	p.reviewRoundLimit = n
	return p
}

// WithOwnAuthors sets the accounts whose pull request comments are the
// deployment's own (Phase 40 step 12a).
func (p *Pack) WithOwnAuthors(a config.OwnAuthorsConfig) *Pack {
	p.ownAuthors = a
	return p
}

// AttachDelegationIssuer gives the CLI environment the issuer for delegation
// credentials. It is separate from the constructor because the pack is built
// before the auth stack the issuer signs with; environments built afterwards
// see it.
func (p *Pack) AttachDelegationIssuer(issuer delegationIssuer) { p.issuer = issuer }

// New constructs a software domain pack without tool adapters — environments
// fall back to no-op behavior. Used by tests and the conformance suite.
func New() *Pack { return &Pack{} }

// NewWithTools constructs a software pack whose environments dispatch to the
// supplied tool registry (real GitHub / Linear / Slack adapters when configured).
func NewWithTools(reg *tools.Registry) *Pack { return &Pack{tools: reg} }

func (p *Pack) ID() string      { return "software" }
func (p *Pack) Name() string    { return "Software Development" }
func (p *Pack) Version() string { return "1.0.0" }
func (p *Pack) Description() string {
	return "Capabilities, environments, and agents for autonomous software development"
}

func (p *Pack) Init(_ context.Context, _ domain.Config) error { return nil }

func (p *Pack) Teardown(_ context.Context) error { return nil }

func (p *Pack) Capabilities() []capability.Capability {
	return append(softwareCapabilities(), selfImproveCapabilities()...)
}

func (p *Pack) EnvironmentFactories() []environment.Factory {
	factories := softwareEnvironmentFactories(p.tools, p.ownAuthors, p.reviewRoundLimit)
	for i, f := range factories {
		if f.EnvID == EnvGit && p.store != nil {
			build := f.Build
			factories[i].Build = func(ctx environment.BuildContext) (environment.Environment, error) {
				env, err := build(ctx)
				if git, ok := env.(*gitEnv); ok {
					git.posted = newPostedCommentReader(p.store)
				}
				return env, err
			}
			continue
		}
		if f.EnvID != "software.env.cli_agent" {
			continue
		}
		build := f.Build
		factories[i].Build = func(ctx environment.BuildContext) (environment.Environment, error) {
			env, err := build(ctx)
			if cli, ok := env.(*cliEnv); ok {
				cli.issuer = p.issuer
			}
			return env, err
		}
	}
	return append(factories, platformTelemetryFactory())
}

func (p *Pack) AgentDefinitions() []agent.Definition {
	return append(append(softwareAgentDefinitions(), selfImproveAgents()...), pullRequestReviewAgent())
}

func (p *Pack) ObjectiveTemplates() []objective.Template {
	return append(append(append(softwareObjectiveTemplates(), selfImproveTemplates()...), streamTemplates()...), pullRequestReviewTemplate())
}

func (p *Pack) PlannerHints() []domain.PlannerHint {
	return softwarePlannerHints()
}
