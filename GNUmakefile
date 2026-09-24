SPOTLIGHT_REPO ?= ../spotlight
TERRAFORM_EXAMPLES := examples
SWEEP ?= $(SPOTLIGHT_ACC_WORKSPACE)

.PHONY: default build install test testacc sweep lint vet fmt fmtcheck generate client docs openapi check

default: check

build:
	go build ./...

install:
	go install .

test:
	go test ./... -timeout 15m

testacc:
	TF_ACC=1 go test ./... -v -count=1 -timeout 60m

sweep:
	@test -n "$(SWEEP)" || (echo "Set SWEEP, or SPOTLIGHT_ACC_WORKSPACE, to the acceptance workspace's slug." && exit 1)
	go test ./internal/provider -v -count=1 -sweep=$(SWEEP) -timeout 60m

lint:
	golangci-lint run ./...

vet:
	go vet ./...

fmt:
	gofmt -w .
	terraform fmt -recursive $(TERRAFORM_EXAMPLES)

fmtcheck:
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
	terraform fmt -check -recursive $(TERRAFORM_EXAMPLES)

openapi:
	cp $(SPOTLIGHT_REPO)/control-plane/openapi.json api/openapi.json

client:
	go tool oapi-codegen -config api/oapi-codegen.yaml api/openapi.json

docs:
	go tool tfplugindocs generate --provider-name spotlight

generate: client docs

check: fmtcheck lint vet build test
