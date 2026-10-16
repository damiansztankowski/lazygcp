BINARY_NAME=lazygcp
VERSION=0.4.0

.PHONY: all build run demo iam test clean install release-check

all: build

build:
	go build -ldflags="-s -w" -o $(BINARY_NAME) .

run: build
	./$(BINARY_NAME)

demo: build
	./$(BINARY_NAME) --demo

iam: build
	./$(BINARY_NAME) iam

test:
	go vet ./...
	go test -v ./...

install:
	go install -ldflags="-s -w" .

release-check:
	GOOS=darwin GOARCH=arm64 go build -o /dev/null .
	GOOS=darwin GOARCH=amd64 go build -o /dev/null .
	GOOS=linux GOARCH=amd64 go build -o /dev/null .
	GOOS=linux GOARCH=arm64 go build -o /dev/null .
	GOOS=windows GOARCH=amd64 go build -o /dev/null .
	@echo "✅ Cross-compilation verified for darwin/linux/windows (amd64 & arm64)"

clean:
	rm -f $(BINARY_NAME)
