package delivery

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/entity"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/bsenel/karakuri/internal/core/vfs"
	"github.com/bsenel/karakuri/internal/feature/artifact"
	platformagent "github.com/bsenel/karakuri/internal/platform/agent"
	"github.com/bsenel/karakuri/internal/platform/storage"
)

type Reviewer struct {
	factory  *platformagent.Factory
	artifact *artifact.Service
	store    storage.StorageAdapter
	events   *event.Hub
}

func NewReviewer(factory *platformagent.Factory, art *artifact.Service, store storage.StorageAdapter, events *event.Hub) *Reviewer {
	return &Reviewer{factory: factory, artifact: art, store: store, events: events}
}

func (r *Reviewer) RunReview(ctx context.Context, sessionSHA, role, artifactSHA, implContent string) (string, error) {
	prompt := fmt.Sprintf("You are a %s. Review the following implementation and respond with APPROVED or REJECTED followed by feedback.\n\n%s", role, implContent)
	ag, err := r.factory.NewWithSession(ctx, sessionSHA, agent.Input{Role: role, Provider: "claude"})
	if err != nil {
		return "", err
	}
	out, err := ag.Run(ctx, agent.Input{
		Role: role, SystemPrompt: prompt, UserPrompt: implContent, Temperature: 0.2, Provider: "claude",
	})
	if err != nil {
		return "", err
	}
	verdict := "approved"
	if len(out.Content) > 8 && out.Content[:8] == "REJECTED" {
		verdict = "rejected"
	}
	reviewSHA, _ := newSHA()
	review := entity.Review{
		SHA: reviewSHA, SessionSHA: sessionSHA, ArtifactSHA: artifactSHA,
		Role: role, Verdict: verdict, Feedback: out.Content, CreatedAt: time.Now().UTC(),
	}
	_ = r.store.SaveReview(ctx, review)
	_ = r.events.Publish(ctx, event.Event{
		Type: event.ReviewCompleted, SessionSHA: sessionSHA,
		Payload: map[string]any{"role": role, "verdict": verdict, "artifact_sha": artifactSHA},
		Timestamp: time.Now().UTC(),
	})
	if verdict == "approved" {
		_ = r.artifact.Approve(ctx, artifactSHA)
	} else {
		_ = r.store.UpdateArtifactStatus(ctx, artifactSHA, vfs.StatusRejected)
	}
	return verdict, nil
}

func newSHA() (string, error) {
	b := make([]byte, 8)
	_, err := rand.Read(b)
	return hex.EncodeToString(b), err
}
