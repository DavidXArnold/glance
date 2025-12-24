ARCH?=amd64
GO_VERSION?=1.25
GIT_VERSION := $(shell git describe --tags --always --abbrev=8)
RELEASE_VERSION := $(shell sed -nE 's/^var[[:space:]]Version[[:space:]]=[[:space:]]"([^"]+)".*/\1/p' version/version.go)
PACKAGE := gitlab.com/davidxarnold/glance
APPLICATION?=kubectl-glance
NOW := $(shell date +'%s')
VERSION := $(shell echo $(GIT_VERSION) | sed s/-dirty/-dirty-$(NOW)/)
REF?=master
URL?=\"https://${PACKAGE}/-/jobs/${REF}/artifacts/raw/archive/${APPLICATION}-${RELEASE_VERSION}.tar.gz?job=build-darwin\"
SED 				:=
UNAME_S := $(shell uname -s)
	ifeq ($(UNAME_S),Linux)
		SED += sed -i
	endif
	ifeq ($(UNAME_S),Darwin)
		SED += sed -i .bkp
	endif

# Use docker if available, otherwise fall back to nerdctl
DOCKER_CMD := $(shell command -v docker 2>/dev/null || command -v nerdctl 2>/dev/null)

all: check build 

archive:
	mkdir -p ./archive/ && tar -zcvf ./archive/$(APPLICATION)-$(RELEASE_VERSION).tar.gz ./target/$(APPLICATION)

build:
	mkdir -p target
	GOARCH=$(ARCH) CGO_ENABLED=0 go build -o ./target/$(APPLICATION) ./cmd

test:
	go test ./...

lint:
	# golangci-lint run
	$(DOCKER_CMD) run --rm -t -v $(PWD):/go/src/$(PACKAGE) -w /go/src/$(PACKAGE) golangci/golangci-lint:v2.7.2 golangci-lint run -v --timeout=5m

fmt:
	go run golang.org/x/tools/cmd/goimports@latest -w `find . -type f -name '*.go'`

check: fmt lint test

container:
	$(DOCKER_CMD) build --build-arg GO_VERSION=$(GO_VERSION) \
	--build-arg PACKAGE=$(PACKAGE) \
	--build-arg APPLICATION=$(APPLICATION) \
	-f deploy/docker/Dockerfile -t $(APPLICATION):${VERSION} .

download-deps:
	@echo Download go.mod dependencies
	@go mod download

formula:
	${SED} "s#\(sha256 \)\(.*\)#\1\"${ARCHIVE_SHA}\"#" Formula/glance.rb
	${SED} "s#\(version \)\(.*\)#\1\"${RELEASE_VERSION}\"#" Formula/glance.rb
	${SED} "s#\(url \)\(.*\)#\1${URL}#" Formula/glance.rb

install-tools: download-deps
	@echo Installing tools from tools/tools.go
	@cat ./tools/tools.go | grep _ | awk -F'"' '{print $$2}' | xargs -tI % go install %

krew-plugin:
	${SED} "s#\(version: \)\"[^\"]*\"#\1\"${RELEASE_VERSION}\"#" plugins/krew/glance.yaml
	${SED} "s#PLACEHOLDER_SHA256_DARWIN_AMD64#${ARCHIVE_SHA_DARWIN_AMD64}#" plugins/krew/glance.yaml
	${SED} "s#PLACEHOLDER_SHA256_DARWIN_ARM64#${ARCHIVE_SHA_DARWIN_ARM64}#" plugins/krew/glance.yaml
	${SED} "s#PLACEHOLDER_SHA256_LINUX_AMD64#${ARCHIVE_SHA_LINUX_AMD64}#" plugins/krew/glance.yaml
	${SED} "s#PLACEHOLDER_SHA256_LINUX_ARM64#${ARCHIVE_SHA_LINUX_ARM64}#" plugins/krew/glance.yaml
	${SED} "s#PLACEHOLDER_SHA256_WINDOWS_AMD64#${ARCHIVE_SHA_WINDOWS_AMD64}#" plugins/krew/glance.yaml

release_version:
	@echo $(RELEASE_VERSION)

tag-release:
	git tag v$(RELEASE_VERSION)
	
.PHONY: test version build archive formula
