BINARY  := minieth
CMD     := ./cmd/minieth
BIN_DIR := ./bin

.PHONY: build run clean test lint

build:
	go build -o $(BIN_DIR)/$(BINARY) $(CMD)

run: build
	$(BIN_DIR)/$(BINARY) node --genesis genesis.json --addr :8545

clean:
	rm -rf $(BIN_DIR)

test:
	go test ./...

lint:
	go vet ./...
