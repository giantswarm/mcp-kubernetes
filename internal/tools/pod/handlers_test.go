package pod

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/giantswarm/mcp-kubernetes/internal/k8s"
	"github.com/giantswarm/mcp-kubernetes/internal/server"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

// MockK8sClient is a mock implementation of the k8s.Client interface
type MockK8sClient struct {
	mock.Mock
}

func (m *MockK8sClient) ListContexts(ctx context.Context) ([]k8s.ContextInfo, error) {
	args := m.Called(ctx)
	return args.Get(0).([]k8s.ContextInfo), args.Error(1)
}

func (m *MockK8sClient) GetCurrentContext(ctx context.Context) (*k8s.ContextInfo, error) {
	args := m.Called(ctx)
	return args.Get(0).(*k8s.ContextInfo), args.Error(1)
}

func (m *MockK8sClient) SwitchContext(ctx context.Context, contextName string) error {
	args := m.Called(ctx, contextName)
	return args.Error(0)
}

func (m *MockK8sClient) Get(ctx context.Context, kubeContext, namespace, resourceType, name string) (runtime.Object, error) {
	args := m.Called(ctx, kubeContext, namespace, resourceType, name)
	return args.Get(0).(runtime.Object), args.Error(1)
}

func (m *MockK8sClient) List(ctx context.Context, kubeContext, namespace, resourceType string, opts k8s.ListOptions) ([]runtime.Object, error) {
	args := m.Called(ctx, kubeContext, namespace, resourceType, opts)
	return args.Get(0).([]runtime.Object), args.Error(1)
}

func (m *MockK8sClient) Describe(ctx context.Context, kubeContext, namespace, resourceType, name string) (*k8s.ResourceDescription, error) {
	args := m.Called(ctx, kubeContext, namespace, resourceType, name)
	return args.Get(0).(*k8s.ResourceDescription), args.Error(1)
}

func (m *MockK8sClient) Create(ctx context.Context, kubeContext, namespace string, obj runtime.Object) (runtime.Object, error) {
	args := m.Called(ctx, kubeContext, namespace, obj)
	return args.Get(0).(runtime.Object), args.Error(1)
}

func (m *MockK8sClient) Apply(ctx context.Context, kubeContext, namespace string, obj runtime.Object) (runtime.Object, error) {
	args := m.Called(ctx, kubeContext, namespace, obj)
	return args.Get(0).(runtime.Object), args.Error(1)
}

func (m *MockK8sClient) Delete(ctx context.Context, kubeContext, namespace, resourceType, name string) error {
	args := m.Called(ctx, kubeContext, namespace, resourceType, name)
	return args.Error(0)
}

func (m *MockK8sClient) Patch(ctx context.Context, kubeContext, namespace, resourceType, name string, patchType types.PatchType, data []byte) (runtime.Object, error) {
	args := m.Called(ctx, kubeContext, namespace, resourceType, name, patchType, data)
	return args.Get(0).(runtime.Object), args.Error(1)
}

func (m *MockK8sClient) Scale(ctx context.Context, kubeContext, namespace, resourceType, name string, replicas int32) error {
	args := m.Called(ctx, kubeContext, namespace, resourceType, name, replicas)
	return args.Error(0)
}

func (m *MockK8sClient) GetLogs(ctx context.Context, kubeContext, namespace, podName, containerName string, opts k8s.LogOptions) (io.ReadCloser, error) {
	args := m.Called(ctx, kubeContext, namespace, podName, containerName, opts)
	return args.Get(0).(io.ReadCloser), args.Error(1)
}

func (m *MockK8sClient) Exec(ctx context.Context, kubeContext, namespace, podName, containerName string, command []string, opts k8s.ExecOptions) (*k8s.ExecResult, error) {
	args := m.Called(ctx, kubeContext, namespace, podName, containerName, command, opts)
	return args.Get(0).(*k8s.ExecResult), args.Error(1)
}

