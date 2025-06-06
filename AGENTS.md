# Repository Purpose
This repository contains a Go HTTP/2 proxy that allows the Cursor IDE to access any model provided by OpenRouter.  It translates OpenAI API requests from Cursor so that they work with models offered by OpenRouter.

# Development Guidelines
- Go version **1.21** is expected. Any Go code should build with this version.
- Format Go files with `gofmt -w` before committing.
- Verify builds with `go vet ./...` and `go build ./...`. The code must compile without errors.
- If `proxy.go` changes, rebuild the binary using `go build -o proxy proxy.go` so the included `proxy` binary matches the source.
- Keep the `proxy` binary and existing configuration files in the repository.

# Testing
- No unit tests are provided. Building successfully is considered passing.
- The `test_proxy.sh` script requires a valid `.env` setup and internet access and is optional.
