package dto

import "github.com/diogoX451/archon/pkg/types"

type PlanRequest struct {
	Rules       []types.RuleDef       `json:"rules"`
	Workflow    CreateWorkflowRequest `json:"workflow"`
	AddAgents   []AddAgentRequest     `json:"add_agents,omitempty"`
	Connections []ConnectRequest      `json:"connections,omitempty"`
}
