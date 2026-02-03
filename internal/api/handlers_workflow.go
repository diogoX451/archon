package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
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

	if s.stateStore == nil {
		respondError(w, http.StatusNotImplemented, "STATE_STORE_DISABLED", "state store not configured")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	state, err := s.stateStore.GetWorkflow(ctx, types.WorkflowID(workflowID))
	if err != nil {
		if isWorkflowNotFound(err) {
			respondError(w, http.StatusNotFound, "WORKFLOW_NOT_FOUND", err.Error())
			return
		}
		respondError(w, http.StatusInternalServerError, "WORKFLOW_QUERY_FAILED", err.Error())
		return
	}

	respondJSON(w, http.StatusOK, state)
}

// Handler: GET /api/v1/workflows/{id}/status
func (s *Server) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "id")

	if s.stateStore == nil {
		respondError(w, http.StatusNotImplemented, "STATE_STORE_DISABLED", "state store not configured")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	state, err := s.stateStore.GetWorkflow(ctx, types.WorkflowID(workflowID))
	if err != nil {
		if isWorkflowNotFound(err) {
			respondError(w, http.StatusNotFound, "WORKFLOW_NOT_FOUND", err.Error())
			return
		}
		respondError(w, http.StatusInternalServerError, "WORKFLOW_QUERY_FAILED", err.Error())
		return
	}

	status := deriveWorkflowStatus(state)

	respondJSON(w, http.StatusOK, dto.WorkflowStatusResponse{
		WorkflowID: workflowID,
		Status:     string(status),
		UpdatedAt:  state.UpdatedAt,
	})
}

// Handler: GET /api/v1/workflows/{id}/result
func (s *Server) handleGetResult(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "id")

	if s.stateStore == nil {
		respondError(w, http.StatusNotImplemented, "STATE_STORE_DISABLED", "state store not configured")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	state, err := s.stateStore.GetWorkflow(ctx, types.WorkflowID(workflowID))
	if err != nil {
		if isWorkflowNotFound(err) {
			respondError(w, http.StatusNotFound, "WORKFLOW_NOT_FOUND", err.Error())
			return
		}
		respondError(w, http.StatusInternalServerError, "WORKFLOW_QUERY_FAILED", err.Error())
		return
	}

	status := deriveWorkflowStatus(state)
	output, finishedAt, ok := latestCompletedOutput(state)

	response := dto.WorkflowResultResponse{
		WorkflowID: workflowID,
		Status:     string(status),
	}

	if status == types.WorkflowCompleted && ok {
		response.Output = output
		response.FinishedAt = finishedAt
	}
	if status == types.WorkflowFailed {
		response.FinishedAt = state.UpdatedAt
	}

	if status == types.WorkflowCompleted || status == types.WorkflowFailed {
		respondJSON(w, http.StatusOK, response)
		return
	}

	respondJSON(w, http.StatusAccepted, response)
}

func deriveWorkflowStatus(state *types.WorkflowState) types.WorkflowStatus {
	if state == nil {
		return types.WorkflowFailed
	}
	if len(state.Agents) == 0 {
		if state.Status != "" {
			return state.Status
		}
		return types.WorkflowRunning
	}

	hasWaiting := len(state.Waiting) > 0
	hasRunning := false
	completedCount := 0

	for _, agent := range state.Agents {
		switch agent.State {
		case types.StateFailed:
			return types.WorkflowFailed
		case types.StateWaiting:
			hasWaiting = true
		case types.StateRunning:
			hasRunning = true
		case types.StateCompleted:
			completedCount++
		}
	}

	if completedCount == len(state.Agents) {
		return types.WorkflowCompleted
	}
	if hasWaiting {
		return types.WorkflowWaiting
	}
	if hasRunning {
		return types.WorkflowRunning
	}
	if state.Status != "" {
		return state.Status
	}
	return types.WorkflowRunning
}

func latestCompletedOutput(state *types.WorkflowState) (types.Data, time.Time, bool) {
	if state == nil {
		return nil, time.Time{}, false
	}

	var (
		bestOutput types.Data
		bestTime   time.Time
		found      bool
	)

	for _, agent := range state.Agents {
		if agent.State != types.StateCompleted {
			continue
		}
		if len(agent.Output) == 0 {
			continue
		}
		if !found || agent.UpdatedAt.After(bestTime) {
			bestOutput = agent.Output
			bestTime = agent.UpdatedAt
			found = true
		}
	}

	return bestOutput, bestTime, found
}

func isWorkflowNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "workflow not found")
}
