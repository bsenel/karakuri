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

func (h *EventsHandler) Stream(w http.ResponseWriter, r *http.Request) {
	sha := chi.URLParam(r, "sha")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, unsub, err := h.Hub.Subscribe(r.Context(), sha)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
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