func (m *MockK8sClient) PortForward(ctx context.Context, kubeContext, namespace, podName string, ports []string, opts k8s.PortForwardOptions) (*k8s.PortForwardSession, error) {
	args := m.Called(ctx, kubeContext, namespace, podName, ports, opts)
	return args.Get(0).(*k8s.PortForwardSession), args.Error(1)
}

func (m *MockK8sClient) PortForwardToService(ctx context.Context, kubeContext, namespace, serviceName string, ports []string, opts k8s.PortForwardOptions) (*k8s.PortForwardSession, error) {
	args := m.Called(ctx, kubeContext, namespace, serviceName, ports, opts)
	return args.Get(0).(*k8s.PortForwardSession), args.Error(1)
}

func (m *MockK8sClient) GetAPIResources(ctx context.Context, kubeContext string) ([]k8s.APIResourceInfo, error) {
	args := m.Called(ctx, kubeContext)
	return args.Get(0).([]k8s.APIResourceInfo), args.Error(1)
}

func (m *MockK8sClient) GetClusterHealth(ctx context.Context, kubeContext string) (*k8s.ClusterHealth, error) {
	args := m.Called(ctx, kubeContext)
	return args.Get(0).(*k8s.ClusterHealth), args.Error(1)
}

// MockLogger is a mock implementation of the server.Logger interface
type MockLogger struct {
	mock.Mock
}

func (m *MockLogger) Info(msg string, args ...interface{}) {
	m.Called(msg, args)
}

func (m *MockLogger) Debug(msg string, args ...interface{}) {
	m.Called(msg, args)
}

func (m *MockLogger) Warn(msg string, args ...interface{}) {
	m.Called(msg, args)
}

func (m *MockLogger) Error(msg string, args ...interface{}) {
	m.Called(msg, args)
}

func (m *MockLogger) With(args ...interface{}) server.Logger {
	mockArgs := m.Called(args)
	return mockArgs.Get(0).(server.Logger)
}

// MockReadCloser is a mock implementation of io.ReadCloser for testing log responses
type MockReadCloser struct {
	*strings.Reader
	closed bool
}

func NewMockReadCloser(content string) *MockReadCloser {
	return &MockReadCloser{
		Reader: strings.NewReader(content),
		closed: false,
	}
}

func (m *MockReadCloser) Close() error {
	m.closed = true
	return nil
}

func (m *MockReadCloser) IsClosed() bool {
	return m.closed
}

// Helper function to create a test ServerContext
func createTestServerContext(k8sClient k8s.Client, logger server.Logger) *server.ServerContext {
	ctx := context.Background()
	sc, err := server.NewServerContext(ctx,
		server.WithK8sClient(k8sClient),
		server.WithLogger(logger),
		server.WithNonDestructiveMode(false),
	)
	if err != nil {
		panic(err) // Should not happen in tests
	}
	return sc
}

// Helper function to create a CallToolRequest with proper structure
func createCallToolRequest(args map[string]interface{}) mcp.CallToolRequest {
	// Create request with arguments directly, as that's how the handlers access them
	request := mcp.CallToolRequest{}
	// Mock GetArguments to return our test args
	return request
}

// Helper function to get text content from CallToolResult
func getTextContent(result *mcp.CallToolResult) string {
	if len(result.Content) > 0 {
		if textContent, ok := mcp.AsTextContent(result.Content[0]); ok {
			return textContent.Text
		}
	}
	return ""
}

// mockCallToolRequest is a simple mock that implements the interface needed by handlers
type mockCallToolRequest struct {
	args map[string]interface{}
}

func (m mockCallToolRequest) GetArguments() map[string]interface{} {
	return m.args
}

