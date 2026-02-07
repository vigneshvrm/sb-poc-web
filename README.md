# StackBill Deployer

Web-based deployment portal for StackBill. Provides a web interface to deploy StackBill to remote servers via SSH.

## Quick Start

Run a single command to get started:

```bash
curl -fsSL https://raw.githubusercontent.com/vigneshvrm/sb-poc-web/main/scripts/deploy.sh | sudo bash
```

This will:
- Install `podman-docker` if no container runtime is found
- Pull and run the StackBill Deployer container
- Display the URL and access token
- Monitor deployment progress and auto-cleanup on completion

## Features

- Web UI for entering server details and deployment options
- Token-based authentication (auto-generated on startup)
- SSH-based remote deployment
- Real-time log streaming via SSE (Server-Sent Events)
- Stage progress sidebar with live status
- Retry failed deployments from where they left off
- CloudStack Simulator configuration
- SSL support (Let's Encrypt or custom certificates)

## Requirements

- Target server: Ubuntu 22.04, 8+ vCPU, 16GB+ RAM (32GB+ with CloudStack Simulator)

## Docker

```bash
docker run -d -p 9876:9876 vickyinfra/sb-poc:latest
```

Check the container logs for the access token:

```bash
docker logs <container_id>
```

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `SB_DEPLOYER_PORT` | `9876` | Server port |
| `SB_AUTH_TOKEN` | (auto-generated) | Access token for the web UI |
| `SB_SCRIPT_PATH` | `scripts/install-stackbill-poc.sh` | Path to install script |
| `SB_TLS_CERT` | | Path to TLS certificate for HTTPS |
| `SB_TLS_KEY` | | Path to TLS private key for HTTPS |

## Development

```bash
go mod tidy
go run main.go
```

## Project Structure

```
stackbill-deployer/
├── main.go                  # Entry point
├── cmd/server/              # Server startup
├── internal/
│   ├── config/              # Configuration
│   ├── deployer/            # SSH deployment logic
│   ├── handlers/            # HTTP & SSE handlers
│   └── models/              # Data models
├── web/
│   ├── static/css/          # Styles
│   ├── static/js/           # Frontend logic
│   └── templates/           # HTML templates
├── scripts/
│   ├── install-stackbill-poc.sh  # StackBill install script
│   └── deploy.sh                 # Quick start script
└── Dockerfile
```
