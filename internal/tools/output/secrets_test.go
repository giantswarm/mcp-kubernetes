package output

import (
	"testing"
)

func TestMaskSecrets(t *testing.T) {
	tests := []struct {
		name       string
		obj        map[string]interface{}
		wantMasked bool
	}{
		{
			name:       "nil object",
			obj:        nil,
			wantMasked: false,
		},
		{
			name: "non-secret resource",
			obj: map[string]interface{}{
				"kind":       "Pod",
				"apiVersion": "v1",
				"metadata": map[string]interface{}{
					"name": "test-pod",
				},
			},
			wantMasked: false,
		},
		{
			name: "secret with data",
			obj: map[string]interface{}{
				"kind":       "Secret",
				"apiVersion": "v1",
				"metadata": map[string]interface{}{
					"name": "test-secret",
				},
				"data": map[string]interface{}{
					"username": "dXNlcm5hbWU=",
					"password": "cGFzc3dvcmQ=",
				},
				"type": "Opaque",
			},
			wantMasked: true,
		},
		{
			name: "secret with stringData",
			obj: map[string]interface{}{
				"kind":       "Secret",
				"apiVersion": "v1",
				"metadata": map[string]interface{}{
					"name": "test-secret",
				},
				"stringData": map[string]interface{}{
					"config": "sensitive-config-data",
				},
				"type": "Opaque",
			},
			wantMasked: true,
		},
		{
			name: "secret - lowercase kind",
			obj: map[string]interface{}{
				"kind": "secret",
				"data": map[string]interface{}{
					"key": "value",
				},
			},
			wantMasked: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaskSecrets(tt.obj)

			if tt.obj == nil {
				if result != nil {
					t.Error("Expected nil result for nil input")
				}
				return
			}

			if tt.wantMasked {
				// Check data is masked
				if data, ok := result["data"].(map[string]interface{}); ok {
					for key, value := range data {
						if value != RedactedValue {
							t.Errorf("data[%s] = %v, want %s", key, value, RedactedValue)
						}
					}
				}

				// Check stringData is masked
				if stringData, ok := result["stringData"].(map[string]interface{}); ok {
					for key, value := range stringData {
						if value != RedactedValue {
							t.Errorf("stringData[%s] = %v, want %s", key, value, RedactedValue)
						}
					}
				}
			}
		})
	}
}

func TestMaskSecrets_PreservesType(t *testing.T) {
	secret := map[string]interface{}{
		"kind":       "Secret",
		"apiVersion": "v1",
		"metadata": map[string]interface{}{
			"name":      "test-secret",
			"namespace": "default",
		},
		"data": map[string]interface{}{
			"key": "c2VjcmV0",
		},
		"type": "kubernetes.io/tls",
	}

	result := MaskSecrets(secret)

	// Type should be preserved
	if result["type"] != "kubernetes.io/tls" {
		t.Error("Secret type should be preserved")
	}

	// Metadata should be preserved
	meta := result["metadata"].(map[string]interface{})
	if meta["name"] != "test-secret" {
		t.Error("Secret name should be preserved")
	}
}

func TestMaskSecrets_DeepCopy(t *testing.T) {
	original := map[string]interface{}{
		"kind": "Secret",
		"data": map[string]interface{}{
			"password": "original-password",
		},
	}

	result := MaskSecrets(original)

	// Verify original is not modified
	if original["data"].(map[string]interface{})["password"] != "original-password" {
		t.Error("Original object was modified")
	}

	// Verify result is masked
	if result["data"].(map[string]interface{})["password"] != RedactedValue {
		t.Error("Result should be masked")
	}
}

func TestMaskSecretsInList(t *testing.T) {
	objects := []map[string]interface{}{
		{
			"kind": "Secret",
			"data": map[string]interface{}{
				"key1": "value1",
			},
		},
		{
			"kind": "Pod",
			"metadata": map[string]interface{}{
				"name": "test-pod",
			},
		},
		{
			"kind": "Secret",
			"data": map[string]interface{}{
				"key2": "value2",
			},
		},
	}

	result := MaskSecretsInList(objects)

	if len(result) != 3 {
		t.Errorf("Expected 3 results, got %d", len(result))
	}

	// First secret should be masked
	if result[0]["data"].(map[string]interface{})["key1"] != RedactedValue {
		t.Error("First secret should be masked")
	}

	// Pod should be unchanged (no data field)
	if result[1]["kind"] != "Pod" {
		t.Error("Pod kind should be preserved")
	}

	// Second secret should be masked
	if result[2]["data"].(map[string]interface{})["key2"] != RedactedValue {
		t.Error("Second secret should be masked")
	}
}

func TestMaskSecretsInList_Empty(t *testing.T) {
	result := MaskSecretsInList([]map[string]interface{}{})
	if len(result) != 0 {
		t.Error("Expected empty result for empty input")
	}

	result = MaskSecretsInList(nil)
	if result != nil {
		t.Error("Expected nil result for nil input")
	}
}

