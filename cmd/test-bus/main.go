package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/diogoX451/archon/internal/events"
	"github.com/diogoX451/archon/internal/events/nats"
	"github.com/diogoX451/archon/pkg/types"
)

func main() {
	bus, err := nats.New(nats.Config{
		URL:           "nats://localhost:4222",
		MaxReconnects: 10,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer bus.Close()

	// Setup streams
	if err := bus.SetupArchonStreams(); err != nil {
		log.Fatal("setup streams:", err)
	}
	fmt.Println("✅ Streams criados")

	ctx := context.Background()

	// Teste 1: Publicar evento
	event := types.SpawnEvent{
		WorkflowID: "test-1",
		Input:      []byte(`{"test": true}`),
		CreatedAt:  time.Now(),
	}

	if err := bus.PublishEvent(ctx, "archon.command.spawn", event); err != nil {
		log.Fatal("publish:", err)
	}
	fmt.Println("✅ Evento publicado")

	// Teste 2: Consumer pull
	consumer, err := bus.CreateConsumer("ARCHON_COMMANDS", "test-consumer", events.ConsumerConfig{
		Durable:       "test-consumer",
		DeliverPolicy: events.DeliverAll,
		AckPolicy:     events.AckExplicit,
		MaxAckPending: 100,
		AckWait:       30 * time.Second,
	})
	if err != nil {
		log.Fatal("create consumer:", err)
	}

	fmt.Println("⏳ Buscando mensagens...")
	msgs, err := consumer.Fetch(1, 5*time.Second)
	if err != nil {
		log.Fatal("fetch:", err)
	}

	for _, msg := range msgs {
		fmt.Printf("📨 Recebido: %s\n", string(msg.Data()))

		var received types.SpawnEvent
		if err := json.Unmarshal(msg.Data(), &received); err != nil {
			log.Println("unmarshal error:", err)
		} else {
			fmt.Printf("   Workflow: %s\n", received.WorkflowID)
		}

		// Acknowledge
		if err := msg.Ack(); err != nil {
			log.Println("ack error:", err)
		}
		fmt.Println("✅ Acknowledgado")
	}

	fmt.Println("\n🎉 Teste completo!")
}
