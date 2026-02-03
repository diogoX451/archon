package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/diogoX451/archon/internal/api/dto"
	"github.com/diogoX451/archon/pkg/types"
	"github.com/go-chi/chi/v5"
)

// Handler: POST /api/v1/webhooks/needs/{correlation_id}
// Executores externos (HTTP, SQL, etc) chamam aqui para devolver resposta
func (s *Server) handleNeedResponse(w http.ResponseWriter, r *http.Request) {
	correlationID := chi.URLParam(r, "correlation_id")
	if correlationID == "" {
		respondError(w, http.StatusBadRequest, "MISSING_CORRELATION_ID", "correlation_id is required")
		return
	}

	var req dto.WebhookResponseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	// Cria evento de resposta
	responseEvent := types.ResponseEvent{
		CorrelationID: correlationID,
		Payload:       req.Payload,
		ReceivedAt:    time.Now(),
	}

	// 🚀 PUBLICA EVENTO REAL NO NATS!
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := s.eventBus.PublishEvent(ctx, "archon.response."+correlationID, responseEvent); err != nil {
		respondError(w, http.StatusInternalServerError, "PUBLISH_FAILED", err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"status":         "accepted",
		"correlation_id": correlationID,
	})
}
