package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/diogoX451/archon/internal/core/domain"
	"github.com/google/uuid"
)

// HTTPAgent faz requisições HTTP (pausa e espera)
type HTTPAgent struct {
	domain.BaseAgent
	Method  string
	URL     string
	Headers map[string]string
	Timeout int // segundos
	NeedType string
}

// NewHTTPAgent cria novo agente HTTP
func NewHTTPAgent(id string, config domain.Data) *HTTPAgent {
	base := domain.NewBaseAgent(id, "http", config)
	base.SetPrincipalPort(domain.PortID{AgentID: id, PortName: "trigger"})
	base.SetAuxiliaryPorts([]domain.PortID{
		{AgentID: id, PortName: "response"},
	})

	var cfg struct {
		Method  string            `json:"method"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
		Timeout int               `json:"timeout"`
		NeedType string           `json:"need_type"`
	}
	json.Unmarshal(config, &cfg)

	if cfg.Timeout == 0 {
		cfg.Timeout = 30
	}
	if cfg.NeedType == "" {
		cfg.NeedType = "http"
	}

	return &HTTPAgent{
		BaseAgent: base,
		Method:    cfg.Method,
		URL:       cfg.URL,
		Headers:   cfg.Headers,
		Timeout:   cfg.Timeout,
		NeedType:  cfg.NeedType,
	}
}

func NewHTTPAgentFromSnapshot(snap domain.AgentSnapshot) *HTTPAgent {
	var cfg struct {
		Method  string            `json:"method"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
		Timeout int               `json:"timeout"`
		NeedType string           `json:"need_type"`
	}
	json.Unmarshal(snap.Config, &cfg)

	agent := &HTTPAgent{
		BaseAgent: domain.NewBaseAgent(snap.ID, snap.Type, snap.Config),
		Method:    cfg.Method,
		URL:       cfg.URL,
		Headers:   cfg.Headers,
		Timeout:   cfg.Timeout,
		NeedType:  cfg.NeedType,
	}
	if agent.NeedType == "" {
		agent.NeedType = "http"
	}
	agent.BaseAgent.SetPrincipalPort(domain.PortID{AgentID: snap.ID, PortName: "trigger"})
	agent.BaseAgent.SetAuxiliaryPorts([]domain.PortID{
		{AgentID: snap.ID, PortName: "response"},
	})
	agent.BaseAgent.BaseDeserialize(snap)
	return agent
}

func (h *HTTPAgent) Execute(ctx context.Context) domain.ExecuteResult {
	// Parse input para variáveis de URL
	input := make(map[string]interface{})
	if h.GetInput() != nil {
		json.Unmarshal(h.GetInput(), &input)
	}

	// Substitui variáveis na URL: /api/user/{id} → /api/user/123
	url := h.substituteVars(h.URL, input)

	// Prepara payload se tiver body no input
	var body []byte
	if b, ok := input["body"]; ok {
		body, _ = json.Marshal(b)
	} else if len(input) > 0 {
		body, _ = json.Marshal(input)
	}

	// Cria need para executor externo
	need := &domain.ExternalNeed{
		Type:          h.NeedType,
		CorrelationID: uuid.New().String(),
		Payload: mustJSON(HTTPRequest{
			Method:  h.Method,
			URL:     url,
			Headers: h.Headers,
			Body:    body,
			Timeout: h.Timeout,
		}),
		CreatedAt: time.Now(),
	}

	// PAUSA! Não executa HTTP aqui
	return domain.ExecuteResult{
		State: domain.StateWaiting,
		Need:  need,
	}
}

func (h *HTTPAgent) Resume(ctx context.Context, response domain.Data) domain.ExecuteResult {
	// Recebe resposta HTTP do executor externo
	var httpResp HTTPResponse
	if err := json.Unmarshal(response, &httpResp); err != nil {
		output, _ := json.Marshal(map[string]interface{}{
			"ok":    false,
			"error": fmt.Sprintf("invalid response: %v", err),
		})
		return domain.ExecuteResult{
			State:  domain.StateCompleted,
			Output: output,
		}
	}

	// Verifica erro HTTP
	if httpResp.StatusCode >= 400 {
		output, _ := json.Marshal(map[string]interface{}{
			"ok":          false,
			"status":      httpResp.StatusCode,
			"error":       fmt.Sprintf("HTTP %d", httpResp.StatusCode),
			"body":        string(httpResp.Body),
			"headers":     httpResp.Headers,
		})
		return domain.ExecuteResult{
			State:  domain.StateCompleted,
			Output: output,
		}
	}

	// Parse body como JSON
	var bodyData interface{}
	if err := json.Unmarshal(httpResp.Body, &bodyData); err != nil {
		// Se não é JSON, retorna como string
		bodyData = string(httpResp.Body)
	}

	output, _ := json.Marshal(map[string]interface{}{
		"ok":      true,
		"status":  httpResp.StatusCode,
		"body":    bodyData,
		"headers": httpResp.Headers,
	})

	return domain.ExecuteResult{
		State:  domain.StateCompleted,
		Output: output,
	}
}

func (h *HTTPAgent) Serialize() (domain.AgentSnapshot, error) {
	snap := h.BaseAgent.BaseSerialize()
	// Need não precisa serializar, é transitório
	return snap, nil
}

func (h *HTTPAgent) Interact(ctx context.Context, other domain.Agent) domain.InteractionResult {
	exec := h.Execute(ctx)
	result := domain.InteractionResult{
		AgentAState: exec.State,
		Error:       exec.Error,
	}
	if exec.State == domain.StateCompleted {
		result.OutputA = exec.Output
		result.IsFinal = true
		result.Output = exec.Output
	}
	return result
}

func (h *HTTPAgent) substituteVars(url string, vars map[string]interface{}) string {
	// Simplificado: substitui {key} por valor
	for k, v := range vars {
		placeholder := fmt.Sprintf("{%s}", k)
		url = strings.ReplaceAll(url, placeholder, fmt.Sprintf("%v", v))
	}
	return url
}

// Tipos auxiliares
type HTTPRequest struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    []byte            `json:"body"`
	Timeout int               `json:"timeout"`
}

type HTTPResponse struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body"`
}

func mustJSON(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}
