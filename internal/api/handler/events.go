package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/bsenel/karakuri/internal/core/event"
	"github.com/go-chi/chi/v5"
)

type EventsHandler struct {
	Hub *event.Hub
}

func (h *EventsHandler) StreamObjective(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.stream(w, r, "obj:"+id)
}

func (h *EventsHandler) StreamTwin(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.stream(w, r, "twin:"+id)
}

func (h *EventsHandler) stream(w http.ResponseWriter, r *http.Request, key string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

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
			data, _ := json.Marshal(evt)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
