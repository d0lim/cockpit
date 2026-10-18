SHELL := /usr/bin/env bash
export CGO_ENABLED := 0
BIN := bin

.PHONY: verify vet test generate-check licenses build size clean

verify: vet test generate-check licenses build size

vet:
	go vet ./...

test:
	CGO_ENABLED=1 go test -race -count=1 ./...

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

clean:
	rm -rf $(BIN) dist
