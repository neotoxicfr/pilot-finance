.PHONY: test bench lint build coverage docker dev vuln

test:
	cd go && go test -timeout 120s ./...

test-race:
	cd go && go test -race -timeout 600s ./...

bench:
	cd go && go test -bench=. -benchmem ./internal/projection/

lint:
	cd go && golangci-lint run

coverage:
	cd go && go test -timeout 120s -coverprofile=coverage.out $$(go list ./... | grep -v -E '/cmd/|/db$$')
	cd go && go tool cover -func=coverage.out | tail -1

coverage-html:
	cd go && go test -timeout 120s -coverprofile=coverage.out $$(go list ./... | grep -v -E '/cmd/|/db$$')
	cd go && go tool cover -html=coverage.out

build:
	cd go && go build ./...

docker:
	docker compose build

dev:
	cd go && go run ./cmd/server

vuln:
	cd go && govulncheck ./...
