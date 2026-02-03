# Archon Interaction Nets — Documentação

Esta documentação descreve a arquitetura baseada em **Interaction Nets** (Lafont 1989), o modelo de execução do Archon e como operar o sistema via API e NATS.

---

## Sumário

- [Arquitetura](#arquitetura)
- [Conceitos de Interaction Nets](#conceitos-de-interaction-nets)
- [Agentes built-in e portas](#agentes-built-in-e-portas)
- [Subjects NATS](#subjects-nats)
- [API HTTP](#api-http)
- [Plan (LLM JSON)](#plan-llm-json)
- [Regras (RuleDefs)](#regras-ruledefs)
- [Persistência no Redis](#persistência-no-redis)
- [Executores externos](#executores-externos)
- [Como rodar](#como-rodar)
- [Troubleshooting](#troubleshooting)

---

## Arquitetura

Fluxo geral:
1. **API** publica comandos no NATS (`archon.command.*`).
2. **Worker** consome comandos, atualiza o estado no Redis e dispara interações.
3. **Engine** executa regras de Interaction Nets (reescrita topológica). 
4. **Resultados** e **needs** são publicados em `archon.result.*` e `archon.need.*`.
5. **Executores externos** (ex.: HTTP) consomem `archon.need.*` e respondem via webhook.

Componentes principais:
- **API** (`cmd/api`): entrada REST.
- **Worker** (`cmd/worker`): orquestra workflow/Interações.
- **Redis**: persistência do estado e regras.
- **NATS JetStream**: fila/event bus.

---

## Conceitos de Interaction Nets

O sistema segue os princípios do paper de Lafont (1989):

1) **Linearity**
- Cada porta interna deve ser usada no máximo uma vez.

2) **Binary interaction**
- Interações só ocorrem entre **portas principais** conectadas.

3) **No ambiguity**
- No máximo uma regra para cada par de símbolos distintos.
- Não há regra para `S,S`.

4) **RHS sem par ativo**
- A regra não pode gerar um par principal↔principal no lado direito.

Esses invariantes são verificados no momento do registro de regras.

---

## Agentes built-in e portas

- **calculator**
  - principal: `input`
  - auxiliares: `output`

- **transform**
  - principal: `input`
  - auxiliares: `output`

- **http**
  - principal: `trigger`
  - auxiliares: `response`
  - resposta: sempre retorna JSON com `ok` (true/false); em erro inclui `status`, `error`, `body`, `headers`

---

## Subjects NATS

### Comandos
- `archon.command.spawn`
- `archon.command.add_agent`
- `archon.command.connect`
- `archon.command.define_rule`

### Execução
- `archon.interaction.pending`

### I/O externo
- `archon.need.*`
- `archon.response.*`

### Resultado
- `archon.result.*`

---

## API HTTP

### Criar workflow

`POST /api/v1/workflows`

```json
{
  "user_id": "user_123",
  "agents": [
    {"id": "calc1", "type": "calculator", "config": {"operation": "sum"}}
  ],
  "connections": [],
  "input": [1,2,3]
}
```

### Adicionar agente

`POST /api/v1/workflows/{id}/agents`

```json
{
  "id": "calc2",
  "type": "calculator",
  "config": {"operation": "sum"}
}
```

### Conectar portas

`POST /api/v1/workflows/{id}/connections`

```json
{
  "from": {"agent_id": "calc1", "port": "input"},
  "to": {"agent_id": "calc2", "port": "input"}
}
```

### Definir regra

`POST /api/v1/rules`

```json
{
  "rule": {
    "agent_a_type": "calculator",
    "agent_b_type": "transform",
    "agents": [
      {"id": "x", "type": "calculator", "config": {"operation": "sum"}}
    ],
    "connections": [],
    "interface": [
      {
        "external": {"side": "A", "port": "output"},
        "internal": {"agent_id": "x", "port": "input"}
      },
      {
        "external": {"side": "B", "port": "input"},
        "internal": {"agent_id": "x", "port": "output"}
      }
    ]
  }
}
```

### Listar regras

`GET /api/v1/rules`

### Buscar regra

`GET /api/v1/rules/{a}/{b}`

---

## Plan (LLM JSON)

Use um único endpoint para aplicar **regras → spawn → add_agent → connect**.

`POST /api/v1/plan`

- Exemplo: `docs/plan.example.json`
- Schema oficial: `docs/schema.plan.json`

---

## Regras (RuleDefs)

Uma regra tem:
- `agent_a_type`, `agent_b_type`
- `agents` (sub-rede RHS)
- `connections` (entre agentes RHS)
- `interface` (mapeia portas externas A/B → RHS)

Requisitos validados:
- No ambiguity (A != B)
- Todas as portas auxiliares externas mapeadas
- Sem portas duplicadas
- Sem uso de porta principal na interface
- RHS não gera par ativo principal↔principal

---

## Persistência no Redis

Chaves principais:
- `workflow:{id}`
- `workflow:{id}:agent:{agent_id}`
- `index:state:{state}`
- `rule:{a}|{b}`
- `rules:index`

Regras são persistidas e carregadas na inicialização do worker.

---

## Executores externos

### HTTP Executor

O agent `http` gera um **need** em `archon.need.http` e fica aguardando resposta.
O executor HTTP faz:
1) consome `archon.need.http`
2) executa a chamada HTTP real
3) responde `/api/v1/webhooks/needs/{correlation_id}`

Rodar:
```
ARCHON_NATS_URL=nats://localhost:4222 \
ARCHON_API_URL=http://localhost:8080 \
go run cmd/http-executor/main.go
```

Para consumir needs customizados (ex.: `archon.need.consulta-car`), rode com:
```
ARCHON_NEED_SUBJECT=archon.need.> \
go run cmd/http-executor/main.go
```

---

## Como rodar

### Worker

```
ARCHON_NATS_URL=nats://localhost:4222 \
ARCHON_REDIS_ADDR=localhost:6379 \
go run cmd/worker/main.go
```

### API

```
ARCHON_NATS_URL=nats://localhost:4222 \
ARCHON_REDIS_ADDR=localhost:6379 \
ARCHON_APP_PORT=8080 \
go run cmd/api/main.go
```

### HTTP Executor

```
ARCHON_NATS_URL=nats://localhost:4222 \
ARCHON_API_URL=http://localhost:8080 \
go run cmd/http-executor/main.go
```

---

## Troubleshooting

- **Nenhuma interação executa**: verifique se existe regra para o par de símbolos.
- **Erro “filtered consumer not unique on workqueue stream”**: há consumers antigos com o mesmo filtro; reinicie ou remova consumers.
- **NUI mostra erro “multiple non-filtered consumers not allowed on workqueue stream”**: o stream `ARCHON_INTERACTIONS` usa workqueue. Para visualizar no NUI, rode com `ARCHON_INTERACTIONS_RETENTION=limits` (ou `interest`) e reinicie API/worker.
- **Poucos logs no worker**: inicie com `ARCHON_LOG_EVENTS=1` para imprimir eventos recebidos/produzidos.
- **HTTPAgent não responde**: verifique se o executor HTTP está rodando.

---

## Referência

Lafont, Yves — “Interaction Nets”, POPL 1990.
