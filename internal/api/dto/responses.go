package dto

import (
	"encoding/json"
	"time"
)

type CreateWorkflowResponse struct {
	WorkflowID string    `json:"workflow_id"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

type WorkflowStatusResponse struct {
	WorkflowID string    `json:"workflow_id"`
	Status     string    `json:"status"` // pending, running, waiting, completed, failed
	UpdatedAt  time.Time `json:"updated_at"`
}

type WorkflowResultResponse struct {
	WorkflowID string          `json:"workflow_id"`
	Status     string          `json:"status"`
	Output     json.RawMessage `json:"output,omitempty"`
	Error      string          `json:"error,omitempty"`
	FinishedAt time.Time       `json:"finished_at,omitempty"`
}

type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Details string `json:"details,omitempty"`
}
