# Fluxo, Need/Response e Regras (Archon)

Este documento explica a lógica do fluxo, como os `needs` são publicados/retomados e as mudanças aplicadas durante a depuração.

---

## Conceitos principais

- **Par ativo**: só existe quando **duas portas principais** estão conectadas.  
  - `transform`: principal = `input`, auxiliar = `output`
  - `http`: principal = `trigger`, auxiliar = `response`

- **Regra**: define como um par ativo (`transform|http`) é reescrito.
- **Need**: quando o `http` executa, ele não faz a chamada. Ele publica um `need` em `archon.need.<need_type>`.
- **Response**: seu serviço externo responde via webhook, o worker retoma o agente `http` e continua o fluxo se houver conexão saindo de `response`.

---

## Fluxo mínimo (transform -> http)

Para gerar `need`:

```
t_init:input  ->  http_consulta_car:trigger
```

Resultado:
- `t_init` recebe o input do workflow.
- Regra `transform|http` cria o `http` interno.
- O `http` publica `archon.need.<need_type>`.

Se não houver conexão saindo de `http:response`, o fluxo **termina** após a resposta.

---

## Como o response continua o fluxo

Quando seu serviço envia o webhook `/api/v1/webhooks/needs/{correlation_id}`:

1. API publica `archon.response.<correlation_id>`
2. Worker retoma o `http`
3. `http` produz output (`ok`, `status`, `body`, `headers`)
4. Se houver conexão saindo de `http:response`, o próximo agente é executado

Exemplo:

```
http_consulta_car:response  ->  t_laudo_e_boleto:input
```

---

## Por que não aparecia o need

Os problemas mais comuns encontrados:

- Usar **portas auxiliares** como se fossem principais
  - Ex.: `transform:output -> http:trigger` é ok
  - Ex.: `transform:input -> http:trigger` é ok (principal-principal)
  - Ex.: `http:response -> transform:input` não cria par ativo (auxiliar->principal)

- Usar a mesma porta principal em duas conexões  
  (ex.: `http:trigger` conectado como entrada e saída)

---

## Mudanças aplicadas no código

### 1) Config do agente externo preservada
Quando a regra cria agentes internos do mesmo tipo, a config do agente externo é herdada.  
Isso garante que `need_type` (ex.: `consulta-car`) seja mantido.

Arquivo:  
`internal/core/domain/net.go`

---

### 2) Need publicado após reescrita
O core agora publica `need` após a reescrita quando agentes `http` são criados.

Arquivo:  
`internal/core/service/executor.go`

---

### 3) Input propagado para o HTTP
O `http` agora recebe input mesmo quando o output do transform não foi conectado corretamente.

Arquivo:  
`internal/core/service/executor.go`

---

### 4) Body no payload do HTTP
Se o input não tem `body`, o `http` usa o input inteiro como `body`.

Arquivo:  
`internal/agents/http.go`

---

## Observação sobre Base64 no body

O `body` é `[]byte` no JSON, então aparece Base64 no consumidor.  
Se quiser `body` como JSON direto, é possível mudar o tipo para `json.RawMessage`.

---

## Checklist rápido

- Worker e API reiniciados após mudanças?
- Regra `transform|http` correta (conexão `x:output -> y:trigger`)?
- Conexões não usam portas auxiliares como principal?
- Seu serviço consome `archon.need.<need_type>`?
- Webhook responde `/api/v1/webhooks/needs/{correlation_id}`?

