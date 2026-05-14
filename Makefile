.PHONY: host fake-agent test e2e clean

host:
	go run cmd/host/main.go

fake-agent:
	go run e2e/fake_agent/main.go

test:
	go test ./... -v

e2e:
	go test ./e2e/... -v

clean:
	rm -f a2a_platform.db
	go clean -testcache
