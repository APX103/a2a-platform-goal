.PHONY: host fake-agent llm-agent test e2e clean docker-e2e docker-e2e-down docker-e2e-local docker-e2e-local-down

host:
	go run cmd/host/main.go

fake-agent:
	go run e2e/fake_agent/main.go

llm-agent:
	go run e2e/llm_agent/main.go

test:
	go test ./... -v

e2e:
	go test ./e2e/... -v

docker-e2e:
	cd e2e && docker-compose -f docker-compose.e2e.yml up --build --abort-on-container-exit --exit-code-from e2e-test

docker-e2e-down:
	cd e2e && docker-compose -f docker-compose.e2e.yml down

docker-e2e-local:
	cd e2e && docker-compose -f docker-compose.e2e.yml up -d host fake-agent-a fake-agent-b llm-agent
	@echo "Services started. Run 'make e2e' to run E2E tests against Docker services."
	@echo "Use 'make docker-e2e-local-down' to stop services."

docker-e2e-local-down:
	cd e2e && docker-compose -f docker-compose.e2e.yml down

clean:
	rm -f a2a_platform.db
	go clean -testcache
