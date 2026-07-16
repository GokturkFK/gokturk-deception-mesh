package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/GokturkFK/gokturk-deception-mesh/internal/trap"
)

// trapStore, handler'larin ihtiyac duydugu kalici katman sozlesmesidir.
// Arayuz olarak tutulur ki handler'lar DB olmadan (fake ile) test edilebilsin.
type trapStore interface {
	CreateTrap(ctx context.Context, t trap.Trap) (trap.Trap, error)
	ListTraps(ctx context.Context) ([]trap.Trap, error)
}

// apiServer, /api/v1 handler'larinin bagimliliklarini tutar.
type apiServer struct {
	provider trap.Provider
	store    trapStore
	alerts   alertLister
	logger   *slog.Logger
}

type createTrapRequest struct {
	CreatedBy string `json:"created_by,omitempty"`
}

// createTrapResponse, secret'i yalnizca bu cevapta bir kez dondurur.
type createTrapResponse struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Username  string    `json:"username"`
	Secret    string    `json:"secret"`
	CreatedBy string    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// handleCreateTrap, POST /api/v1/traps — bir canary provision eder, DB'ye yazar
// ve secret dahil artifacts'i (bir kez) dondurur.
func (a *apiServer) handleCreateTrap(w http.ResponseWriter, r *http.Request) {
	var req createTrapRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "gecersiz istek govdesi")
			return
		}
	}

	createdBy := req.CreatedBy
	if createdBy == "" {
		createdBy = "system"
	}

	t, artifacts, err := a.provider.Provision(r.Context(), createdBy)
	if err != nil {
		a.logger.Error("tuzak uretilemedi", "err", err)
		writeError(w, http.StatusInternalServerError, "tuzak uretilemedi")
		return
	}

	saved, err := a.store.CreateTrap(r.Context(), *t)
	if err != nil {
		a.logger.Error("tuzak kaydedilemedi", "err", err)
		writeError(w, http.StatusInternalServerError, "tuzak kaydedilemedi")
		return
	}

	writeJSON(w, http.StatusCreated, createTrapResponse{
		ID:        saved.ID,
		Type:      saved.Type,
		Username:  saved.Username,
		Secret:    artifacts.Secret,
		CreatedBy: saved.CreatedBy,
		CreatedAt: saved.CreatedAt,
	})
}

// handleListTraps, GET /api/v1/traps — tuzaklari listeler (secret'siz).
func (a *apiServer) handleListTraps(w http.ResponseWriter, r *http.Request) {
	traps, err := a.store.ListTraps(r.Context())
	if err != nil {
		a.logger.Error("tuzaklar listelenemedi", "err", err)
		writeError(w, http.StatusInternalServerError, "tuzaklar listelenemedi")
		return
	}
	if traps == nil {
		traps = []trap.Trap{}
	}
	writeJSON(w, http.StatusOK, traps)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
