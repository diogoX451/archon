package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/diogoX451/archon/internal/api/dto"
	"github.com/diogoX451/archon/pkg/types"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Handler: POST /api/v1/workflows
func (s *Server) handleCreateWorkflow(w http.ResponseWriter, r *http.Request) {
	var req dto.CreateWorkflowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	// Validação básica
	if req.UserID == "" {
		respondError(w, http.StatusBadRequest, "MISSING_USER_ID", "user_id is required")
		return
	}
	if len(req.Agents) == 0 {
		respondError(w, http.StatusBadRequest, "MISSING_AGENTS", "at least one agent required")
		return
	}

	// Gera ID único
	workflowID := uuid.New().String()

	// Constrói blueprint
	blueprint := types.Blueprint{
		Agents:      make([]types.AgentDef, len(req.Agents)),
		Connections: make([]types.ConnectionDef, len(req.Connections)),
	}

	for i, a := range req.Agents {
		blueprint.Agents[i] = types.AgentDef{
			ID:     a.ID,
			Type:   a.Type,
			Config: a.Config,
		}
	}

	for i, c := range req.Connections {
		blueprint.Connections[i] = types.ConnectionDef{
			From: c.From,
			To:   c.To,
			As:   c.As,
		}
	}

	// Cria evento de spawn
	event := types.SpawnEvent{
		WorkflowID: workflowID,
		Blueprint:  blueprint,
		Input:      req.Input,
		CreatedAt:  time.Now(),
	}

	// 🚀 PUBLICA EVENTO REAL NO NATS!
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := s.eventBus.PublishEvent(ctx, "archon.command.spawn", event); err != nil {
		respondError(w, http.StatusInternalServerError, "PUBLISH_FAILED", err.Error())
		return
	}

	// Responde 202 Accepted (processamento assíncrono)
	respondJSON(w, http.StatusAccepted, dto.CreateWorkflowResponse{
		WorkflowID: workflowID,
		Status:     "spawning",
		CreatedAt:  event.CreatedAt,
	})
}

// Handler: GET /api/v1/workflows/{id}
func (s *Server) handleGetWorkflow(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "id")

	// TODO: Implementar query no repository
	_ = workflowID

	respondError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "query not yet implemented")
}

// Handler: GET /api/v1/workflows/{id}/status
func (s *Server) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "id")

	// TODO: Buscar status no Redis
	_ = workflowID

	// Placeholder
	respondJSON(w, http.StatusOK, dto.WorkflowStatusResponse{
		WorkflowID: workflowID,
		Status:     "pending",
		UpdatedAt:  time.Now(),
	})
}

// Handler: GET /api/v1/workflows/{id}/result
func (s *Server) handleGetResult(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "id")
	_ = workflowID

	// TODO: Buscar resultado quando completed
	respondError(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "result query not yet implemented")
}
