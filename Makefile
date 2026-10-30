BUILD_DIR = bin
BINARY ?= blog_platform

# Binaries baked into the image; entrypoint.sh picks one at runtime.
SERVICES ?= blog_platform loadgen

TOOLS = $(shell cat tools.txt)

run: build
	./$(BUILD_DIR)/$(BINARY)

build: generate_proto
	go build -o $(BUILD_DIR)/$(BINARY) ./cmd/$(BINARY)

build-services: generate_proto
	@for svc in $(SERVICES); do \
		go build -o $(BUILD_DIR)/$$svc ./cmd/$$svc || exit 1; \
	done

install-deps:
	go mod download
	@for tool in $(TOOLS); do \
		go install $$tool; \
	done

generate_proto:
	rm -rf internal/pb
	buf generate

test: generate_proto
	go test ./...

bench:
	./run-all.sh

.PHONY: run build build-services install-deps generate_proto test bench
