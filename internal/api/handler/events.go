package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/bsenel/karakuri/auth"
	karakuriauth "github.com/bsenel/karakuri/internal/auth"
	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/go-chi/chi/v5"
)

type EventsHandler struct {
	Hub *event.Hub

	// Scopes and Containers narrow the global stream to what the caller may
	// see. Both nil on a deployment that never wires them, in which case the
	// per-twin streams behave as they always did and the global one is
	// unfiltered — which is correct, because with no authorizer there are no
	// scopes to be confined to.
	Scopes     karakuriauth.ScopeAuthorizer
	Containers karakuriauth.ScopeResolver
}

func (h *EventsHandler) StreamObjective(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	// The route check already answered whether this caller may read this
	// objective, so every event on this key is one they may see.
	h.stream(w, r, "obj:"+id, nil)
}

func (h *EventsHandler) StreamTwin(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.stream(w, r, "twin:"+id, nil)
}

// StreamAll is the deployment-wide stream a dashboard follows.
//
// Unlike the two above, the route check cannot settle what this caller may see:
// the key names everything, so the question has to be asked again for each
// event. A filter that fails to classify an event withholds it — see
// karakuriauth.StreamFilter for why that direction and not the other.
//
// GET /api/v1/events
func (h *EventsHandler) StreamAll(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.PrincipalFromContext(r.Context())
	filter, err := karakuriauth.NewStreamFilter(r.Context(), h.Scopes, h.Containers, principal.ID)
	if err != nil {
		// Grants that cannot be read mean the stream cannot be filtered, and an
		// unfiltered global stream is the thing this endpoint exists not to be.
		http.Error(w, "authorization could not be evaluated", http.StatusInternalServerError)
		return
	}
	h.stream(w, r, "_global", filter)
}

func (h *EventsHandler) stream(w http.ResponseWriter, r *http.Request, key string, filter *karakuriauth.StreamFilter) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Flush the headers before waiting for the first event. Without this,
	// net/http buffers the response and the client sees nothing at all until
	// something happens to be emitted — so EventSource.onopen never fires on an
	// idle stream, and a plain HTTP client blocks in Do() indefinitely.
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch, unsub := h.Hub.Subscribe(r.Context(), key)
	defer unsub()

	for {
		select {
		case <-r.Context().Done():
			return
		case evt, open := <-ch:
			if !open {
				return
			}
			if filter != nil && !filter.Allow(r.Context(), filterable(evt)) {
				continue
			}
			data, _ := json.Marshal(evt)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// filterable projects an event onto what the filter needs, which is the only
// place the two vocabularies meet.
//
// The payload reads are why this is here rather than in internal/auth: a cost
// event carries the containers it was recorded under, and a quota_pressure
// event carries the key it is about and nothing else. Both are conventions of
// the publishers in this repository, and a filter that lives in the auth
// package should not have to know them.
func filterable(evt event.Event) karakuriauth.Event {
	out := karakuriauth.Event{TwinID: evt.TwinID, ObjectiveID: evt.ObjectiveID}
	if labels, ok := evt.Payload["labels"].([]string); ok {
		out.Labels = labels
	} else if raw, ok := evt.Payload["labels"].([]any); ok {
		// Anything that has been through JSON arrives as []any. The hub
		// delivers Go values in-process, so both shapes are real.
		for _, v := range raw {
			if s, ok := v.(string); ok {
				out.Labels = append(out.Labels, s)
			}
		}
	}
	if key, ok := evt.Payload["key"].(string); ok {
		out.Subject = key
	}
	return out
}
