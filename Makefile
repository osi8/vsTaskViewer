.PHONY: build clean run test

build:
	@echo "Building vsTaskViewer..."
	@GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o vsTaskViewer main.go config.go auth.go task.go api.go viewer.go websocket.go html.go security.go errors.go ratelimit.go timeout.go
	@echo "Build complete: vsTaskViewer"

clean:
	@echo "Cleaning..."
	@rm -f vsTaskViewer
	@go clean
	@echo "Clean complete"

run:
	@go run *.go

test:
	@go test -v ./...

deps:
	@go mod download
	@go mod tidy

