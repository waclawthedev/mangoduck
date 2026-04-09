GO_IMAGE := golang:1.26
GOLANGCI_LINT_IMAGE := golangci/golangci-lint:v2.11.4
WORKDIR := /app

DOCKER_RUN := docker run --rm \
	-v "$(CURDIR)":"$(WORKDIR)" \
	-w "$(WORKDIR)" \
	-v "$(HOME)/go/pkg/mod":/go/pkg/mod \
	-e GOMODCACHE=/go/pkg/mod

.PHONY: go lint tidy fmt vet test build check generate-mocks

# Run arbitrary go commands: make go CMD="build ./..."
go:
	$(DOCKER_RUN) $(GO_IMAGE) go $(CMD)

tidy:
	$(DOCKER_RUN) $(GO_IMAGE) go mod tidy

fmt:
	$(DOCKER_RUN) $(GO_IMAGE) sh -c "go install mvdan.cc/gofumpt@v0.9.2 && gofumpt -l -w ."

vet:
	$(DOCKER_RUN) $(GO_IMAGE) go vet ./...

test:
	$(DOCKER_RUN) $(GO_IMAGE) go test ./...

build:
	$(DOCKER_RUN) $(GO_IMAGE) go build ./...

generate-mocks:
	$(DOCKER_RUN) $(GO_IMAGE) sh -c "go install github.com/vektra/mockery/v3@v3.7.0 && go generate ./..."

lint:
	docker run --rm -v "$(CURDIR)":"$(WORKDIR)" -w "$(WORKDIR)" $(GOLANGCI_LINT_IMAGE) golangci-lint run