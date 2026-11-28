package app

import (
	"context"
	"log/slog"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/core/domain"
)

// registerConfiguredPacks registers every pack an enabled configuration entry
// names, from the pack's manifest and under the entry's pack ID (ADR 028
// section a). A pack no entry enables, or whose manifest is refused, is
// registered in domReg as disabled and stays out of the returned manifest
// registry. It returns that registry and the packs that registered.
//
// The class of a pack is decided here, by the operator's entry, and nothing a
// pack or a server says about itself is read to decide it.
func registerConfiguredPacks(ctx context.Context, entries []config.DomainConfig, packs []domain.Pack, domReg *domain.Registry) (*domain.Registry, []domain.Pack) {
	enabled := make(map[string]config.DomainConfig)
	for _, dc := range entries {
		if dc.Enabled {
			enabled[dc.ID] = dc
		}
	}
	// Kept apart from domReg, which still owns Init, Get and List: registering a
	// pack in both would count its planner hints twice.
	manifests := domain.NewRegistry()
	var active []domain.Pack
	for _, pack := range packs {
		dc, ok := enabled[pack.ID()]
		if !ok {
			_ = domReg.Register(ctx, pack, domain.Config{})
			continue
		}
		// The namespace is the entry's ID. The manifest is checked before Init,
		// so a refused pack is listed as disabled and never initialised with the
		// entry's options; it is registered only once Init has succeeded, so a
		// pack that failed to start contributes no tools.
		tools, m := pack.Capabilities(), domain.ManifestOf(pack)
		if err := domain.NewRegistry().RegisterManifest(dc.ID, tools, m); err != nil {
			slog.Warn("domain pack manifest refused", "domain", dc.ID, "err", err)
			_ = domReg.Register(ctx, pack, domain.Config{})
			continue
		}
		if err := domReg.Register(ctx, pack, domain.Config(dc.Options)); err != nil {
			slog.Warn("domain pack init failed", "domain", dc.ID, "err", err)
			continue
		}
		if err := manifests.RegisterManifest(dc.ID, tools, m); err != nil {
			slog.Warn("domain pack manifest refused", "domain", dc.ID, "err", err)
			continue
		}
		active = append(active, pack)
	}
	return manifests, active
}
