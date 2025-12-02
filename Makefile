.PHONY: all build build-node build-cli clean test run-node install deploy-server

# Binary names
NODE_BINARY=vpn-node
CLI_BINARY=vpn

# Build directories
BUILD_DIR=bin

# Server details
SERVER_IP=95.217.238.72
SERVER_USER=root

all: build

build: build-node build-cli

build-node:
	@echo "Building node daemon..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(NODE_BINARY) ./cmd/vpn-node

build-cli:
	@echo "Building CLI..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(CLI_BINARY) ./cmd/vpn

# Cross-compile for Linux (for deploying to server)
build-linux:
	@echo "Building for Linux..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(NODE_BINARY)-linux ./cmd/vpn-node
	GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(CLI_BINARY)-linux ./cmd/vpn

clean:
	@echo "Cleaning..."
	rm -rf $(BUILD_DIR)

test:
	go test ./...

# Run the node daemon locally in server mode (requires sudo)
run-server:
	sudo $(BUILD_DIR)/$(NODE_BINARY) --server --name local-server --vpn-addr 10.8.0.1 --listen-vpn :8443

# Run the node daemon locally in client mode (requires sudo)
run-client:
	sudo $(BUILD_DIR)/$(NODE_BINARY) --connect localhost:8443 --name local-client

# Run without TUN (for testing control socket only)
run-dev:
	$(BUILD_DIR)/$(NODE_BINARY) --server --name dev-test --listen-control 127.0.0.1:9001

# Install binaries to /usr/local/bin (requires sudo)
install: build
	@echo "Installing to /usr/local/bin..."
	sudo cp $(BUILD_DIR)/$(NODE_BINARY) /usr/local/bin/
	sudo cp $(BUILD_DIR)/$(CLI_BINARY) /usr/local/bin/

# Deploy to remote server
deploy-server: build-linux
	@echo "Deploying to $(SERVER_IP)..."
	scp $(BUILD_DIR)/$(NODE_BINARY)-linux $(SERVER_USER)@$(SERVER_IP):/opt/vpn-node
	scp $(BUILD_DIR)/$(CLI_BINARY)-linux $(SERVER_USER)@$(SERVER_IP):/opt/vpn
	ssh $(SERVER_USER)@$(SERVER_IP) "chmod +x /opt/vpn-node /opt/vpn"
	@echo "Binaries deployed to /opt/"
	@echo "To start: ssh $(SERVER_USER)@$(SERVER_IP) '/opt/vpn-node --server --vpn-addr 10.8.0.1 --listen-vpn :443'"

# Deploy and restart service
deploy-restart: deploy-server
	ssh $(SERVER_USER)@$(SERVER_IP) "systemctl restart vpn-node || /opt/vpn-node --server --vpn-addr 10.8.0.1 --listen-vpn :443 &"

# Download dependencies
deps:
	go mod download
	go mod tidy

# Generate self-signed TLS certificates
certs:
	@mkdir -p certs
	openssl req -x509 -newkey rsa:4096 \
		-keyout certs/server.key -out certs/server.crt \
		-days 365 -nodes \
		-subj "/CN=vpn.local"
	@echo "Certificates generated in certs/"

# Check server status
server-status:
	@echo "Checking server $(SERVER_IP)..."
	@ssh $(SERVER_USER)@$(SERVER_IP) "uptime; ss -tlnp | grep -E '443|9000|9001' || echo 'No VPN ports listening'"

# View server logs
server-logs:
	ssh $(SERVER_USER)@$(SERVER_IP) "journalctl -u vpn-node -f 2>/dev/null || tail -f /var/log/vpn-node.log 2>/dev/null || echo 'No logs found'"

# SSH to server
ssh:
	ssh $(SERVER_USER)@$(SERVER_IP)

# Run all deployment scripts for changed services
deploy-changed:
	./scripts/deploy.sh --changed

# Run deployment for all services
deploy-all:
	./scripts/deploy.sh --all
