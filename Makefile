.PHONY: up down test lint proto

# Subir infraestrutura
up:
	cd docker && docker-compose up -d

# Derrubar infra
down:
	cd docker && docker-compose down

# Ver logs
logs:
	cd docker && docker-compose logs -f

# Testes
test:
	go test -v ./...

# Dependências
deps:
	go mod tidy
	go mod download

# Lint
lint:
	golangci-lint run

# Formatar
fmt:
	go fmt ./...

# Limpar tudo
clean:
	cd docker && docker-compose down -v
	docker system prune -f

# Health check
health:
	@echo "NATS: "
	@curl -s http://localhost:8222/healthz | jq -r '.status' || echo "DOWN"
	@echo "Redis: "
	@redis-cli -p 6379 ping || echo "DOWN"

# Teste do NATS
test-bus:
	go run cmd/test-bus/main.go

# Setup streams (sem código)
setup-streams:
	nats stream add ARCHON_COMMANDS --subjects="archon.command.>" --retention=work --storage=memory --replicas=1 --discard=old -f || true
	nats stream add ARCHON_INTERACTIONS --subjects="archon.interaction.>" --retention=work --storage=file --replicas=1 -f || true
	nats stream add ARCHON_NEEDS --subjects="archon.need.>" --retention=interest --storage=file --replicas=1 -f || true
	nats stream add ARCHON_RESPONSES --subjects="archon.response.>" --retention=work --storage=memory --replicas=1 -f || true
	nats stream add ARCHON_RESULTS --subjects="archon.result.>" --retention=limits --storage=file --replicas=1 -f || true
	@echo "✅ Streams criados via CLI"

# Listar streams
list-streams:
	nats stream list