// Test handleGetLogs function
func TestHandleGetLogs(t *testing.T) {
	tests := []struct {
		name           string
		args           map[string]interface{}
		setupMock      func(*MockK8sClient)
		expectedError  bool
		expectedResult string
		errorContains  string
	}{
		{
			name: "successful log retrieval",
			args: map[string]interface{}{
				"namespace": "default",
				"podName":   "test-pod",
			},
			setupMock: func(m *MockK8sClient) {
				logContent := "log line 1\nlog line 2\n"
				mockReader := NewMockReadCloser(logContent)
				m.On("GetLogs", mock.Anything, "", "default", "test-pod", "", k8s.LogOptions{}).Return(mockReader, nil)
			},
			expectedError:  false,
			expectedResult: "log line 1\nlog line 2\n",
		},
		{
			name: "successful log retrieval with all options",
			args: map[string]interface{}{
				"kubeContext":   "test-context",
				"namespace":     "kube-system",
				"podName":       "my-pod",
				"containerName": "main-container",
				"follow":        true,
				"previous":      true,
				"timestamps":    true,
				"tailLines":     float64(100),
			},
			setupMock: func(m *MockK8sClient) {
				logContent := "2023-01-01T10:00:00Z log line 1\n2023-01-01T10:00:01Z log line 2\n"
				mockReader := NewMockReadCloser(logContent)
				tailLines := int64(100)
				expectedOpts := k8s.LogOptions{
					Follow:     true,
					Previous:   true,
					Timestamps: true,
					TailLines:  &tailLines,
				}
				m.On("GetLogs", mock.Anything, "test-context", "kube-system", "my-pod", "main-container", expectedOpts).Return(mockReader, nil)
			},
			expectedError:  false,
			expectedResult: "2023-01-01T10:00:00Z log line 1\n2023-01-01T10:00:01Z log line 2\n",
		},
		{
			name: "missing namespace",
			args: map[string]interface{}{
				"podName": "test-pod",
			},
			setupMock:     func(m *MockK8sClient) {},
			expectedError: true,
			errorContains: "namespace is required",
		},
		{
			name: "empty namespace",
			args: map[string]interface{}{
				"namespace": "",
				"podName":   "test-pod",
			},
			setupMock:     func(m *MockK8sClient) {},
			expectedError: true,
			errorContains: "namespace is required",
		},
		{
			name: "missing podName",
			args: map[string]interface{}{
				"namespace": "default",
			},
			setupMock:     func(m *MockK8sClient) {},
			expectedError: true,
			errorContains: "podName is required",
		},
		{
			name: "empty podName",
			args: map[string]interface{}{
				"namespace": "default",
				"podName":   "",
			},
			setupMock:     func(m *MockK8sClient) {},
			expectedError: true,
			errorContains: "podName is required",
		},
		{
			name: "k8s client error",
			args: map[string]interface{}{
				"namespace": "default",
				"podName":   "test-pod",
			},
			setupMock: func(m *MockK8sClient) {
				m.On("GetLogs", mock.Anything, "", "default", "test-pod", "", k8s.LogOptions{}).Return((*MockReadCloser)(nil), errors.New("pod not found"))
			},
			expectedError: true,
			errorContains: "Failed to get logs: pod not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockK8sClient := &MockK8sClient{}
			mockLogger := &MockLogger{}
			tt.setupMock(mockK8sClient)

			// Allow any logger calls (we're not testing logging in detail here)
			mockLogger.On("Debug", mock.Anything, mock.Anything).Maybe()
			mockLogger.On("Info", mock.Anything, mock.Anything).Maybe()

			// Create ServerContext
			sc := createTestServerContext(mockK8sClient, mockLogger)
			defer sc.Shutdown()

			// Create a simple request that mimics how real requests work
			// We need to create a mock that returns our test arguments when GetArguments() is called
			request := mockCallToolRequest{args: tt.args}

			// Call the handler
			result, err := handleGetLogs(context.Background(), request, sc)

			// Assertions
			require.NoError(t, err) // handleGetLogs should never return an error directly
			require.NotNil(t, result)

			if tt.expectedError {
				assert.True(t, result.IsError)
				// For error results, check the text content of the first Content item
				content := getTextContent(result)
				assert.Contains(t, content, tt.errorContains)
			} else {
				assert.False(t, result.IsError)
				content := getTextContent(result)
				assert.Equal(t, tt.expectedResult, content)
			}

			// Verify all mock expectations
			mockK8sClient.AssertExpectations(t)
		})
	}
}

