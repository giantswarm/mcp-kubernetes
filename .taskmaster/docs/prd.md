# Product Requirements Document: MCP Kubernetes Server

## 1. Overview

The project is to create a new MCP (Model Context Protocol) server for interacting with Kubernetes clusters. The server will be written in Go. It will expose Kubernetes cluster operations as tools that an MCP client (like an AI agent) can call.

## 2. Core Technologies

*   **Language:** Go
*   **MCP Library:** `mark3labs/mcp-go`
*   **Kubernetes Interaction:** `client-go`. The use of `kubectl` is strictly forbidden. All interactions with the Kubernetes API must be done programmatically using the `client-go` library.
*   **Protocol:** The server must strictly adhere to the [Model Context Protocol (MCP) specification](https://modelcontextprotocol.io/llms-full.txt).

## 3. Feature Set

The goal is to build a fully functional mcp server for kubernetes with a clean implementation based on `client-go`.

The server should provide tools for common Kubernetes operations, including but not limited to:
*   Listing resources (Pods, Deployments, Services, Namespaces, etc.)
*   Getting detailed information about a specific resource (e.g., `describe pod <pod-name>`).
*   Viewing logs from a pod.
*   Applying and deleting Kubernetes manifests (from file or string content).
*   Managing contexts, clusters, and users from kubeconfig.
*   Checking health and status of cluster components.

**Crucially, all tools that interact with a Kubernetes cluster must accept a `context` parameter.** This allows the user to specify the target cluster for each operation, enabling multi-cluster management.

The implementation should also draw inspiration from other Giant Swarm MCP servers for best practices and architecture:
*   `/home/teemow/projects/giantswarm/mcp-capi`
*   `/home/teemow/projects/giantswarm/mcp-gs-apps/`

## 4. Detailed Tool Requirements

The following tools should be implemented:

### Core Kubernetes Operations
- **kubernetes_get**: Get specific resources by name/type with flexible output formats
- **kubernetes_list**: List resources of a specific type with optional filtering
- **kubernetes_describe**: Get detailed information about a resource
- **kubernetes_create**: Create resources from YAML/JSON definitions
- **kubernetes_apply**: Apply configurations to resources (create or update)
- **kubernetes_delete**: Delete resources by name/type
- **kubernetes_logs**: View logs from pods/containers
- **kubernetes_scale**: Scale deployments, replica sets, or stateful sets
- **kubernetes_patch**: Update specific fields of a resource
- **kubernetes_rollout**: Manage deployment rollouts (status, history, undo, restart)

### Context & Configuration
- **kubernetes_context**: Manage kubectl contexts (list, get current, switch)

### Information & Discovery
- **kubernetes_explain_resource**: Get documentation for Kubernetes resource types
- **kubernetes_list_api_resources**: List all available API resources in the cluster

### Advanced Operations
- **port_forward**: Set up port forwarding to pods/services

## 5. Architecture Patterns

Based on analysis of Giant Swarm Go MCP implementations:

### Project Structure
```
mcp-kubernetes/
├── cmd/
│   └── mcp-kubernetes/
│       ├── main.go          # Server initialization and setup
│       ├── tools.go         # Tool registration
│       └── handlers.go      # Tool handler implementations
├── internal/
│   ├── k8s/                 # Kubernetes client wrapper
│   │   ├── client.go        # Client initialization
│   │   ├── resources.go     # Resource operations
│   │   ├── contexts.go      # Context management
│   │   └── portforward.go   # Port forwarding logic
│   └── server/              # MCP server utilities
│       └── context.go       # Server context management
├── pkg/                     # Public packages (if any)
├── go.mod
├── go.sum
├── Makefile
├── Dockerfile
└── README.md
```

### Design Patterns
1. **ServerContext Pattern**: Share resources (like k8s client) across handlers
2. **Functional Options**: Use the mcp-go library's functional options for tool configuration
3. **Error Handling**: Wrap errors with context, use MCP error responses appropriately
4. **Context Propagation**: Pass Go context through all operations for cancellation support

## 6. Implementation Plan

The implementation should follow a structured plan:
1.  **Project Setup:** Initialize a Go project with proper dependency management (`go mod`).
2.  **Core Server:** Implement the basic MCP server structure using `mark3labs/mcp-go`.
3.  **Kubernetes Client:** Implement a robust Kubernetes client service using `client-go`. This service will handle all interactions with the Kubernetes API and should be easily injectable into the MCP tools.
4.  **Tool Implementation:** Implement the features listed in section 3 as individual MCP tools. Each tool should be well-defined and testable.
5.  **Testing:** Implement comprehensive unit tests for all components, especially the Kubernetes client service and the MCP tools.
6.  **Packaging:** Create a Dockerfile for containerizing the server and a Makefile for easy building and testing.

## 7. Non-Functional Requirements

*   **Decoupling:** The Kubernetes interaction logic should be decoupled from the MCP server logic to allow for independent testing and maintenance.
*   **Error Handling:** The server must have robust error handling and provide clear error messages to the MCP client.
*   **Security:** The server should handle kubeconfig and credentials securely.
*   **Performance:** Operations should be efficient and support timeouts/cancellation

## 8. Testing Strategy

1. **Unit Tests**: Test individual components in isolation
2. **Integration Tests**: Test tool handlers with mock Kubernetes clients
3. **Coverage Requirements**: Minimum 80% test coverage as per workspace standards

## 9. Additional Requirements

- All tools must support the kubeconfig `context` parameter for multi-cluster operations