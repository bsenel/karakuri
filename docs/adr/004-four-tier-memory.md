# ADR 004: Four-Tier Memory Architecture

## Status

Accepted

## Context

A single loop run produces useful reasoning traces and action outcomes. Without persistence, each run starts from scratch and the agent cannot improve over time. Different kinds of knowledge have different lifetimes and retrieval patterns:

- Immediate context within a run (fast, volatile)
- Detailed traces of what happened in past runs (medium-term, queryable)
- Distilled facts across many runs (long-term, similarity-searchable)
- Statistical capability performance (long-term, point-lookup)

A single storage tier cannot serve all four patterns efficiently.

## Decision

Memory is split into four tiers, each with its own storage backend and retrieval semantic:

| Tier | Backend | Written by | Read by |
|------|---------|-----------|---------|
| Working | sync.Map | Any loop step | Any step in the same run |
| Episodic | SQLite (`memory_episodic`) | Learn step | Observe step (top-K recall) |
| Semantic | SQLite (`memory_semantic`) | Consolidation job | Observe step (similarity recall) |
| Procedural | SQLite (`memory_procedural`) | Learn step | Decide step (confidence biasing) |

After each learn step, the consolidation job promotes high-confidence (≥0.8) episodic entries to semantic. The procedural tier accumulates per-capability success/failure counts and adjusts plan confidence at the decide step (+0.05 for >80% success, −0.10 for <30%).

The `Memory` interface (`internal/core/memory/`) abstracts all tiers. The semantic backend defaults to SQLite keyword search; swapping to sqlite-vec or pgvector requires only changing `internal/platform/memory/semantic.go`.

## Consequences

- Second and subsequent runs on the same objective domain produce demonstrably better reasoning (procedural bias, episodic recall)
- Memory is queryable via `POST /memory/recall` and `krk memory recall`
- Consolidation runs automatically; threshold is configurable (`memory.consolidation_threshold` in config)
- Working memory is lost on server restart; all persistent tiers survive restarts