// Test handleExec function
func TestHandleExec(t *testing.T) {
	tests := []struct {
		name           string
		args           map[string]interface{}
		setupMock      func(*MockK8sClient)
		expectedError  bool
		expectedResult string
		errorContains  string
	}{
		{
			name: "successful command execution",
			args: map[string]interface{}{
				"namespace": "default",
				"podName":   "test-pod",
				"command":   []interface{}{"echo", "hello"},
			},
			setupMock: func(m *MockK8sClient) {
				execResult := &k8s.ExecResult{
					ExitCode: 0,
					Stdout:   "hello\n",
					Stderr:   "",
				}
				m.On("Exec", mock.Anything, "", "default", "test-pod", "", []string{"echo", "hello"}, k8s.ExecOptions{}).Return(execResult, nil)
			},
			expectedError:  false,
			expectedResult: "Exit Code: 0\nStdout:\nhello\n\n",
		},
		{
			name: "successful command execution with all options",
			args: map[string]interface{}{
				"kubeContext":   "test-context",
				"namespace":     "kube-system",
				"podName":       "my-pod",
				"containerName": "main-container",
				"command":       []interface{}{"ls", "-la", "/tmp"},
				"tty":           true,
			},
			setupMock: func(m *MockK8sClient) {
				execResult := &k8s.ExecResult{
					ExitCode: 0,
					Stdout:   "total 0\ndrwxrwxrwt 2 root root 40 Jan  1 10:00 .\n",
					Stderr:   "",
				}
				expectedOpts := k8s.ExecOptions{TTY: true}
				m.On("Exec", mock.Anything, "test-context", "kube-system", "my-pod", "main-container", []string{"ls", "-la", "/tmp"}, expectedOpts).Return(execResult, nil)
			},
			expectedError:  false,
			expectedResult: "Exit Code: 0\nStdout:\ntotal 0\ndrwxrwxrwt 2 root root 40 Jan  1 10:00 .\n\n",
		},
		{
			name: "command execution with stderr",
			args: map[string]interface{}{
				"namespace": "default",
				"podName":   "test-pod",
				"command":   []interface{}{"ls", "/nonexistent"},
			},
			setupMock: func(m *MockK8sClient) {
				execResult := &k8s.ExecResult{
					ExitCode: 2,
					Stdout:   "",
					Stderr:   "ls: cannot access '/nonexistent': No such file or directory\n",
				}
				m.On("Exec", mock.Anything, "", "default", "test-pod", "", []string{"ls", "/nonexistent"}, k8s.ExecOptions{}).Return(execResult, nil)
			},
			expectedError:  false,
			expectedResult: "Exit Code: 2\nStderr:\nls: cannot access '/nonexistent': No such file or directory\n\n",
		},
		{
			name: "missing namespace",
			args: map[string]interface{}{
				"podName": "test-pod",
				"command": []interface{}{"echo", "hello"},
			},
			setupMock:     func(m *MockK8sClient) {},
			expectedError: true,
			errorContains: "namespace is required",
		},
		{
			name: "missing podName",
			args: map[string]interface{}{
				"namespace": "default",
				"command":   []interface{}{"echo", "hello"},
			},
			setupMock:     func(m *MockK8sClient) {},
			expectedError: true,
			errorContains: "podName is required",
		},
		{
			name: "missing command",
			args: map[string]interface{}{
				"namespace": "default",
				"podName":   "test-pod",
			},
			setupMock:     func(m *MockK8sClient) {},
			expectedError: true,
			errorContains: "command is required",
		},
		{
			name: "empty command array",
			args: map[string]interface{}{
				"namespace": "default",
				"podName":   "test-pod",
				"command":   []interface{}{},
			},
			setupMock:     func(m *MockK8sClient) {},
			expectedError: true,
			errorContains: "command cannot be empty",
		},
		{
			name: "invalid command type",
			args: map[string]interface{}{
				"namespace": "default",
				"podName":   "test-pod",
				"command":   "echo hello",
			},
			setupMock:     func(m *MockK8sClient) {},
			expectedError: true,
			errorContains: "command must be an array of strings",
		},
		{
			name: "k8s client error",
			args: map[string]interface{}{
				"namespace": "default",
				"podName":   "test-pod",
				"command":   []interface{}{"echo", "hello"},
			},
			setupMock: func(m *MockK8sClient) {
				m.On("Exec", mock.Anything, "", "default", "test-pod", "", []string{"echo", "hello"}, k8s.ExecOptions{}).Return((*k8s.ExecResult)(nil), errors.New("pod not found"))
			},
			expectedError: true,
			errorContains: "Failed to execute command: pod not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockK8sClient := &MockK8sClient{}
			mockLogger := &MockLogger{}
			tt.setupMock(mockK8sClient)

			// Allow any logger calls
			mockLogger.On("Debug", mock.Anything, mock.Anything).Maybe()
			mockLogger.On("Info", mock.Anything, mock.Anything).Maybe()

			// Create ServerContext
			sc := createTestServerContext(mockK8sClient, mockLogger)
			defer sc.Shutdown()

			// Create request
			request := mockCallToolRequest{args: tt.args}

			// Call the handler
			result, err := handleExec(context.Background(), request, sc)

			// Assertions
			require.NoError(t, err)
			require.NotNil(t, result)

			if tt.expectedError {
				assert.True(t, result.IsError)
				content := getTextContent(result)
				assert.Contains(t, content, tt.errorContains)
			} else {
				assert.False(t, result.IsError)
				content := getTextContent(result)
				assert.Equal(t, tt.expectedResult, content)
			}

			// Verify all mock expectations
			mockK8sClient.AssertExpectations(t)
		})
	}
}

