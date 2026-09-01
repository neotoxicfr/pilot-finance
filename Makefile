.PHONY: test test-race bench lint build coverage coverage-html docker dev vuln css css-watch css-lock

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

# --- Assets (même chaîne d'outils que le stage `css` de go/Dockerfile) ---

# Génère go/package-lock.json. À lancer une fois, puis commiter le résultat.
css-lock:
	cd go && npm install --package-lock-only --ignore-scripts --no-audit --no-fund

css:
	cd go && npm ci --ignore-scripts --no-audit --no-fund && npm run build:css

css-watch:
	cd go && npm run watch:css
