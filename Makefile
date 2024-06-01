BIN = bin

export GO111MODULE=on
export GOBIN ?= $(shell pwd)/$(BIN)

GO_FILES = $(shell find . \
	   -path '*/.*' -prune -o \
	   '(' -type f -a -name '*.go' ')' -print)

GOLINT = $(BIN)/golint
STATICCHECK = $(BIN)/staticcheck
EXTRACT_CHANGELOG = $(BIN)/extract-changelog
TOOLS = $(GOLINT) $(STATICCHECK) $(EXTRACT_CHANGELOG)

.PHONY: all
all: build lint test

.PHONY: build
build:
	go build ./...

.PHONY: test
test:
	go test -v -race ./...

.PHONY: cover
cover:
	go test -race -coverprofile=cover.out -coverpkg=./... ./...
	go tool cover -html=cover.out -o cover.html

.PHONY: lint
lint: gofmt golint staticcheck

.PHONY: gofmt
gofmt:
	$(eval FMT_LOG := $(shell mktemp -t gofmt.XXXXX))
	@gofmt -e -s -l $(GO_FILES) > $(FMT_LOG) || true
	@[ ! -s "$(FMT_LOG)" ] || \
		(echo "gofmt failed. Please reformat the following files:" | \
		cat - $(FMT_LOG) && false)

.PHONY: golint
golint: $(GOLINT)
	$(GOLINT) ./...

.PHONY: staticcheck
staticcheck: $(STATICCHECK)
	$(STATICCHECK) ./...

.PHONY: tools
tools: $(GOLINT) $(STATICCHECK)

$(GOLINT): tools/go.mod
	cd tools && go install golang.org/x/lint/golint

$(STATICCHECK): tools/go.mod
	cd tools && go install honnef.co/go/tools/cmd/staticcheck

$(EXTRACT_CHANGELOG): tools/cmd/extract-changelog/main.go
	cd tools && go install github.com/BIwashi/gopatch/tools/cmd/extract-changelog
