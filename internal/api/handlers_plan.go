package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/diogoX451/archon/internal/api/dto"
	"github.com/diogoX451/archon/pkg/types"
	"github.com/google/uuid"
)

// Handler: POST /api/v1/plan
func (s *Server) handlePlan(w http.ResponseWriter, r *http.Request) {
	var req dto.PlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if err := validatePlan(req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_PLAN", err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// 1) Rules
	for _, rule := range req.Rules {
		event := types.DefineRuleEvent{Rule: rule, CreatedAt: time.Now()}
		if err := s.eventBus.PublishEvent(ctx, "archon.command.define_rule", event); err != nil {
			respondError(w, http.StatusInternalServerError, "PUBLISH_FAILED", err.Error())
			return
		}
	}

	// 2) Workflow spawn
	spawnEvent, err := buildSpawnEvent(req.Workflow)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_WORKFLOW", err.Error())
		return
	}
	if err := s.eventBus.PublishEvent(ctx, "archon.command.spawn", spawnEvent); err != nil {
		respondError(w, http.StatusInternalServerError, "PUBLISH_FAILED", err.Error())
		return
	}

	// 3) Add agents
	for _, agent := range req.AddAgents {
		event := types.AddAgentEvent{
			WorkflowID: spawnEvent.WorkflowID,
			Agent: types.AgentDef{
				ID:     agent.ID,
				Type:   agent.Type,
				Config: agent.Config,
			},
			CreatedAt: time.Now(),
		}
		if err := s.eventBus.PublishEvent(ctx, "archon.command.add_agent", event); err != nil {
			respondError(w, http.StatusInternalServerError, "PUBLISH_FAILED", err.Error())
			return
		}
	}

	// 4) Connections
	for _, conn := range req.Connections {
		event := types.ConnectEvent{
			WorkflowID: spawnEvent.WorkflowID,
			From:       types.PortRef(conn.From),
			To:         types.PortRef(conn.To),
			CreatedAt:  time.Now(),
		}
		if err := s.eventBus.PublishEvent(ctx, "archon.command.connect", event); err != nil {
			respondError(w, http.StatusInternalServerError, "PUBLISH_FAILED", err.Error())
			return
		}
	}

	respondJSON(w, http.StatusAccepted, map[string]string{
		"workflow_id": spawnEvent.WorkflowID,
		"status":      "planned",
	})
}

func validatePlan(req dto.PlanRequest) error {
	if req.Workflow.UserID == "" {
		return errors.New("workflow.user_id is required")
	}
	if len(req.Workflow.Agents) == 0 {
		return errors.New("workflow.agents is required")
	}
	return nil
}

func buildSpawnEvent(req dto.CreateWorkflowRequest) (types.SpawnEvent, error) {
	if req.UserID == "" {
		return types.SpawnEvent{}, errors.New("user_id is required")
	}
	workflowID := uuid.NewString()

	blueprint := types.Blueprint{
		Agents:      make([]types.AgentDef, len(req.Agents)),
		Connections: make([]types.ConnectionDef, len(req.Connections)),
	}
	for i, a := range req.Agents {
		blueprint.Agents[i] = types.AgentDef{ID: a.ID, Type: a.Type, Config: a.Config}
	}
	for i, c := range req.Connections {
		blueprint.Connections[i] = types.ConnectionDef{From: c.From, To: c.To, As: c.As}
	}

	return types.SpawnEvent{
		WorkflowID: workflowID,
		Blueprint:  blueprint,
		Input:      req.Input,
		CreatedAt:  time.Now(),
	}, nil
}
