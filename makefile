ARCH?=amd64
GO_VERSION?=1.25
GIT_VERSION := $(shell git describe --tags --always --abbrev=8)
# Allow CI to override version via CI_COMMIT_TAG, otherwise extract from version.go
RELEASE_VERSION ?= $(shell grep 'var Version' version/version.go | cut -d'"' -f2)
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

# Build binaries for all platforms (Step 1)
PLATFORMS := darwin-amd64 darwin-arm64 linux-amd64 linux-arm64 windows-amd64

build-all:
	@echo "Building for all platforms..."
	@mkdir -p target/release
	@for platform in $(PLATFORMS); do \
		GOOS=$$(echo $$platform | cut -d'-' -f1); \
		GOARCH=$$(echo $$platform | cut -d'-' -f2); \
		OUTPUT="target/release/kubectl-glance-$(RELEASE_VERSION)-$$platform"; \
		if [ "$$GOOS" = "windows" ]; then \
			OUTPUT="$$OUTPUT.exe"; \
		fi; \
		echo "Building $$GOOS/$$GOARCH..."; \
		GOOS=$$GOOS GOARCH=$$GOARCH CGO_ENABLED=0 go build -o $$OUTPUT ./cmd; \
	done
	@echo "Build complete!"

# Create archives for all platforms (Step 1 continued)
archive-all: build-all
	@echo "Creating archives..."
	@mkdir -p target/archives
	@for platform in $(PLATFORMS); do \
		BINARY="target/release/kubectl-glance-$(RELEASE_VERSION)-$$platform"; \
		ARCHIVE="target/archives/kubectl-glance-$(RELEASE_VERSION)-$$platform.tar.gz"; \
		if [ "$$(echo $$platform | cut -d'-' -f1)" = "windows" ]; then \
			BINARY="$$BINARY.exe"; \
		fi; \
		echo "Archiving $$platform..."; \
		tar -czvf $$ARCHIVE -C target/release $$(basename $$BINARY); \
	done
	@echo "Archives created in target/archives/"

# Generate SHA256 checksums (Step 2)
checksums: archive-all
	@echo "Generating SHA256 checksums..."
	@cd target/archives && shasum -a 256 *.tar.gz > checksums.txt
	@cat target/archives/checksums.txt
	@echo ""
	@echo "Checksums saved to target/archives/checksums.txt"

# Update krew manifest with version and checksums (Step 3)
krew-plugin: checksums
	@echo "Updating krew plugin manifest..."
	@echo "RELEASE_VERSION = $(RELEASE_VERSION)"
	$(eval SHA_DARWIN_AMD64 := $(shell shasum -a 256 target/archives/kubectl-glance-$(RELEASE_VERSION)-darwin-amd64.tar.gz | cut -d' ' -f1))
	$(eval SHA_DARWIN_ARM64 := $(shell shasum -a 256 target/archives/kubectl-glance-$(RELEASE_VERSION)-darwin-arm64.tar.gz | cut -d' ' -f1))
	$(eval SHA_LINUX_AMD64 := $(shell shasum -a 256 target/archives/kubectl-glance-$(RELEASE_VERSION)-linux-amd64.tar.gz | cut -d' ' -f1))
	$(eval SHA_LINUX_ARM64 := $(shell shasum -a 256 target/archives/kubectl-glance-$(RELEASE_VERSION)-linux-arm64.tar.gz | cut -d' ' -f1))
	$(eval SHA_WINDOWS_AMD64 := $(shell shasum -a 256 target/archives/kubectl-glance-$(RELEASE_VERSION)-windows-amd64.tar.gz | cut -d' ' -f1))
	@# Update version
	$(SED) 's#version: "v[^"]*"#version: "v$(RELEASE_VERSION)"#' plugins/krew/glance.yaml
	@# Update release URLs
	$(SED) 's#/releases/v[^/]*/downloads/#/releases/v$(RELEASE_VERSION)/downloads/#g' plugins/krew/glance.yaml
	@# Update archive filenames in URLs
	$(SED) 's#kubectl-glance--#kubectl-glance-$(RELEASE_VERSION)-#g' plugins/krew/glance.yaml
	$(SED) 's#kubectl-glance-[0-9]*\.[0-9]*\.[0-9]*-#kubectl-glance-$(RELEASE_VERSION)-#g' plugins/krew/glance.yaml
	@# Update checksums (match sha256 followed by any hex string)
	$(SED) '/darwin-amd64/,/darwin-arm64/{s#sha256: "[^"]*"#sha256: "$(SHA_DARWIN_AMD64)"#;}' plugins/krew/glance.yaml
	$(SED) '/darwin-arm64/,/linux-amd64/{s#sha256: "[^"]*"#sha256: "$(SHA_DARWIN_ARM64)"#;}' plugins/krew/glance.yaml
	$(SED) '/linux-amd64/,/linux-arm64/{s#sha256: "[^"]*"#sha256: "$(SHA_LINUX_AMD64)"#;}' plugins/krew/glance.yaml
	$(SED) '/linux-arm64/,/windows-amd64/{s#sha256: "[^"]*"#sha256: "$(SHA_LINUX_ARM64)"#;}' plugins/krew/glance.yaml
	$(SED) '/windows-amd64/,$$/{s#sha256: "[^"]*"#sha256: "$(SHA_WINDOWS_AMD64)"#;}' plugins/krew/glance.yaml
	@rm -f plugins/krew/glance.yaml.bkp
	@echo "Krew manifest updated!"
	@echo ""
	@echo "SHA256 checksums:"
	@echo "  darwin-amd64:  $(SHA_DARWIN_AMD64)"
	@echo "  darwin-arm64:  $(SHA_DARWIN_ARM64)"
	@echo "  linux-amd64:   $(SHA_LINUX_AMD64)"
	@echo "  linux-arm64:   $(SHA_LINUX_ARM64)"
	@echo "  windows-amd64: $(SHA_WINDOWS_AMD64)"

# Full release process: build, archive, checksum, update manifest
release: krew-plugin
	@echo ""
	@echo "========================================="
	@echo "Release v$(RELEASE_VERSION) prepared!"
	@echo "========================================="
	@echo ""
	@echo "Next steps:"
	@echo "  1. Review plugins/krew/glance.yaml"
	@echo "  2. Commit changes: git add -A && git commit -m 'Release v$(RELEASE_VERSION)'"
	@echo "  3. Tag release: make tag-release"
	@echo "  4. Push: git push && git push --tags"
	@echo "  5. Upload archives from target/archives/ to GitLab release"
	@echo "  6. Submit PR to kubernetes-sigs/krew-index"
	@echo ""

release_version:
	@echo $(RELEASE_VERSION)

# Reset krew manifest to use placeholders (for clean rebuilds)
krew-reset:
	@echo "Resetting krew manifest placeholders..."
	git checkout plugins/krew/glance.yaml

# Validate krew manifest locally
krew-validate: krew-plugin
	@echo "Validating krew manifest..."
	kubectl krew install --manifest=plugins/krew/glance.yaml
	kubectl glance --help
	kubectl krew uninstall glance
	@echo "Validation passed!"

tag-release:
	git tag v$(RELEASE_VERSION)

clean:
	rm -rf target/
	
.PHONY: test version build archive formula build-all archive-all checksums krew-plugin release krew-reset krew-validate clean
