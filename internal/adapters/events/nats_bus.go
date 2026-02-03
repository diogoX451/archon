package events

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/diogoX451/archon/internal/core/domain"
	"github.com/diogoX451/archon/internal/core/ports"
	"github.com/diogoX451/archon/internal/events"
	natsevents "github.com/diogoX451/archon/internal/events/nats"
)

// EventBusImpl adapta NATS para interface do Core
type EventBusImpl struct {
	bus *natsevents.NATSBus
}

var _ ports.EventBus = (*EventBusImpl)(nil)

func NewEventBus(bus *natsevents.NATSBus) *EventBusImpl {
	return &EventBusImpl{bus: bus}
}

func (e *EventBusImpl) PublishInteraction(ctx context.Context, netID string, pair domain.ActivePair) error {
	event := struct {
		NetID  string `json:"net_id"`
		AgentA string `json:"agent_a"`
		AgentB string `json:"agent_b"`
	}{
		NetID:  netID,
		AgentA: pair.AgentAID,
		AgentB: pair.AgentBID,
	}
	return e.bus.PublishEvent(ctx, "archon.interaction.pending", event)
}

func (e *EventBusImpl) PublishNeed(ctx context.Context, netID string, need domain.ExternalNeed) error {
	event := struct {
		NetID string              `json:"net_id"`
		Need  domain.ExternalNeed `json:"need"`
	}{
		NetID: netID,
		Need:  need,
	}
	return e.bus.PublishEvent(ctx, "archon.need."+need.Type, event)
}

func (e *EventBusImpl) PublishResult(ctx context.Context, netID string, output domain.Data) error {
	event := struct {
		NetID  string      `json:"net_id"`
		Output domain.Data `json:"output"`
	}{
		NetID:  netID,
		Output: output,
	}
	return e.bus.PublishEvent(ctx, "archon.result.completed", event)
}

func (e *EventBusImpl) SubscribeInteractions(ctx context.Context, handler ports.InteractionHandler) error {
	_, err := e.bus.Subscribe("archon.interaction.pending", func(ctx context.Context, msg events.Message) error {
		var event struct {
			NetID  string `json:"net_id"`
			AgentA string `json:"agent_a"`
			AgentB string `json:"agent_b"`
		}
		if err := json.Unmarshal(msg.Data(), &event); err != nil {
			return err
		}

		pair := domain.ActivePair{
			AgentAID: event.AgentA,
			AgentBID: event.AgentB,
		}

		if err := handler(ctx, event.NetID, pair); err != nil {
			return err
		}
		return msg.Ack()
	})
	return err
}

func (e *EventBusImpl) SubscribeResponses(ctx context.Context, handler ports.ResponseHandler) error {
	_, err := e.bus.Subscribe("archon.response.*", func(ctx context.Context, msg events.Message) error {
		// Extrai correlationID do subject
		// archon.response.corr_123 -> corr_123
		subject := msg.Subject()
		parts := strings.Split(subject, ".")
		correlationID := parts[len(parts)-1]

		if err := handler(ctx, correlationID, msg.Data()); err != nil {
			return err
		}
		return msg.Ack()
	})
	return err
}

func (e *EventBusImpl) Close() error {
	return e.bus.Close()
}

func (e *EventBusImpl) PublishEvent(ctx context.Context, subject string, event interface{}) error {
	return e.bus.PublishEvent(ctx, subject, event)
}
