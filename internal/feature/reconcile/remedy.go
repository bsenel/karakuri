package reconcile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	coreloop "github.com/bsenel/karakuri/internal/core/loop"
	"github.com/bsenel/karakuri/internal/core/memory"
	"github.com/bsenel/karakuri/internal/core/reconcile"
)

// maxRecalledRemedies bounds the hints one pass is shown.
const maxRecalledRemedies = 3

// recallRemedies fills the brief with the remedies stored for blockers of its
// kind by the stream's agent (Phase 40 step 14b). They are hints: they ride on
// the request and the plan made from them passes the same limits. A store that
// cannot answer recalls nothing, and the pass goes on without hints.
func (s *Service) recallRemedies(ctx context.Context, agent coreagent.AgentID, brief *coreloop.Unblock) {
	if brief == nil {
		return
	}
	entries, err := s.store.QuerySemantic(ctx, memory.Query{
		AgentID: agent, Domain: brief.Kind, TopK: maxRecalledRemedies,
	})
	if err != nil {
		slog.Warn("recall remedies", "kind", brief.Kind, "error", err)
		return
	}
	for _, e := range entries {
		// The store's filter is not trusted alone: another kind's remedy is
		// never handed to this blocker.
		if e.Domain == brief.Kind && e.Content != "" {
			brief.Recalled = append(brief.Recalled, e.Content)
		}
	}
}

// storeConfirmedRemedy keeps the fix a pass applied as one semantic memory
// entry of the stream's agent, with the blocker's kind as its domain and the
// pass's loop as its source. It does so only when the pass named a fix and the
// blocker it was asked to fix is no longer on the state: the next reading
// confirmed it. The same text for the same kind is one entry.
func (s *Service) storeConfirmedRemedy(ctx context.Context, agent coreagent.AgentID, brief *coreloop.Unblock, st *reconcile.State, final coreloop.State) {
	if brief == nil || final.Remedy == "" {
		return
	}
	for _, b := range st.Blockers {
		if string(b.Kind) == brief.Kind && b.Subject == brief.Subject {
			return
		}
	}
	sum := sha256.Sum256([]byte(brief.Kind + "\x00" + final.Remedy))
	entry := memory.Entry{
		ID:        "remedy-" + hex.EncodeToString(sum[:8]),
		AgentID:   agent,
		Tier:      string(memory.TierSemantic),
		Domain:    brief.Kind,
		Content:   final.Remedy,
		Sources:   []string{final.LoopID},
		CreatedAt: s.now(),
	}
	if err := s.store.SaveMemorySemantic(ctx, entry); err != nil {
		slog.Warn("store remedy", "kind", brief.Kind, "error", err)
	}
}
