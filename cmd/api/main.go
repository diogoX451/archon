package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/diogoX451/archon/internal/adapters/events"
	"github.com/diogoX451/archon/internal/api"
	"github.com/diogoX451/archon/internal/config"
	natsevents "github.com/diogoX451/archon/internal/events/nats"
	redisstore "github.com/diogoX451/archon/internal/store/redis"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Println("Connecting to NATS...")
	natsBus, err := natsevents.New(natsevents.Config{
		URL:           cfg.NATS.URL,
		MaxReconnects: cfg.NATS.MaxReconnects,
		ReconnectWait: 2 * time.Second,
	})
	if err != nil {
		log.Fatalf("Failed to connect to NATS: %v", err)
	}
	defer natsBus.Close()

	log.Println("Setting up NATS streams...")
	if err := natsBus.SetupArchonStreams(); err != nil {
		log.Fatalf("Failed to setup streams: %v", err)
	}

	eventBus := events.NewEventBus(natsBus)

	log.Println("Creating HTTP server...")
	redisClient, err := redisstore.New(redisstore.Config{
		Addr:       cfg.Redis.Addr,
		Password:   cfg.Redis.Password,
		DB:         cfg.Redis.DB,
		PoolSize:   cfg.Redis.PoolSize,
		DefaultTTL: 24 * time.Hour,
	})
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer redisClient.Close()

	server := api.NewServer(eventBus, redisClient)

	srv := &http.Server{
		Addr:         ":" + cfg.App.Port,
		Handler:      server,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	done := make(chan bool, 1)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		log.Println("Server is shutting down...")

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		srv.SetKeepAlivesEnabled(false)
		if err := srv.Shutdown(ctx); err != nil {
			log.Fatalf("Could not gracefully shutdown the server: %v\n", err)
		}
		close(done)
	}()

	log.Printf("Server is ready to handle requests at %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Could not listen on %s: %v\n", srv.Addr, err)
	}

	<-done
	log.Println("Server stopped")
}
