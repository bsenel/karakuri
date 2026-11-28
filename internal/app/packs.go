package app

import (
	"context"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/core/domain"
)

// registerConfiguredPacks registers every pack an enabled configuration entry
// names, from the pack's manifest and under the entry's pack ID (ADR 028
// section a). A pack no entry enables, or whose manifest is refused, is
// registered in domReg as disabled and stays out of the returned manifest
// registry. It returns that registry and the packs that registered.
//
// Phase 39 step 2, part B: compile scaffolding only; it registers nothing yet.
func registerConfiguredPacks(_ context.Context, _ []config.DomainConfig, _ []domain.Pack, _ *domain.Registry) (*domain.Registry, []domain.Pack) {
	return domain.NewRegistry(), nil
}
