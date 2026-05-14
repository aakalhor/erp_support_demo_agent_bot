.PHONY: tidy index api bot test run-all clean

tidy:
	go mod tidy

index:
	go run ./cmd/indexer

api:
	go run ./cmd/api

bot:
	go run ./cmd/bot

test:
	go test ./...

# Build the index first, then start API and bot in parallel.
# On Windows use two terminals instead.
run-all: index
	@echo "Starting API and bot in parallel..."
	@(go run ./cmd/api &) ; sleep 2 ; go run ./cmd/bot

clean:
	rm -f ./storage/index.json
	rm -rf ./tmp
