package pod

import (
	"encoding/json"
	"testing"
)

func TestPortForwardResponseStructure(t *testing.T) {
	// Test that the PortForwardResponse struct can be properly marshaled to JSON
	response := PortForwardResponse{
		Success:      true,
		Message:      "Port forwarding session established to service mimir-query-frontend",
		SessionID:    "mimir/service/mimir-query-frontend:8888:8080",
		ResourceType: "service",
		ResourceName: "mimir-query-frontend",
		Namespace:    "mimir",
		PortMappings: []PortMapping{
			{
				LocalPort:  8888,
				RemotePort: 8080,
			},
		},
		Instructions: "This is a long-running session. Use 'list_port_forward_sessions' to view active sessions and 'stop_port_forward_session' to stop this session.",
	}

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal response to JSON: %v", err)
	}

	// Verify the JSON structure
	var parsedResponse PortForwardResponse
	err = json.Unmarshal(jsonData, &parsedResponse)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// Verify key fields
	if parsedResponse.Success != true {
		t.Errorf("Expected Success to be true, got %v", parsedResponse.Success)
	}

	if parsedResponse.ResourceType != "service" {
		t.Errorf("Expected ResourceType to be 'service', got %s", parsedResponse.ResourceType)
	}

	if parsedResponse.ResourceName != "mimir-query-frontend" {
		t.Errorf("Expected ResourceName to be 'mimir-query-frontend', got %s", parsedResponse.ResourceName)
	}

	if parsedResponse.Namespace != "mimir" {
		t.Errorf("Expected Namespace to be 'mimir', got %s", parsedResponse.Namespace)
	}

	if len(parsedResponse.PortMappings) != 1 {
		t.Errorf("Expected 1 port mapping, got %d", len(parsedResponse.PortMappings))
	} else {
		if parsedResponse.PortMappings[0].LocalPort != 8888 {
			t.Errorf("Expected LocalPort to be 8888, got %d", parsedResponse.PortMappings[0].LocalPort)
		}
		if parsedResponse.PortMappings[0].RemotePort != 8080 {
			t.Errorf("Expected RemotePort to be 8080, got %d", parsedResponse.PortMappings[0].RemotePort)
		}
	}

	// Print the JSON for verification
	t.Logf("Generated JSON response:\n%s", string(jsonData))
}