func TestIsSecretResource(t *testing.T) {
	tests := []struct {
		name string
		obj  map[string]interface{}
		want bool
	}{
		{
			name: "nil object",
			obj:  nil,
			want: false,
		},
		{
			name: "secret resource",
			obj:  map[string]interface{}{"kind": "Secret"},
			want: true,
		},
		{
			name: "lowercase secret",
			obj:  map[string]interface{}{"kind": "secret"},
			want: true,
		},
		{
			name: "uppercase SECRET",
			obj:  map[string]interface{}{"kind": "SECRET"},
			want: true,
		},
		{
			name: "pod resource",
			obj:  map[string]interface{}{"kind": "Pod"},
			want: false,
		},
		{
			name: "no kind field",
			obj:  map[string]interface{}{"name": "test"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSecretResource(tt.obj)
			if got != tt.want {
				t.Errorf("IsSecretResource() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsSensitiveSecretType(t *testing.T) {
	tests := []struct {
		secretType string
		want       bool
	}{
		{"Opaque", true},
		{"kubernetes.io/tls", true},
		{"kubernetes.io/service-account-token", true},
		{"kubernetes.io/dockerconfigjson", true},
		{"kubernetes.io/basic-auth", true},
		{"custom-type", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.secretType, func(t *testing.T) {
			got := IsSensitiveSecretType(tt.secretType)
			if got != tt.want {
				t.Errorf("IsSensitiveSecretType(%q) = %v, want %v", tt.secretType, got, tt.want)
			}
		})
	}
}

func TestMaskSecretSummary(t *testing.T) {
	secret := map[string]interface{}{
		"kind":       "Secret",
		"apiVersion": "v1",
		"metadata": map[string]interface{}{
			"name":              "test-secret",
			"namespace":         "default",
			"creationTimestamp": "2024-01-01T00:00:00Z",
			"labels": map[string]interface{}{
				"app": "test",
			},
		},
		"data": map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
		"type": "Opaque",
	}

	result := MaskSecretSummary(secret)

	// Should have kind and apiVersion
	if result["kind"] != "Secret" {
		t.Error("Should preserve kind")
	}
	if result["apiVersion"] != "v1" {
		t.Error("Should preserve apiVersion")
	}

	// Should have type
	if result["type"] != "Opaque" {
		t.Error("Should preserve type")
	}

	// Should have key count but not actual data
	if result["dataKeys"] != 2 {
		t.Errorf("dataKeys = %v, want 2", result["dataKeys"])
	}
	if result["data"] != nil {
		t.Error("Should not include actual data")
	}

	// Should have redacted marker
	if result["_dataRedacted"] != true {
		t.Error("Should have _dataRedacted marker")
	}

	// Should preserve limited metadata
	meta := result["metadata"].(map[string]interface{})
	if meta["name"] != "test-secret" {
		t.Error("Should preserve name in metadata")
	}
	if meta["namespace"] != "default" {
		t.Error("Should preserve namespace in metadata")
	}
}

func TestMaskSecretSummary_Nil(t *testing.T) {
	result := MaskSecretSummary(nil)
	if result != nil {
		t.Error("Expected nil result for nil input")
	}
}

func TestContainsSensitiveData(t *testing.T) {
	tests := []struct {
		name string
		obj  map[string]interface{}
		want bool
	}{
		{
			name: "nil object",
			obj:  nil,
			want: false,
		},
		{
			name: "secret",
			obj:  map[string]interface{}{"kind": "Secret"},
			want: true,
		},
		{
			name: "service account",
			obj:  map[string]interface{}{"kind": "ServiceAccount"},
			want: true,
		},
		{
			name: "credentials configmap",
			obj: map[string]interface{}{
				"kind": "ConfigMap",
				"metadata": map[string]interface{}{
					"name": "my-credentials",
				},
			},
			want: true,
		},
		{
			name: "password configmap",
			obj: map[string]interface{}{
				"kind": "ConfigMap",
				"metadata": map[string]interface{}{
					"name": "app-password-config",
				},
			},
			want: true,
		},
		{
			name: "regular configmap",
			obj: map[string]interface{}{
				"kind": "ConfigMap",
				"metadata": map[string]interface{}{
					"name": "app-config",
				},
			},
			want: false,
		},
		{
			name: "regular pod",
			obj:  map[string]interface{}{"kind": "Pod"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContainsSensitiveData(tt.obj)
			if got != tt.want {
				t.Errorf("ContainsSensitiveData() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRedactedValue(t *testing.T) {
	if RedactedValue != "***REDACTED***" {
		t.Errorf("RedactedValue = %q, want %q", RedactedValue, "***REDACTED***")
	}
}
