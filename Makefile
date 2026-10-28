.DEFAULT_GOAL := build

BINARY_NAME = k3sctl
BUILD_PATH = build
# сведения о сборке (internal/constant): версия, коммит, время сборки
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null)
BUILT_AT ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
MODULE = $(shell go list -m)
LDFLAGS = -X $(MODULE)/internal/constant.Version=$(VERSION) \
	-X $(MODULE)/internal/constant.Commit=$(COMMIT) \
	-X $(MODULE)/internal/constant.BuiltAt=$(BUILT_AT)

.SILENT:
.PHONY: build install clean check e2e

build:
	mkdir -p $(BUILD_PATH)
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(BUILD_PATH)/$(BINARY_NAME) ./cmd/k3sctl

# установить бинарник в $(go env GOPATH)/bin
install:
	CGO_ENABLED=0 go install -ldflags "$(LDFLAGS)" ./cmd/k3sctl

clean:
	rm -rf $(BUILD_PATH)

# проверки кода: gofmt, go vet, unit-тесты
check:
	test -z "$$(gofmt -l .)" || { echo "gofmt: не отформатировано:"; gofmt -l .; exit 1; }
	go vet ./...
	go test ./...

# сквозной тест на docker-стенде (test/run_tests.sh)
e2e:
	./test/run_tests.sh
