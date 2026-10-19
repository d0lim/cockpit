SHELL := /usr/bin/env bash
export CGO_ENABLED := 0
BIN := bin

.PHONY: verify vet test generate-check licenses build size clean run-env seal-fixture

verify: vet test generate-check licenses build size

vet:
	go vet ./...
	@unformatted="$$(gofmt -l $$(git ls-files --cached --others --exclude-standard '*.go'))"; \
	if [ -n "$$unformatted" ]; then printf 'gofmt needed:\n%s\n' "$$unformatted" >&2; exit 1; fi

test:
	CGO_ENABLED=1 go test -race -count=1 ./...
	cd web && bun install --frozen-lockfile --silent && bun run typecheck && bun test

generate-check:
	go generate ./...
	@dirty="$$(git status --porcelain -- schema sdk web/shared internal/hub/ui/dist 2>/dev/null)"; \
	if [ -n "$$dirty" ]; then printf 'generated files drifted:\n%s\n' "$$dirty" >&2; exit 1; fi

licenses:
	scripts/check-licenses.sh .

build:
	go build -trimpath -o $(BIN)/cockpit ./cmd/cockpit
	go build -trimpath -o $(BIN)/cockpit-runner ./cmd/cockpit-runner

size: build
	@printf 'cockpit %s bytes\ncockpit-runner %s bytes\n' "$$(wc -c < $(BIN)/cockpit | tr -d ' ')" "$$(wc -c < $(BIN)/cockpit-runner | tr -d ' ')"

run-env:
	scripts/build-runners.sh
	go build -trimpath -o $(BIN)/cockpit ./cmd/cockpit
	scripts/run-env.sh $(RUN_ENV_ARGS)

seal-fixture:
	COCKPIT_INTEGRATION=1 COCKPIT_UPDATE_SEAL_FIXTURE=1 go test -count=1 -run TestIntegrationTheCommittedSealFixtureEqualsAFreshRenderOfTheToyConsumer ./internal/seal

clean:
	rm -rf $(BIN) dist