// Test handlePortForward function
func TestHandlePortForward(t *testing.T) {
	tests := []struct {
		name          string
		args          map[string]interface{}
		setupMock     func(*MockK8sClient)
		expectedError bool
		errorContains string
		checkResult   func(*testing.T, *mcp.CallToolResult)
	}{
		{
			name: "successful port forward to pod",
			args: map[string]interface{}{
				"namespace":    "default",
				"resourceName": "test-pod",
				"ports":        []interface{}{"8080:80"},
			},
			setupMock: func(m *MockK8sClient) {
				session := &k8s.PortForwardSession{
					LocalPorts:  []int{8080},
					RemotePorts: []int{80},
					StopChan:    make(chan struct{}),
					ReadyChan:   make(chan struct{}),
				}
				m.On("PortForward", mock.Anything, "", "default", "test-pod", []string{"8080:80"}, k8s.PortForwardOptions{}).Return(session, nil)
			},
			expectedError: false,
			checkResult: func(t *testing.T, result *mcp.CallToolResult) {
				content := getTextContent(result)
				assert.Contains(t, content, "Port forwarding session established to pod test-pod")
				assert.Contains(t, content, "Local port 8080 -> Remote port 80")
			},
		},
		{
			name: "successful port forward to service",
			args: map[string]interface{}{
				"namespace":    "default",
				"resourceType": "service",
				"resourceName": "test-service",
				"ports":        []interface{}{"8080:80", "9090:9090"},
			},
			setupMock: func(m *MockK8sClient) {
				session := &k8s.PortForwardSession{
					LocalPorts:  []int{8080, 9090},
					RemotePorts: []int{80, 9090},
					StopChan:    make(chan struct{}),
					ReadyChan:   make(chan struct{}),
				}
				m.On("PortForwardToService", mock.Anything, "", "default", "test-service", []string{"8080:80", "9090:9090"}, k8s.PortForwardOptions{}).Return(session, nil)
			},
			expectedError: false,
			checkResult: func(t *testing.T, result *mcp.CallToolResult) {
				content := getTextContent(result)
				assert.Contains(t, content, "Port forwarding session established to service test-service")
				assert.Contains(t, content, "Local port 8080 -> Remote port 80")
				assert.Contains(t, content, "Local port 9090 -> Remote port 9090")
			},
		},
		{
			name: "backward compatibility with podName",
			args: map[string]interface{}{
				"namespace": "default",
				"podName":   "old-pod",
				"ports":     []interface{}{"8080:80"},
			},
			setupMock: func(m *MockK8sClient) {
				session := &k8s.PortForwardSession{
					LocalPorts:  []int{8080},
					RemotePorts: []int{80},
					StopChan:    make(chan struct{}),
					ReadyChan:   make(chan struct{}),
				}
				m.On("PortForward", mock.Anything, "", "default", "old-pod", []string{"8080:80"}, k8s.PortForwardOptions{}).Return(session, nil)
			},
			expectedError: false,
			checkResult: func(t *testing.T, result *mcp.CallToolResult) {
				content := getTextContent(result)
				assert.Contains(t, content, "Port forwarding session established to pod old-pod")
			},
		},
		{
			name: "missing namespace",
			args: map[string]interface{}{
				"resourceName": "test-pod",
				"ports":        []interface{}{"8080:80"},
			},
			setupMock:     func(m *MockK8sClient) {},
			expectedError: true,
			errorContains: "namespace is required",
		},
		{
			name: "missing resourceName and podName",
			args: map[string]interface{}{
				"namespace": "default",
				"ports":     []interface{}{"8080:80"},
			},
			setupMock:     func(m *MockK8sClient) {},
			expectedError: true,
			errorContains: "resourceName is required",
		},
		{
			name: "missing ports",
			args: map[string]interface{}{
				"namespace":    "default",
				"resourceName": "test-pod",
			},
			setupMock:     func(m *MockK8sClient) {},
			expectedError: true,
			errorContains: "ports is required",
		},
		{
			name: "empty ports array",
			args: map[string]interface{}{
				"namespace":    "default",
				"resourceName": "test-pod",
				"ports":        []interface{}{},
			},
			setupMock:     func(m *MockK8sClient) {},
			expectedError: true,
			errorContains: "ports cannot be empty",
		},
		{
			name: "invalid resource type",
			args: map[string]interface{}{
				"namespace":    "default",
				"resourceType": "deployment",
				"resourceName": "test-deployment",
				"ports":        []interface{}{"8080:80"},
			},
			setupMock:     func(m *MockK8sClient) {},
			expectedError: true,
			errorContains: "Invalid resource type: deployment. Must be 'pod' or 'service'",
		},
		{
			name: "k8s client error for pod",
			args: map[string]interface{}{
				"namespace":    "default",
				"resourceName": "test-pod",
				"ports":        []interface{}{"8080:80"},
			},
			setupMock: func(m *MockK8sClient) {
				m.On("PortForward", mock.Anything, "", "default", "test-pod", []string{"8080:80"}, k8s.PortForwardOptions{}).Return((*k8s.PortForwardSession)(nil), errors.New("pod not found"))
			},
			expectedError: true,
			errorContains: "Failed to setup port forwarding to pod: pod not found",
		},
		{
			name: "k8s client error for service",
			args: map[string]interface{}{
				"namespace":    "default",
				"resourceType": "service",
				"resourceName": "test-service",
				"ports":        []interface{}{"8080:80"},
			},
			setupMock: func(m *MockK8sClient) {
				m.On("PortForwardToService", mock.Anything, "", "default", "test-service", []string{"8080:80"}, k8s.PortForwardOptions{}).Return((*k8s.PortForwardSession)(nil), errors.New("service not found"))
			},
			expectedError: true,
			errorContains: "Failed to setup port forwarding to service: service not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockK8sClient := &MockK8sClient{}
			mockLogger := &MockLogger{}
			tt.setupMock(mockK8sClient)

			// Allow any logger calls
			mockLogger.On("Debug", mock.Anything, mock.Anything).Maybe()
			mockLogger.On("Info", mock.Anything, mock.Anything).Maybe()

			// Create ServerContext
			sc := createTestServerContext(mockK8sClient, mockLogger)
			defer sc.Shutdown()

			// Create request
			request := mockCallToolRequest{args: tt.args}

			// Call the handler
			result, err := handlePortForward(context.Background(), request, sc)

			// Assertions
			require.NoError(t, err)
			require.NotNil(t, result)

			if tt.expectedError {
				assert.True(t, result.IsError)
				content := getTextContent(result)
				assert.Contains(t, content, tt.errorContains)
			} else {
				assert.False(t, result.IsError)
				if tt.checkResult != nil {
					tt.checkResult(t, result)
				}
			}

			// Verify all mock expectations
			mockK8sClient.AssertExpectations(t)
		})
	}
}

