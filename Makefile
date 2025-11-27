.PHONY: build clean run test

build:
	@echo "Building vsTaskViewer..."
	@GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o vsTaskViewer main.go config.go auth.go task.go api.go viewer.go websocket.go websocket_manager.go html.go security.go errors.go ratelimit.go timeout.go
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

deb:
	@echo "Building .deb package..."
	@if ! command -v dpkg-buildpackage >/dev/null 2>&1; then \
		echo "Error: dpkg-buildpackage not found. Install with: sudo apt-get install build-essential devscripts debhelper"; \
		exit 1; \
	fi
	@dpkg-buildpackage -b -us -uc
	@echo "Package built successfully. Check parent directory for .deb file."

deb-clean:
	@echo "Cleaning Debian build artifacts..."
	@rm -rf debian/vstaskviewer debian/.debhelper debian/files debian/*.substvars
	@rm -f ../vstaskviewer_*.deb ../vstaskviewer_*.changes ../vstaskviewer_*.buildinfo
	@echo "Clean complete"

