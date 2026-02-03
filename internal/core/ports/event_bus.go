package ports

import (
	"context"

	"github.com/diogoX451/archon/internal/core/domain"
)

// EventBus abstração de mensageria
type EventBus interface {
	// Publicação
	PublishInteraction(ctx context.Context, netID string, pair domain.ActivePair) error
	PublishNeed(ctx context.Context, netID string, need domain.ExternalNeed) error
	PublishResult(ctx context.Context, netID string, output domain.Data) error
	PublishEvent(ctx context.Context, subject string, payload interface{}) error

	// Subscrição (para workers)
	SubscribeInteractions(ctx context.Context, handler InteractionHandler) error
	SubscribeResponses(ctx context.Context, handler ResponseHandler) error

	Close() error
}

type InteractionHandler func(ctx context.Context, netID string, pair domain.ActivePair) error
type ResponseHandler func(ctx context.Context, correlationID string, data domain.Data) error