// Test handleListPortForwardSessions function
func TestHandleListPortForwardSessions(t *testing.T) {
	tests := []struct {
		name            string
		setupSessions   func(*server.ServerContext)
		expectedError   bool
		expectedContent string
	}{
		{
			name:            "no active sessions",
			setupSessions:   func(sc *server.ServerContext) {},
			expectedError:   false,
			expectedContent: "No active port forwarding sessions.",
		},
		{
			name: "one active session",
			setupSessions: func(sc *server.ServerContext) {
				session := &k8s.PortForwardSession{
					LocalPorts:  []int{8080},
					RemotePorts: []int{80},
				}
				sc.RegisterPortForwardSession("default/test-pod:8080:80", session)
			},
			expectedError:   false,
			expectedContent: "Active port forwarding sessions (1):",
		},
		{
			name: "multiple active sessions",
			setupSessions: func(sc *server.ServerContext) {
				session1 := &k8s.PortForwardSession{
					LocalPorts:  []int{8080},
					RemotePorts: []int{80},
				}
				session2 := &k8s.PortForwardSession{
					LocalPorts:  []int{9090, 9091},
					RemotePorts: []int{9090, 9091},
				}
				sc.RegisterPortForwardSession("default/pod1:8080:80", session1)
				sc.RegisterPortForwardSession("kube-system/service/svc1:9090,9091", session2)
			},
			expectedError:   false,
			expectedContent: "Active port forwarding sessions (2):",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockK8sClient := &MockK8sClient{}
			mockLogger := &MockLogger{}

			// Allow any logger calls
			mockLogger.On("Debug", mock.Anything, mock.Anything).Maybe()
			mockLogger.On("Info", mock.Anything, mock.Anything).Maybe()

			// Create ServerContext
			sc := createTestServerContext(mockK8sClient, mockLogger)
			defer sc.Shutdown()

			// Setup sessions
			tt.setupSessions(sc)

			// Create request
			request := mockCallToolRequest{args: map[string]interface{}{}}

			// Call the handler
			result, err := handleListPortForwardSessions(context.Background(), request, sc)

			// Assertions
			require.NoError(t, err)
			require.NotNil(t, result)

			if tt.expectedError {
				assert.True(t, result.IsError)
			} else {
				assert.False(t, result.IsError)
				content := getTextContent(result)
				assert.Contains(t, content, tt.expectedContent)
			}
		})
	}
}

