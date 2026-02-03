package domain

import (
	"fmt"
	"sort"
)

// Rule define uma regra de interação entre dois símbolos.
// A regra é aplicada a um par ativo (porta principal conectada).
type Rule struct {
	AgentAType string
	AgentBType string

	Agents      []RuleAgent
	Connections []RuleConnection
	Interface   []RuleInterface
}

type RuleAgent struct {
	ID     string
	Type   string
	Config Data
}

type RulePortRef struct {
	AgentID string
	Port    string
}

type RuleConnection struct {
	From RulePortRef
	To   RulePortRef
}

type RuleExternalPort struct {
	Side string // "A" ou "B"
	Port string
}

type RuleInterface struct {
	External RuleExternalPort
	Internal RulePortRef
}

type RuleRegistry struct {
	rules map[string]Rule
}

func NewRuleRegistry() *RuleRegistry {
	return &RuleRegistry{rules: make(map[string]Rule)}
}

func (r *RuleRegistry) Register(rule Rule) error {
	if rule.AgentAType == "" || rule.AgentBType == "" {
		return fmt.Errorf("rule types required")
	}
	if rule.AgentAType == rule.AgentBType {
		return fmt.Errorf("no rule for identical symbols: %s", rule.AgentAType)
	}
	key := ruleKey(rule.AgentAType, rule.AgentBType)
	if _, exists := r.rules[key]; exists {
		return fmt.Errorf("rule already exists for %s,%s", rule.AgentAType, rule.AgentBType)
	}

	ids := make(map[string]bool)
	for _, a := range rule.Agents {
		if a.ID == "" || a.Type == "" {
			return fmt.Errorf("rule agent must have id and type")
		}
		if ids[a.ID] {
			return fmt.Errorf("duplicate rule agent id: %s", a.ID)
		}
		ids[a.ID] = true
	}

	r.rules[key] = rule
	return nil
}

func (r *RuleRegistry) Find(aType, bType string) (Rule, bool, bool) {
	key := ruleKey(aType, bType)
	rule, ok := r.rules[key]
	if !ok {
		return Rule{}, false, false
	}
	swapped := !(rule.AgentAType == aType && rule.AgentBType == bType)
	return rule, swapped, true
}

func ruleKey(a, b string) string {
	types := []string{a, b}
	sort.Strings(types)
	return types[0] + "|" + types[1]
}
