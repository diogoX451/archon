package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/diogoX451/archon/internal/events"
	natsevents "github.com/diogoX451/archon/internal/events/nats"
	"github.com/diogoX451/archon/pkg/types"
)

type httpRequest struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    []byte            `json:"body"`
	Timeout int               `json:"timeout"`
}

type httpResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body"`
}

type needEvent struct {
	NetID string             `json:"net_id"`
	Need  types.ExternalNeed `json:"need"`
}

func main() {
	natsURL := types.Getenv("ARCHON_NATS_URL", "nats://localhost:4222")
	apiURL := types.Getenv("ARCHON_API_URL", "http://localhost:8080")
	needSubject := types.Getenv("ARCHON_NEED_SUBJECT", "archon.need.http")

	bus, err := natsevents.New(natsevents.Config{URL: natsURL})
	if err != nil {
		log.Fatalf("nats: %v", err)
	}
	defer bus.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Shutting down...")
		cancel()
	}()

	log.Printf("HTTP executor started (subject=%s)", needSubject)

	_, err = bus.Subscribe(needSubject, func(ctx context.Context, msg events.Message) error {
		var event needEvent
		if err := json.Unmarshal(msg.Data(), &event); err != nil {
			return err
		}

		var req httpRequest
		if err := json.Unmarshal(event.Need.Payload, &req); err != nil {
			return err
		}

		resp, err := doRequest(ctx, req)
		if err != nil {
			return err
		}

		payload, err := json.Marshal(resp)
		if err != nil {
			return err
		}

		if err := postWebhook(ctx, apiURL, event.Need.CorrelationID, payload); err != nil {
			return err
		}

		return msg.Ack()
	})
	if err != nil {
		log.Fatal(err)
	}

	<-ctx.Done()
}

func doRequest(ctx context.Context, req httpRequest) (*httpResponse, error) {
	timeout := time.Duration(req.Timeout) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	client := &http.Client{Timeout: timeout}
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}

	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, err
	}

	headers := make(map[string]string)
	for k, v := range httpResp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	return &httpResponse{
		StatusCode: httpResp.StatusCode,
		Headers:    headers,
		Body:       body,
	}, nil
}

func postWebhook(ctx context.Context, apiURL, correlationID string, payload []byte) error {
	url := fmt.Sprintf("%s/api/v1/webhooks/needs/%s", apiURL, correlationID)
	body := map[string]json.RawMessage{"payload": payload}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook failed: %s", resp.Status)
	}
	return nil
}