// Test handleStopPortForwardSession function
func TestHandleStopPortForwardSession(t *testing.T) {
	tests := []struct {
		name            string
		args            map[string]interface{}
		setupSessions   func(*server.ServerContext)
		expectedError   bool
		errorContains   string
		expectedContent string
	}{
		{
			name: "successfully stop existing session",
			args: map[string]interface{}{
				"sessionID": "default/test-pod:8080:80",
			},
			setupSessions: func(sc *server.ServerContext) {
				session := &k8s.PortForwardSession{
					LocalPorts:  []int{8080},
					RemotePorts: []int{80},
					StopChan:    make(chan struct{}, 1),
				}
				sc.RegisterPortForwardSession("default/test-pod:8080:80", session)
			},
			expectedError:   false,
			expectedContent: "Port forwarding session default/test-pod:8080:80 stopped successfully.",
		},
		{
			name: "session not found",
			args: map[string]interface{}{
				"sessionID": "nonexistent-session",
			},
			setupSessions: func(sc *server.ServerContext) {},
			expectedError: true,
			errorContains: "Failed to stop session: session nonexistent-session not found",
		},
		{
			name:          "missing sessionID",
			args:          map[string]interface{}{},
			setupSessions: func(sc *server.ServerContext) {},
			expectedError: true,
			errorContains: "sessionID is required",
		},
		{
			name: "empty sessionID",
			args: map[string]interface{}{
				"sessionID": "",
			},
			setupSessions: func(sc *server.ServerContext) {},
			expectedError: true,
			errorContains: "sessionID is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockK8sClient := &MockK8sClient{}
			mockLogger := &MockLogger{}

			// Allow any logger calls
			mockLogger.On("Debug", mock.Anything, mock.Anything).Maybe()
			mockLogger.On("Info", mock.Anything, mock.Anything).Maybe()

			// Create ServerContext
			sc := createTestServerContext(mockK8sClient, mockLogger)
			defer sc.Shutdown()

			// Setup sessions
			tt.setupSessions(sc)

			// Create request
			request := mockCallToolRequest{args: tt.args}

			// Call the handler
			result, err := handleStopPortForwardSession(context.Background(), request, sc)

			// Assertions
			require.NoError(t, err)
			require.NotNil(t, result)

			if tt.expectedError {
				assert.True(t, result.IsError)
				if tt.errorContains != "" {
					content := getTextContent(result)
					assert.Contains(t, content, tt.errorContains)
				}
			} else {
				assert.False(t, result.IsError)
				if tt.expectedContent != "" {
					content := getTextContent(result)
					assert.Contains(t, content, tt.expectedContent)
				}
			}
		})
	}
}

