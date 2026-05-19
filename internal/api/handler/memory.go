package handler

import (
	"encoding/json"
	"net/http"

	"github.com/bsenel/karakuri/internal/core/memory"
	featurememory "github.com/bsenel/karakuri/internal/feature/memory"
)

type MemoryHandler struct {
	Memory *featurememory.Service
}

func (h *MemoryHandler) Store(w http.ResponseWriter, r *http.Request) {
	var entry memory.Entry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.Memory.Store(r.Context(), entry); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "stored"})
}

func (h *MemoryHandler) Recall(w http.ResponseWriter, r *http.Request) {
	var q memory.Query
	if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	entries, err := h.Memory.Recall(r.Context(), q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, entries)
}

func (h *MemoryHandler) Forget(w http.ResponseWriter, r *http.Request) {
	var p memory.RetentionPolicy
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.Memory.Forget(r.Context(), p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"status": "forgotten"})
}
