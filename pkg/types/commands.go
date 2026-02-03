package types

import "time"

type PortRef struct {
	AgentID string `json:"agent_id"`
	Port    string `json:"port"`
}

type AddAgentEvent struct {
	WorkflowID string    `json:"workflow_id"`
	Agent      AgentDef  `json:"agent"`
	CreatedAt  time.Time `json:"created_at"`
}

type ConnectEvent struct {
	WorkflowID string    `json:"workflow_id"`
	From       PortRef   `json:"from"`
	To         PortRef   `json:"to"`
	CreatedAt  time.Time `json:"created_at"`
}

type DefineRuleEvent struct {
	Rule      RuleDef   `json:"rule"`
	CreatedAt time.Time `json:"created_at"`
}

type RuleDef struct {
	AgentAType  string              `json:"agent_a_type"`
	AgentBType  string              `json:"agent_b_type"`
	Agents      []AgentDef          `json:"agents"`
	Connections []RuleConnectionDef `json:"connections"`
	Interface   []RuleInterfaceDef  `json:"interface"`
}

type RuleConnectionDef struct {
	From PortRef `json:"from"`
	To   PortRef `json:"to"`
}

type RuleInterfaceDef struct {
	External RuleExternalPortDef `json:"external"`
	Internal PortRef             `json:"internal"`
}

type RuleExternalPortDef struct {
	Side string `json:"side"`
	Port string `json:"port"`
}
