package quota

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/platform/valkey"
	"github.com/bsenel/karakuri/quota"
	quotasql "github.com/bsenel/karakuri/quota/sql"
	quotavalkey "github.com/bsenel/karakuri/quota/valkey"
)

// Build constructs the deps the API mounts, choosing a backend from config.
//
// db may be nil when the SQL backend is not selected. hub may be nil, in which
// case pressure is logged rather than published.
func Build(ctx context.Context, cfg config.QuotaConfig, db *sql.DB, hub *event.Hub) (Deps, error) {
	tiers := DefaultTiers(cfg)
	if err := tiers.Validate(); err != nil {
		return Deps{}, fmt.Errorf("quota tiers: %w", err)
	}

	backend, closeFn, err := buildBackend(ctx, cfg, db)
	if err != nil {
		return Deps{}, err
	}
	deps := Deps{Backend: backend, Tiers: tiers, Hub: hub, Close: closeFn}

	budget, err := buildBudget(cfg, deps)
	if err != nil {
		_ = closeFn()
		return Deps{}, err
	}
	deps.TokenBudget = budget
	return deps, nil
}

// buildBudget picks who counts model spend.
func buildBudget(cfg config.QuotaConfig, deps Deps) (TokenBudget, error) {
	switch cfg.LLMBudgetBackend {
	case config.LLMBudgetNative, "":
		return deps.Budget(), nil
	case config.LLMBudgetLiteLLM:
		key := ""
		if cfg.LiteLLMKeyEnv != "" {
			key = os.Getenv(cfg.LiteLLMKeyEnv)
		}
		b, err := NewLiteLLMBudget(cfg.LiteLLMURL, key, deps.Tiers.LLMTokens.Cap, nil)
		if err != nil {
			return nil, err
		}
		slog.Info("llm spend is counted by a LiteLLM gateway",
			"url", cfg.LiteLLMURL,
			"note", "the gateway is the authority on refusals; Karakuri turns them into checkpoints")
		return b, nil
	default:
		return nil, fmt.Errorf("unknown llm budget backend %q (want native or litellm)", cfg.LLMBudgetBackend)
	}
}

func buildBackend(ctx context.Context, cfg config.QuotaConfig, db *sql.DB) (quota.Backend, func() error, error) {
	noClose := func() error { return nil }

	switch cfg.Backend {
	case config.QuotaBackendMemory, "":
		// Loud, because the failure mode is silent: three replicas each keep
		// their own counters, so a limit of 60/min admits 180 and nothing
		// anywhere reports a problem.
		slog.Info("quota counters are per-replica",
			"backend", config.QuotaBackendMemory,
			"note", "set quota.backend=valkey to share them across replicas")
		return quota.NewMemoryBackend(), noClose, nil

	case config.QuotaBackendSQL:
		if db == nil {
			return nil, nil, fmt.Errorf("quota backend %q needs a database", cfg.Backend)
		}
		dialect := quotasql.SQLite
		if isPostgres(db) {
			dialect = quotasql.Postgres
		}
		b, err := quotasql.New(db, quotasql.Options{Dialect: dialect})
		if err != nil {
			return nil, nil, err
		}
		if err := b.Migrate(ctx); err != nil {
			return nil, nil, fmt.Errorf("quota schema: %w", err)
		}
		return b, noClose, nil

	case config.QuotaBackendValkey:
		if cfg.ValkeyURL == "" {
			return nil, nil, fmt.Errorf("quota backend %q needs quota.valkey_url", cfg.Backend)
		}
		client, err := valkey.New(cfg.ValkeyURL, valkey.Options{})
		if err != nil {
			return nil, nil, err
		}
		b, err := quotavalkey.New(client, quotavalkey.Options{KeyPrefix: "karakuri:quota:"})
		if err != nil {
			client.Close()
			return nil, nil, err
		}
		return b, client.Close, nil

	default:
		return nil, nil, fmt.Errorf("unknown quota backend %q (want memory, sql or valkey)", cfg.Backend)
	}
}

// isPostgres asks the driver rather than the config, because the quota backend
// is handed a *sql.DB that somebody else opened and the two could disagree.
func isPostgres(db *sql.DB) bool {
	// Postgres is the only dialect here whose placeholder syntax differs, and
	// the cheapest reliable probe is a statement only it accepts.
	var one int
	return db.QueryRow(`SELECT 1 WHERE $1 = $1`, 1).Scan(&one) == nil
}
