include config.mk

.PHONY: install-deps
install-deps: ## Installs Dependencies
	@echo "--->  Installing Dependencies"
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install github.com/boumenot/gocover-cobertura@latest
	@go install github.com/jstemmer/go-junit-report/v2@latest
	@go install github.com/jandelgado/gcov2lcov@latest
	@go install github.com/vektra/mockery/v3@latest

.PHONY: generate
generate: ## Generate mock code
	@echo "--->  Generating code"
	@go generate ./...
	@go run github.com/vektra/mockery/v2@latest

.PHONY: lint
lint: ## Linting
	@echo "--->  Linting"
	@golangci-lint run -v

.PHONY: lint-fix
lint-fix: ## Lint-Fixing code
	@echo "---> Lint-Fixing code"
	@golangci-lint run --fix

.PHONY: test
test: ## Test code
	@.script/test.sh

.PHONY: cov
cov: cov ## Show test coverage
	@go tool cover -html=.reports/coverage.out

.PHONY: test-cov
test-cov: test cov ## Test coverage

# Build target
build: ## Build code
	@echo "---> Building for $(GOOS)/$(GOARCH) with binary name $(BINARY_NAME)"
	@mkdir -p $(OUTPUT_DIR)
	@CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags="-s -w" -buildvcs=true -o $(OUTPUT_DIR)/$(BINARY_NAME) ./

# Target for Linux (amd64)
build-linux: ## Build for Linux (amd64)
	@$(MAKE) build GOOS=linux GOARCH=amd64

# Target for Linux (arm)
build-linux-arm: ## Build for Linux (arm)
	@$(MAKE) build GOOS=linux GOARCH=arm

# Target for Linux (arm64)
build-linux-arm64: ## Build for Linux (arm64)
	@$(MAKE) build GOOS=linux GOARCH=arm64

# Target for macOS (amd64)
build-macos: ## Build for macOS (amd64)
	@$(MAKE) build GOOS=darwin GOARCH=amd64

# Target for macOS (arm64)
build-macos-arm64: ## Build for macOS (arm64, Apple Silicon)
	@$(MAKE) build GOOS=darwin GOARCH=arm64

# Target for Windows (amd64)
build-windows: ## Build for Windows (amd64)
	@$(MAKE) build GOOS=windows GOARCH=amd64 BINARY_NAME=$(BINARY_NAME).exe

# Target for Windows (arm)
build-windows-arm: ## Build for Windows (arm)
	@$(MAKE) build GOOS=windows GOARCH=arm BINARY_NAME=$(BINARY_NAME).exe

# Target for Windows (arm64)
build-windows-arm64: ## Build for Windows (arm64)
	@$(MAKE) build GOOS=windows GOARCH=arm64 BINARY_NAME=$(BINARY_NAME).exe

build-all: ## Build for all platforms
	@$(MAKE) build-linux
	@$(MAKE) build-linux-arm
	@$(MAKE) build-linux-arm64
	@$(MAKE) build-macos
	@$(MAKE) build-macos-arm64
	@$(MAKE) build-windows
	@$(MAKE) build-windows-arm
	@$(MAKE) build-windows-arm64

.PHONY: clean
clean: ## Clean bin and coverage files
	@echo "--->  Cleaning bin and coverage files"
	@rm -rf bin/
	@rm -f coverage.out
	@rm -rf .reports/

.PHONY: help
help: ## Help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sed 's/Makefile://' | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'
