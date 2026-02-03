package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/diogoX451/archon/internal/events"
	"github.com/diogoX451/archon/pkg/types"
	"github.com/nats-io/nats.go"
)

type NATSBus struct {
	conn *nats.Conn
	js   nats.JetStreamContext
}

// Verifica interface
var _ events.Bus = (*NATSBus)(nil)

type Config struct {
	URL           string
	MaxReconnects int
	ReconnectWait time.Duration
}

func New(cfg Config) (*NATSBus, error) {
	opts := []nats.Option{
		nats.MaxReconnects(cfg.MaxReconnects),
		nats.ReconnectWait(cfg.ReconnectWait),
		nats.Name("archon-bus"),
	}

	conn, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("nats connection failed: %w", err)
	}

	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("jetstream init failed: %w", err)
	}

	return &NATSBus{
		conn: conn,
		js:   js,
	}, nil
}

// CreateStream cria stream se não existir
func (n *NATSBus) CreateStream(cfg events.StreamConfig) error {
	storage := nats.FileStorage
	if cfg.Storage == events.StorageMemory {
		storage = nats.MemoryStorage
	}

	retention := nats.LimitsPolicy
	switch cfg.Retention {
	case events.RetentionInterest:
		retention = nats.InterestPolicy
	case events.RetentionWorkQueue:
		retention = nats.WorkQueuePolicy
	}

	_, err := n.js.AddStream(&nats.StreamConfig{
		Name:      cfg.Name,
		Subjects:  cfg.Subjects,
		Retention: retention,
		MaxMsgs:   cfg.MaxMsgs,
		MaxBytes:  cfg.MaxBytes,
		MaxAge:    cfg.MaxAge,
		Storage:   storage,
		Replicas:  cfg.Replicas,
	})

	if err == nats.ErrStreamNameAlreadyInUse {
		return nil // Já existe, ok
	}

	return err
}

// SetupArchonStreams cria streams padrão do Archon
func (n *NATSBus) SetupArchonStreams() error {
	// Stream: Comandos (criar workflow)
	if err := n.CreateStream(events.StreamConfig{
		Name:      "ARCHON_COMMANDS",
		Subjects:  []string{"archon.command.>"},
		Retention: events.RetentionLimits, // Permite múltiplos consumers (UI)
		MaxMsgs:   10000,
		MaxAge:    24 * time.Hour,
		Storage:   events.StorageMemory,
	}); err != nil {
		return fmt.Errorf("commands stream: %w", err)
	}

	// Stream: Interações (core do algoritmo)
	if err := n.CreateStream(events.StreamConfig{
		Name:      "ARCHON_INTERACTIONS",
		Subjects:  []string{"archon.interaction.>"},
		Retention: interactionsRetention(),
		MaxMsgs:   100000,
		MaxAge:    24 * time.Hour,
		Storage:   events.StorageFile,
	}); err != nil {
		return fmt.Errorf("interactions stream: %w", err)
	}

	// Stream: Needs (pedidos ao mundo externo)
	if err := n.CreateStream(events.StreamConfig{
		Name:      "ARCHON_NEEDS",
		Subjects:  []string{"archon.need.>"},
		Retention: events.RetentionInterest, // Mantém até alguém consumir
		MaxMsgs:   100000,
		Storage:   events.StorageFile,
	}); err != nil {
		return fmt.Errorf("needs stream: %w", err)
	}

	// Stream: Respostas (do mundo externo de volta)
	if err := n.CreateStream(events.StreamConfig{
		Name:      "ARCHON_RESPONSES",
		Subjects:  []string{"archon.response.>"},
		Retention: events.RetentionLimits, // Permite múltiplos consumers (UI)
		MaxMsgs:   100000,
		MaxAge:    24 * time.Hour,
		Storage:   events.StorageMemory,
	}); err != nil {
		return fmt.Errorf("responses stream: %w", err)
	}

	// Stream: Resultados (output final)
	if err := n.CreateStream(events.StreamConfig{
		Name:      "ARCHON_RESULTS",
		Subjects:  []string{"archon.result.>"},
		Retention: events.RetentionLimits, // Mantém histórico
		MaxMsgs:   1000000,
		MaxAge:    7 * 24 * time.Hour,
		Storage:   events.StorageFile,
	}); err != nil {
		return fmt.Errorf("results stream: %w", err)
	}

	return nil
}

func interactionsRetention() events.RetentionPolicy {
	switch strings.ToLower(types.Getenv("ARCHON_INTERACTIONS_RETENTION", "workqueue")) {
	case "limits":
		return events.RetentionLimits
	case "interest":
		return events.RetentionInterest
	default:
		return events.RetentionWorkQueue
	}
}

// Publish envia mensagem bruta
func (n *NATSBus) Publish(ctx context.Context, subject string, payload []byte) error {
	_, err := n.js.Publish(subject, payload)
	return err
}

