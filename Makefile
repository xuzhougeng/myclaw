APP_NAME := baize
CMD_PATH := ./cmd/baize
DESKTOP_CMD_PATH := ./cmd/baize-desktop
DIST_DIR := dist
PACKAGE_DIR := $(DIST_DIR)/packages
STAGE_DIR := $(DIST_DIR)/stage
GO ?= go
CGO_ENABLED ?= 0
HTTP_DEV_ADDR ?= 127.0.0.1:3415

.PHONY: help frontend-bundle dev test clean install-hooks build build-current build-linux build-linux-amd64 build-linux-arm64 build-windows build-windows-amd64 build-windows-arm64 build-macos build-macos-amd64 build-macos-arm64 package-linux package-macos package-windows release

help:
	@printf "Targets:\n"
	@printf "  make install-hooks\n"
	@printf "  make dev\n"
	@printf "  make test\n"
	@printf "  make build-current\n"
	@printf "  make build-linux\n"
	@printf "  make build-windows\n"
	@printf "  make build-macos\n"
	@printf "  make package-linux\n"
	@printf "  make package-windows\n"
	@printf "  make package-macos\n"
	@printf "  make release\n"
	@printf "  make clean\n"

install-hooks:
	sh ./scripts/install-hooks.sh

frontend-bundle:
	$(GO) run ./scripts/build_frontend_bundle.go

dev: frontend-bundle
	$(GO) run $(DESKTOP_CMD_PATH) -http-dev -http-listen $(HTTP_DEV_ADDR)

test:
	$(GO) test ./...

clean:
	rm -rf $(DIST_DIR)

build: build-current

build-current:
	mkdir -p $(DIST_DIR)
	$(GO) build -trimpath -o $(DIST_DIR)/$(APP_NAME) $(CMD_PATH)

build-linux: build-linux-amd64 build-linux-arm64

build-linux-amd64:
	mkdir -p $(DIST_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=linux GOARCH=amd64 $(GO) build -trimpath -o $(DIST_DIR)/$(APP_NAME)-linux-amd64 $(CMD_PATH)

build-linux-arm64:
	mkdir -p $(DIST_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=linux GOARCH=arm64 $(GO) build -trimpath -o $(DIST_DIR)/$(APP_NAME)-linux-arm64 $(CMD_PATH)

build-windows: build-windows-amd64 build-windows-arm64

build-windows-amd64:
	mkdir -p $(DIST_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=windows GOARCH=amd64 $(GO) build -trimpath -o $(DIST_DIR)/$(APP_NAME)-windows-amd64.exe $(CMD_PATH)

build-windows-arm64:
	mkdir -p $(DIST_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=windows GOARCH=arm64 $(GO) build -trimpath -o $(DIST_DIR)/$(APP_NAME)-windows-arm64.exe $(CMD_PATH)

build-macos: build-macos-amd64 build-macos-arm64

build-macos-amd64:
	mkdir -p $(DIST_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=darwin GOARCH=amd64 $(GO) build -trimpath -o $(DIST_DIR)/$(APP_NAME)-darwin-amd64 $(CMD_PATH)

build-macos-arm64:
	mkdir -p $(DIST_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=darwin GOARCH=arm64 $(GO) build -trimpath -o $(DIST_DIR)/$(APP_NAME)-darwin-arm64 $(CMD_PATH)

package-linux: build-linux
	rm -rf $(STAGE_DIR)
	mkdir -p $(STAGE_DIR)/$(APP_NAME)-linux-amd64 $(STAGE_DIR)/$(APP_NAME)-linux-arm64 $(PACKAGE_DIR)
	cp $(DIST_DIR)/$(APP_NAME)-linux-amd64 $(STAGE_DIR)/$(APP_NAME)-linux-amd64/$(APP_NAME)
	cp README.md $(STAGE_DIR)/$(APP_NAME)-linux-amd64/
	cp $(DIST_DIR)/$(APP_NAME)-linux-arm64 $(STAGE_DIR)/$(APP_NAME)-linux-arm64/$(APP_NAME)
	cp README.md $(STAGE_DIR)/$(APP_NAME)-linux-arm64/
	cd $(STAGE_DIR)/$(APP_NAME)-linux-amd64 && zip -rq ../../packages/$(APP_NAME)-linux-amd64.zip .
	cd $(STAGE_DIR)/$(APP_NAME)-linux-arm64 && zip -rq ../../packages/$(APP_NAME)-linux-arm64.zip .

package-macos: build-macos
	rm -rf $(STAGE_DIR)
	mkdir -p $(STAGE_DIR)/$(APP_NAME)-darwin-amd64 $(STAGE_DIR)/$(APP_NAME)-darwin-arm64 $(PACKAGE_DIR)
	cp $(DIST_DIR)/$(APP_NAME)-darwin-amd64 $(STAGE_DIR)/$(APP_NAME)-darwin-amd64/$(APP_NAME)
	cp README.md $(STAGE_DIR)/$(APP_NAME)-darwin-amd64/
	cp $(DIST_DIR)/$(APP_NAME)-darwin-arm64 $(STAGE_DIR)/$(APP_NAME)-darwin-arm64/$(APP_NAME)
	cp README.md $(STAGE_DIR)/$(APP_NAME)-darwin-arm64/
	cd $(STAGE_DIR)/$(APP_NAME)-darwin-amd64 && zip -rq ../../packages/$(APP_NAME)-darwin-amd64.zip .
	cd $(STAGE_DIR)/$(APP_NAME)-darwin-arm64 && zip -rq ../../packages/$(APP_NAME)-darwin-arm64.zip .

package-windows: build-windows
	rm -rf $(STAGE_DIR)
	mkdir -p $(STAGE_DIR)/$(APP_NAME)-windows-amd64 $(STAGE_DIR)/$(APP_NAME)-windows-arm64 $(PACKAGE_DIR)
	cp $(DIST_DIR)/$(APP_NAME)-windows-amd64.exe $(STAGE_DIR)/$(APP_NAME)-windows-amd64/$(APP_NAME).exe
	cp packaging/windows/*.ps1 $(STAGE_DIR)/$(APP_NAME)-windows-amd64/
	cp packaging/windows/README.txt $(STAGE_DIR)/$(APP_NAME)-windows-amd64/
	cp $(DIST_DIR)/$(APP_NAME)-windows-arm64.exe $(STAGE_DIR)/$(APP_NAME)-windows-arm64/$(APP_NAME).exe
	cp packaging/windows/*.ps1 $(STAGE_DIR)/$(APP_NAME)-windows-arm64/
	cp packaging/windows/README.txt $(STAGE_DIR)/$(APP_NAME)-windows-arm64/
	cd $(STAGE_DIR)/$(APP_NAME)-windows-amd64 && zip -rq ../../packages/$(APP_NAME)-windows-amd64.zip .
	cd $(STAGE_DIR)/$(APP_NAME)-windows-arm64 && zip -rq ../../packages/$(APP_NAME)-windows-arm64.zip .

release: test package-linux package-windows package-macos
