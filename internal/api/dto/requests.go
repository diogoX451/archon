package dto

import (
	"encoding/json"

	"github.com/diogoX451/archon/pkg/types"
)

type CreateWorkflowRequest struct {
	UserID      string             `json:"user_id"`
	Agents      []AgentConfig      `json:"agents"`
	Connections []ConnectionConfig `json:"connections"`
	Input       json.RawMessage    `json:"input"`
}

type AgentConfig struct {
	ID     string          `json:"id"`
	Type   string          `json:"type"`
	Config json.RawMessage `json:"config,omitempty"`
}

type ConnectionConfig struct {
	From string `json:"from"`
	To   string `json:"to"`
	As   string `json:"as,omitempty"`
}

type WebhookResponseRequest struct {
	Payload json.RawMessage `json:"payload"`
}

type AddAgentRequest struct {
	ID     string          `json:"id"`
	Type   string          `json:"type"`
	Config json.RawMessage `json:"config,omitempty"`
}

type ConnectRequest struct {
	From PortRef `json:"from"`
	To   PortRef `json:"to"`
}

type PortRef struct {
	AgentID string `json:"agent_id"`
	Port    string `json:"port"`
}

type DefineRuleRequest struct {
	Rule types.RuleDef `json:"rule"`
}
