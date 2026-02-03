package events

import (
	"context"
	"time"
)

// Bus abstração de fila de eventos
type Bus interface {
	// Publicação
	Publish(ctx context.Context, subject string, payload []byte) error
	PublishEvent(ctx context.Context, subject string, event interface{}) error

	// Subscrição push (callback)
	Subscribe(subject string, handler Handler) (Subscription, error)

	// Subscrição pull (para workers)
	CreateConsumer(stream, consumer string, cfg ConsumerConfig) (Consumer, error)

	// Streams
	CreateStream(cfg StreamConfig) error
	DeleteStream(name string) error

	// Utilidades
	Close() error
}

// Handler processa mensagens
type Handler func(ctx context.Context, msg Message) error

type Message interface {
	Data() []byte
	Subject() string
	Ack() error
	Nak(delay ...time.Duration) error
	InProgress() error
	Metadata() (*MsgMetadata, error)
}

type MsgMetadata struct {
	Sequence   uint64
	Time       time.Time
	Stream     string
	Consumer   string
	Deliveries int
}

type Subscription interface {
	Unsubscribe() error
}

type Consumer interface {
	Fetch(batch int, timeout time.Duration) ([]Message, error)
}

type ConsumerConfig struct {
	Durable       string
	DeliverPolicy DeliverPolicy
	AckPolicy     AckPolicy
	MaxAckPending int
	AckWait       time.Duration
	MaxDeliver    int
}

type DeliverPolicy int

const (
	DeliverAll DeliverPolicy = iota
	DeliverLast
	DeliverNew
)

type AckPolicy int

const (
	AckExplicit AckPolicy = iota
	AckAll
	AckNone
)

type StreamConfig struct {
	Name      string
	Subjects  []string
	Retention RetentionPolicy
	MaxMsgs   int64
	MaxBytes  int64
	MaxAge    time.Duration
	Storage   StorageType
	Replicas  int
}

type RetentionPolicy int

const (
	RetentionLimits RetentionPolicy = iota
	RetentionInterest
	RetentionWorkQueue
)

type StorageType int

const (
	StorageFile StorageType = iota
	StorageMemory
)
