# MCP Kubernetes

An MCP (Model Context Protocol) server that provides comprehensive Kubernetes management capabilities through structured tools and interfaces.

## Overview

This project implements an MCP server for Kubernetes operations, providing a standardized interface for AI agents and tools to interact with Kubernetes clusters. It supports multi-cluster operations, non-destructive mode, and comprehensive resource management.

## Architecture

### Project Structure

```
cmd/mcp-kubernetes/           # Main application entrypoint
internal/k8s/                 # Kubernetes client and helpers
internal/server/              # MCP server logic and ServerContext
internal/tools/               # MCP tool implementations
├── context/                  # Context management tools
├── resource/                 # Resource management tools
├── pod/                      # Pod operations tools
├── cluster/                  # Cluster management tools
└── helm/                     # Helm chart management tools
internal/security/            # Security enhancements
internal/util/                # Utility functions
pkg/                          # Public packages
test/integration/             # Integration tests
docs/                         # Tool documentation
```

### Key Features

- **Multi-cluster Support**: Manage multiple Kubernetes clusters seamlessly
- **Non-destructive Mode**: Safe operations with validation and confirmation
- **Comprehensive Tool Set**: Full range of kubectl and helm operations
- **ServerContext Pattern**: Decoupled, testable architecture
- **Security First**: Authentication, authorization, and secure credential handling

### Supported Operations

#### Context Management
- List available contexts
- Get current context
- Switch between contexts

#### Resource Management
- Get, list, create, apply, delete resources
- Describe resources and patch operations
- Scale deployments and other scalable resources

#### Pod Operations
- View logs with filtering and streaming
- Execute commands in pods
- Port forwarding capabilities

#### Cluster Management
- API resource discovery
- Cluster health monitoring

#### Helm Operations
- Install, upgrade, and uninstall charts
- List Helm releases

## Development

### Prerequisites

- Go 1.21+
- Access to Kubernetes clusters
- Helm (for Helm operations)

### Building

```bash
go build -o mcp-kubernetes ./cmd/mcp-kubernetes
```

### Testing

```bash
# Run unit tests
make test

# Run integration tests
make test-integration

# Test coverage
make coverage
```

## Configuration

The server supports various configuration options for cluster access, security settings, and operational modes. See the documentation in `docs/` for detailed configuration instructions.

## License

This project is part of Giant Swarm's infrastructure tooling. 