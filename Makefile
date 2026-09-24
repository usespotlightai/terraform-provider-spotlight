SPOTLIGHT_REPO ?= ../spotlight
TERRAFORM_EXAMPLES := examples

.PHONY: default build install test testacc vet fmt fmtcheck generate client docs openapi check

default: check

build:
	go build ./...

install:
	go install .

test:
	go test ./... -timeout 10m

testacc:
	TF_ACC=1 go test ./internal/acctest/... -v -count=1 -timeout 30m

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

check: fmtcheck vet build test