// Test handleStopAllPortForwardSessions function
func TestHandleStopAllPortForwardSessions(t *testing.T) {
	tests := []struct {
		name            string
		setupSessions   func(*server.ServerContext)
		expectedError   bool
		expectedContent string
	}{
		{
			name:            "no sessions to stop",
			setupSessions:   func(sc *server.ServerContext) {},
			expectedError:   false,
			expectedContent: "No active port forwarding sessions to stop.",
		},
		{
			name: "stop one session",
			setupSessions: func(sc *server.ServerContext) {
				session := &k8s.PortForwardSession{
					LocalPorts:  []int{8080},
					RemotePorts: []int{80},
					StopChan:    make(chan struct{}, 1),
				}
				sc.RegisterPortForwardSession("default/test-pod:8080:80", session)
			},
			expectedError:   false,
			expectedContent: "Stopped 1 port forwarding session(s) successfully.",
		},
		{
			name: "stop multiple sessions",
			setupSessions: func(sc *server.ServerContext) {
				session1 := &k8s.PortForwardSession{
					LocalPorts:  []int{8080},
					RemotePorts: []int{80},
					StopChan:    make(chan struct{}, 1),
				}
				session2 := &k8s.PortForwardSession{
					LocalPorts:  []int{9090},
					RemotePorts: []int{9090},
					StopChan:    make(chan struct{}, 1),
				}
				sc.RegisterPortForwardSession("default/pod1:8080:80", session1)
				sc.RegisterPortForwardSession("default/pod2:9090:9090", session2)
			},
			expectedError:   false,
			expectedContent: "Stopped 2 port forwarding session(s) successfully.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockK8sClient := &MockK8sClient{}
			mockLogger := &MockLogger{}

			// Allow any logger calls
			mockLogger.On("Debug", mock.Anything, mock.Anything).Maybe()
			mockLogger.On("Info", mock.Anything, mock.Anything).Maybe()

			// Create ServerContext
			sc := createTestServerContext(mockK8sClient, mockLogger)
			defer sc.Shutdown()

			// Setup sessions
			tt.setupSessions(sc)

			// Create request
			request := mockCallToolRequest{args: map[string]interface{}{}}

			// Call the handler
			result, err := handleStopAllPortForwardSessions(context.Background(), request, sc)

			// Assertions
			require.NoError(t, err)
			require.NotNil(t, result)

			if tt.expectedError {
				assert.True(t, result.IsError)
			} else {
				assert.False(t, result.IsError)
				content := getTextContent(result)
				assert.Contains(t, content, tt.expectedContent)
			}
		})
	}
}
