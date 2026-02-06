# StackBill Deployer

Web-based deployment portal for StackBill. Provides a web interface to deploy StackBill to remote servers via SSH.

## Features

- Web UI for entering server details and deployment options
- SSH-based remote deployment
- Real-time log streaming via WebSocket
- CloudStack Simulator configuration
- SSL and monitoring options

## Requirements

- Go 1.21+
- Target server: Ubuntu 22.04, 8+ vCPU, 16GB+ RAM (32GB+ with CloudStack)

## Quick Start

```bash
# Install dependencies
go mod tidy

# Run the server
go run main.go

# Access at http://localhost:8080
```

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `SB_DEPLOYER_PORT` | `8080` | Server port |
| `SB_SCRIPT_PATH` | `scripts/install-stackbill-poc.sh` | Path to install script |

## Docker

```bash
docker build -t stackbill-deployer .
docker run -p 8080:8080 stackbill-deployer
```

## Project Structure

```
stackbill-deployer/
├── main.go                  # Entry point
├── cmd/server/              # Server startup
├── internal/
│   ├── config/              # Configuration
│   ├── deployer/            # SSH deployment logic
│   ├── handlers/            # HTTP & WebSocket handlers
│   └── models/              # Data models
├── web/
│   ├── static/css/          # Styles
│   ├── static/js/           # Frontend logic
│   └── templates/           # HTML templates
├── scripts/                 # Install scripts
└── Dockerfile
```
