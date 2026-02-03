package api

import (
	"context"

	"github.com/diogoX451/archon/pkg/types"
)

type RuleStore interface {
	ListRules(ctx context.Context) ([]types.RuleDef, error)
	GetRule(ctx context.Context, aType, bType string) (*types.RuleDef, error)
}