// PublishEvent serializa e envia
func (n *NATSBus) PublishEvent(ctx context.Context, subject string, event interface{}) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	return n.Publish(ctx, subject, data)
}

// Subscribe registra handler push
func (n *NATSBus) Subscribe(subject string, handler events.Handler) (events.Subscription, error) {
	durable := durableFromSubject(subject)
	callback := func(msg *nats.Msg) {
		wrapped := &natsMessage{msg: msg}
		ctx := context.Background()

		if err := handler(ctx, wrapped); err != nil {
			// Handler errou, não deu ack = redelivery automático
			return
		}
	}

	sub, err := n.js.Subscribe(subject, callback, nats.Durable(durable), nats.ManualAck())
	if err == nil {
		return &natsSubscription{sub: sub}, nil
	}

	if !strings.Contains(err.Error(), "filtered consumer not unique on workqueue stream") {
		return nil, err
	}

	stream, streamErr := n.js.StreamNameBySubject(subject)
	if streamErr != nil {
		return nil, err
	}

	for name := range n.js.ConsumerNames(stream) {
		info, infoErr := n.js.ConsumerInfo(stream, name)
		if infoErr != nil {
			continue
		}
		if info.Config.FilterSubject != subject {
			continue
		}

		bound, bindErr := n.js.Subscribe(subject, callback, nats.Bind(stream, name), nats.ManualAck())
		if bindErr != nil {
			return nil, bindErr
		}
		return &natsSubscription{sub: bound}, nil
	}

	return nil, err
}

func durableFromSubject(subject string) string {
	var b strings.Builder
	b.Grow(len(subject))
	for _, r := range subject {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// CreateConsumer cria consumer pull (para workers)
func (n *NATSBus) CreateConsumer(stream, consumer string, cfg events.ConsumerConfig) (events.Consumer, error) {
	deliver := nats.DeliverAllPolicy
	switch cfg.DeliverPolicy {
	case events.DeliverLast:
		deliver = nats.DeliverLastPolicy
	case events.DeliverNew:
		deliver = nats.DeliverNewPolicy
	}

	ack := nats.AckExplicitPolicy
	switch cfg.AckPolicy {
	case events.AckAll:
		ack = nats.AckAllPolicy
	case events.AckNone:
		ack = nats.AckNonePolicy
	}

	_, err := n.js.AddConsumer(stream, &nats.ConsumerConfig{
		Durable:       cfg.Durable,
		DeliverPolicy: deliver,
		AckPolicy:     ack,
		MaxAckPending: cfg.MaxAckPending,
		MaxDeliver:    cfg.MaxDeliver,
		AckWait:       cfg.AckWait,
	})

	if err != nil && err != nats.ErrConsumerNameAlreadyInUse {
		return nil, err
	}

	return &natsConsumer{
		js:       n.js,
		stream:   stream,
		consumer: consumer,
	}, nil
}

// DeleteStream remove stream
func (n *NATSBus) DeleteStream(name string) error {
	return n.js.DeleteStream(name)
}

// Close encerra conexão
func (n *NATSBus) Close() error {
	n.conn.Close()
	return nil
}

// --- Implementações internas ---

type natsMessage struct {
	msg *nats.Msg
}

func (m *natsMessage) Data() []byte {
	return m.msg.Data
}

func (m *natsMessage) Subject() string {
	return m.msg.Subject
}

func (m *natsMessage) Ack() error {
	return m.msg.Ack()
}

func (m *natsMessage) Nak(delay ...time.Duration) error {
	if len(delay) > 0 {
		return m.msg.NakWithDelay(delay[0])
	}
	return m.msg.Nak()
}

func (m *natsMessage) InProgress() error {
	return m.msg.InProgress()
}

func (m *natsMessage) Metadata() (*events.MsgMetadata, error) {
	meta, err := m.msg.Metadata()
	if err != nil {
		return nil, err
	}

	return &events.MsgMetadata{
		Sequence:   meta.Sequence.Consumer,
		Time:       meta.Timestamp,
		Stream:     meta.Stream,
		Consumer:   meta.Consumer,
		Deliveries: int(meta.NumDelivered),
	}, nil
}

type natsSubscription struct {
	sub *nats.Subscription
}

func (s *natsSubscription) Unsubscribe() error {
	return s.sub.Unsubscribe()
}

type natsConsumer struct {
	js       nats.JetStreamContext
	stream   string
	consumer string
}

func (c *natsConsumer) Fetch(batch int, timeout time.Duration) ([]events.Message, error) {
	sub, err := c.js.PullSubscribe("", c.consumer, nats.Bind(c.stream, c.consumer))
	if err != nil {
		return nil, err
	}
	defer sub.Unsubscribe()

	msgs, err := sub.Fetch(batch, nats.MaxWait(timeout))
	if err != nil {
		return nil, err
	}

	result := make([]events.Message, len(msgs))
	for i, msg := range msgs {
		result[i] = &natsMessage{msg: msg}
	}

	return result, nil
}
