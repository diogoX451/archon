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

func (s *Server) handleAddAgent(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "id")

	var req dto.AddAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if req.ID == "" || req.Type == "" {
		respondError(w, http.StatusBadRequest, "INVALID_AGENT", "id and type are required")
		return
	}

	event := types.AddAgentEvent{
		WorkflowID: workflowID,
		Agent: types.AgentDef{
			ID:     req.ID,
			Type:   req.Type,
			Config: req.Config,
		},
		CreatedAt: time.Now(),
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := s.eventBus.PublishEvent(ctx, "archon.command.add_agent", event); err != nil {
		respondError(w, http.StatusInternalServerError, "PUBLISH_FAILED", err.Error())
		return
	}

	respondJSON(w, http.StatusAccepted, event)
}

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	workflowID := chi.URLParam(r, "id")

	var req dto.ConnectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if req.From.AgentID == "" || req.From.Port == "" || req.To.AgentID == "" || req.To.Port == "" {
		respondError(w, http.StatusBadRequest, "INVALID_CONNECTION", "from/to agent_id and port are required")
		return
	}

	event := types.ConnectEvent{
		WorkflowID: workflowID,
		From:       types.PortRef(req.From),
		To:         types.PortRef(req.To),
		CreatedAt:  time.Now(),
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := s.eventBus.PublishEvent(ctx, "archon.command.connect", event); err != nil {
		respondError(w, http.StatusInternalServerError, "PUBLISH_FAILED", err.Error())
		return
	}

	respondJSON(w, http.StatusAccepted, event)
}

func (s *Server) handleDefineRule(w http.ResponseWriter, r *http.Request) {
	var req dto.DefineRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if req.Rule.AgentAType == "" || req.Rule.AgentBType == "" {
		respondError(w, http.StatusBadRequest, "INVALID_RULE", "agent_a_type and agent_b_type are required")
		return
	}

	event := types.DefineRuleEvent{
		Rule:      req.Rule,
		CreatedAt: time.Now(),
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := s.eventBus.PublishEvent(ctx, "archon.command.define_rule", event); err != nil {
		respondError(w, http.StatusInternalServerError, "PUBLISH_FAILED", err.Error())
		return
	}

	respondJSON(w, http.StatusAccepted, event)
}

func (s *Server) handleListRules(w http.ResponseWriter, r *http.Request) {
	if s.ruleStore == nil {
		respondError(w, http.StatusNotImplemented, "RULE_STORE_DISABLED", "rule store not configured")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	rules, err := s.ruleStore.ListRules(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "RULES_LIST_FAILED", err.Error())
		return
	}
	respondJSON(w, http.StatusOK, rules)
}

func (s *Server) handleGetRule(w http.ResponseWriter, r *http.Request) {
	if s.ruleStore == nil {
		respondError(w, http.StatusNotImplemented, "RULE_STORE_DISABLED", "rule store not configured")
		return
	}
	aType := chi.URLParam(r, "a")
	bType := chi.URLParam(r, "b")
	if aType == "" || bType == "" {
		respondError(w, http.StatusBadRequest, "INVALID_RULE", "a and b are required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	rule, err := s.ruleStore.GetRule(ctx, aType, bType)
	if err != nil {
		respondError(w, http.StatusNotFound, "RULE_NOT_FOUND", err.Error())
		return
	}
	respondJSON(w, http.StatusOK, rule)
}
