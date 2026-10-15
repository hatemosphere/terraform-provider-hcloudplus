HOSTNAME   ?= registry.terraform.io
NAMESPACE  ?= hatemosphere
NAME       ?= hcloudplus
BINARY     := terraform-provider-$(NAME)
VERSION    ?= 0.1.0
OS_ARCH    ?= $(shell go env GOOS)_$(shell go env GOARCH)

INSTALL_DIR := $(HOME)/.terraform.d/plugins/$(HOSTNAME)/$(NAMESPACE)/$(NAME)/$(VERSION)/$(OS_ARCH)

.PHONY: default build install lint lint.fix test fmt docs clean

default: build

build:
	go build -o $(BINARY) .

install: build
	@mkdir -p $(INSTALL_DIR)
	@install -m 0755 $(BINARY) $(INSTALL_DIR)/$(BINARY)
	@echo "installed to $(INSTALL_DIR)/$(BINARY)"

lint:
	golangci-lint run ./...

lint.fix:
	golangci-lint run --fix ./...

test:
	go test -race ./...

fmt:
	gofumpt -w .
	goimports -w -local github.com/hatemosphere/terraform-provider-hcloudplus .

# Regenerate Markdown docs under docs/ from the provider schema + files in
# examples/. Run after any schema / example change. Requires
# github.com/hashicorp/terraform-plugin-docs (pulled via tools.go).
docs:
	go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate \
		--provider-name $(NAME) \
		--provider-dir . \
		--rendered-provider-name "hcloudplus"

clean:
	rm -f $(BINARY)
	rm -rf dist/
