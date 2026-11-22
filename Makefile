BINARY := portfolio
GOFLAGS := -trimpath

.PHONY: all build web test bench run clean

all: build

build:
	go build $(GOFLAGS) -o $(BINARY) ./cmd/server

web:
	npm --prefix frontend run build

test:
	go test ./internal/...

bench:
	go test ./internal/... -run '^$$' -bench . -benchmem

run: build
	./$(BINARY)

clean:
	rm -f $(BINARY)
