BIN_DIR := ./bin
BIN_DST := /usr/bin
FRP_DIR := ./frp
FRP_VERSION := 0.69.1
BUN ?= bun

ifdef GOOS
	ifeq (${GOOS}, windows)
		WIN_TARGET := True
	endif
else
	ifeq (${OS}, Windows_NT)
		WIN_TARGET := True
	endif
endif

ifdef WIN_TARGET
	SSH05 := ssh05.exe
else
	SSH05 := ssh05
endif

ifeq (${OS}, Windows_NT)
	RM := PowerShell -Command Remove-Item -Force
	GO_TEST := go test
else
	RM := rm -f
	GO_TEST := ${shell basename `which gotest 2>/dev/null` 2>/dev/null || echo go test}
endif

.PHONY: all clean test install download-frp phase0 bench contracts-check ui-check ui-smoke ssh05d trssd frp-smoke ssh05d-frp-smoke trssd-frp-smoke hostkey-smoke trzsz-smoke phase1-smoke p1-smoke verify deb

# Check if frp source is available (may not exist on clone since /frp/ is gitignored)
ifneq ("$(wildcard $(FRP_DIR)/go.mod)","")
  HAS_FRP := yes
endif

all: ${BIN_DIR}/${SSH05}

ifeq ($(HAS_FRP),yes)
all: ${BIN_DIR}/frps ${BIN_DIR}/frpc
endif

all: ssh05d

${BIN_DIR}/${SSH05}: $(wildcard ./cmd/ssh05/*.go ./ssh05/*.go) go.mod go.sum
	CGO_ENABLED=0 go build -o ${BIN_DIR}/ ./cmd/ssh05

ifeq ($(HAS_FRP),yes)
${BIN_DIR}/frps: ${FRP_DIR}/go.mod ${FRP_DIR}/go.sum
	cd ${FRP_DIR} && CGO_ENABLED=0 go build -tags noweb -o ../${BIN_DIR}/frps -ldflags="-s -w" ./cmd/frps

${BIN_DIR}/frpc: ${FRP_DIR}/go.mod ${FRP_DIR}/go.sum
	cd ${FRP_DIR} && CGO_ENABLED=0 go build -tags noweb -o ../${BIN_DIR}/frpc -ldflags="-s -w" ./cmd/frpc
endif

# Download pre-built FRP binaries (used when /frp/ source is not available)
FRP_OS := $(shell uname -s | tr A-Z a-z)
FRP_ARCH := $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/;s/armv7l/arm_hf/;s/armv6l/arm/;s/i386/386/')
download-frp:
	@mkdir -p ${BIN_DIR}
	@if [ "${FRP_OS}" = "darwin" ] && [ "${FRP_ARCH}" = "arm64" ]; then \
		echo "Downloading FRP v${FRP_VERSION} for darwin_arm64..."; \
		curl -sLo /tmp/frp.tar.gz "https://github.com/fatedier/frp/releases/download/v${FRP_VERSION}/frp_${FRP_VERSION}_darwin_arm64.tar.gz"; \
	elif [ "${FRP_OS}" = "darwin" ]; then \
		echo "Downloading FRP v${FRP_VERSION} for darwin_amd64..."; \
		curl -sLo /tmp/frp.tar.gz "https://github.com/fatedier/frp/releases/download/v${FRP_VERSION}/frp_${FRP_VERSION}_darwin_amd64.tar.gz"; \
	else \
		echo "Downloading FRP v${FRP_VERSION} for ${FRP_OS}_${FRP_ARCH}..."; \
		curl -sLo /tmp/frp.tar.gz "https://github.com/fatedier/frp/releases/download/v${FRP_VERSION}/frp_${FRP_VERSION}_${FRP_OS}_${FRP_ARCH}.tar.gz" || \
		( echo "Trying arm fallback..."; \
		  ARCH_ARM=$$(echo "${FRP_ARCH}" | sed 's/arm_hf/arm/'); \
		  curl -sLo /tmp/frp.tar.gz "https://github.com/fatedier/frp/releases/download/v${FRP_VERSION}/frp_${FRP_VERSION}_${FRP_OS}_$${ARCH_ARM}.tar.gz" ); \
	fi && \
	tar -xzf /tmp/frp.tar.gz -C /tmp && \
	cp /tmp/frp_${FRP_VERSION}_*/frps ${BIN_DIR}/ && \
	cp /tmp/frp_${FRP_VERSION}_*/frpc ${BIN_DIR}/ && \
	chmod +x ${BIN_DIR}/frps ${BIN_DIR}/frpc && \
	rm -f /tmp/frp.tar.gz && \
	echo "FRP binaries installed in ${BIN_DIR}/"

clean:
	$(RM) ${BIN_DIR}/ssh05 ${BIN_DIR}/ssh05.exe ${BIN_DIR}/ssh05d ${BIN_DIR}/ssh05d.exe ${BIN_DIR}/trssd ${BIN_DIR}/trssd.exe ${BIN_DIR}/frps ${BIN_DIR}/frpc 2>/dev/null; true

test:
	${GO_TEST} -v -count=1 ./ssh05 ./contracts ./internal/identity ./internal/ipc ./internal/supervisor ./internal/audit ./internal/frp ./internal/transfer

## ssh05d builds the local supervisor. `trssd` remains an alias for scripts
## written before the rename.
ssh05d:
	CGO_ENABLED=0 go build -o ${BIN_DIR}/ ./cmd/ssh05d

trssd: ssh05d

phase0:
	cd prototype/terminal && $(BUN) install && $(BUN) run typecheck && $(BUN) run smoke

ui-check:
	cd prototype/terminal && $(BUN) run typecheck && $(BUN) test

ui-smoke: ssh05d
	cd prototype/terminal && $(BUN) run ui:smoke

frp-smoke:
	./scripts/frp-smoke.sh

ssh05d-frp-smoke:
	./scripts/ssh05d-frp-smoke.sh

trssd-frp-smoke: ssh05d-frp-smoke

p1-smoke: hostkey-smoke ui-smoke ssh05d-frp-smoke

trzsz-smoke:
	./scripts/trzsz-smoke.sh

verify: test contracts-check p1-smoke trzsz-smoke

deb:
	./scripts/build-deb.sh ubuntu amd64
	./scripts/build-deb.sh ubuntu arm64
	./scripts/build-deb.sh debian amd64
	./scripts/build-deb.sh debian arm64

bench:
	CGO_ENABLED=0 ${GO_TEST} -run '^$$' -bench . -benchmem ./internal/ipc ./internal/supervisor ./internal/frp

contracts-check:
	CGO_ENABLED=0 ${GO_TEST} -count=1 ./contracts ./internal/identity ./internal/ipc ./internal/supervisor ./internal/audit ./internal/frp ./internal/transfer
	cd prototype/terminal && $(BUN) run typecheck:contracts

install: all
ifdef WIN_TARGET
	@echo install target is not supported for Windows
else
	@mkdir -p ${DESTDIR}${BIN_DST}
	cp ${BIN_DIR}/ssh05 ${DESTDIR}${BIN_DST}/
	cp ${BIN_DIR}/ssh05d ${DESTDIR}${BIN_DST}/
endif

hostkey-smoke:
	./scripts/hostkey-smoke.sh

phase1-smoke: hostkey-smoke
	$(MAKE) ui-smoke
