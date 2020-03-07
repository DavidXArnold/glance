ARCH?=amd64
GO_VERSION?=1.13
GIT_VERSION := $(shell git describe --tags --always --abbrev=8 --dirty)
PACKAGE := gitlab.com/davidxarnold/glance
APPLICATION := kubectl-glance
NOW := $(shell date +'%s')
VERSION := $(shell echo $(GIT_VERSION) | sed s/-dirty/-dirty-$(NOW)/)

setup:
	cd /tmp && go get golang.org/x/tools/cmd/goimports

all: build lint

build:
	mkdir -p target
	GOARCH=$(ARCH) CGO_ENABLED=0 go build -o ./target/$(APPLICATION) ./cmd

test:
	go test ./...

lint:
	# golangci-lint run
	docker run --rm -t -v $(PWD):/go/src/$(PACKAGE) -w /go/src/$(PACKAGE) golangci/golangci-lint:v1.23 golangci-lint run -v --timeout=5m

fmt:
	goimports -w `find . -type f -name '*.go'`

check: fmt lint test

container:
	docker build --build-arg GO_VERSION=$(GO_VERSION) \
	--build-arg PACKAGE=$(PACKAGE) \
	--build-arg APPLICATION=$(APPLICATION) \
	-f deploy/docker/Dockerfile -t $(APPLICATION):${VERSION} .

.PHONY: test version
