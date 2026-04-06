BINARY  := scion-chat-app
GO      := go
GOFLAGS ?=

.PHONY: build install clean

## build: Compile the chat app binary.
build:
	$(GO) build $(GOFLAGS) -o $(BINARY) ./cmd/scion-chat-app/

## install: Build and install the chat app on a provisioned Hub VM.
##   Installs the binary, systemd unit, config file, and patches the
##   Caddyfile and Hub settings.yaml. Safe to re-run after hub updates.
##
##   Prerequisites:
##     1. Hub provisioned via scripts/starter-hub/
##     2. /home/scion/.scion/chat-app.env populated (see chat-app.env.sample)
install: build
	./install.sh

## clean: Remove the built binary.
clean:
	rm -f $(BINARY)
