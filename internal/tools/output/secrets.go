package output

import (
	"strings"
)

// RedactedValue is the placeholder used for masked secret data.
const RedactedValue = "***REDACTED***"

// secretTypes lists Kubernetes secret types that should always be masked.
var secretTypes = map[string]bool{
	"kubernetes.io/service-account-token": true,
	"kubernetes.io/dockercfg":             true,
	"kubernetes.io/dockerconfigjson":      true,
	"kubernetes.io/basic-auth":            true,
	"kubernetes.io/ssh-auth":              true,
	"kubernetes.io/tls":                   true,
	"bootstrap.kubernetes.io/token":       true,
	"helm.sh/release.v1":                  true, // Helm release secrets
	"Opaque":                              true, // Generic secrets
}

// sensitiveAnnotations lists annotations that contain sensitive data.
var sensitiveAnnotations = map[string]bool{
	"kubernetes.io/service-account.uid":   true,
	"kubernetes.io/service-account.name":  true,
	"kubernetes.io/service-account-token": true,
}

// sensitiveConfigMapPatterns are name patterns that indicate a ConfigMap may contain secrets.
var sensitiveConfigMapPatterns = []string{
	"credentials",
	"password",
	"secret",
	"auth",
	"token",
	"kubeconfig",
}

// MaskSecrets replaces secret data with redacted placeholders.
// This prevents accidental exposure of sensitive data in tool responses.
func MaskSecrets(obj map[string]interface{}) map[string]interface{} {
	if obj == nil {
		return nil
	}

	// Create a deep copy to avoid modifying the original
	result := deepCopyMap(obj)

	// Check if this is a Secret resource
	kind, _ := result["kind"].(string)
	if strings.EqualFold(kind, "Secret") {
		maskSecretData(result)
	}

	return result
}

// MaskSecretsInList masks secrets in a list of resources.
func MaskSecretsInList(objects []map[string]interface{}) []map[string]interface{} {
	if len(objects) == 0 {
		return objects
	}

	result := make([]map[string]interface{}, len(objects))
	for i, obj := range objects {
		result[i] = MaskSecrets(obj)
	}

	return result
}

// maskSecretData masks the data and stringData fields of a Secret.
func maskSecretData(secret map[string]interface{}) {
	// Mask data field (base64 encoded values)
	if data, ok := secret["data"].(map[string]interface{}); ok {
		maskedData := make(map[string]interface{}, len(data))
		for key := range data {
			maskedData[key] = RedactedValue
		}
		secret["data"] = maskedData
	}

	// Mask stringData field (plain text values)
	if stringData, ok := secret["stringData"].(map[string]interface{}); ok {
		maskedStringData := make(map[string]interface{}, len(stringData))
		for key := range stringData {
			maskedStringData[key] = RedactedValue
		}
		secret["stringData"] = maskedStringData
	}

	// Keep type field visible for context (e.g., kubernetes.io/tls)
	// but mask sensitive annotations
	maskSensitiveAnnotations(secret)
}

// maskSensitiveAnnotations masks known sensitive annotations.
func maskSensitiveAnnotations(obj map[string]interface{}) {
	metadata, ok := obj["metadata"].(map[string]interface{})
	if !ok {
		return
	}

	annotations, ok := metadata["annotations"].(map[string]interface{})
	if !ok {
		return
	}

	for key := range annotations {
		if sensitiveAnnotations[key] {
			annotations[key] = RedactedValue
		}
	}
}

// IsSecretResource checks if a resource is a Kubernetes Secret.
func IsSecretResource(obj map[string]interface{}) bool {
	if obj == nil {
		return false
	}

	kind, _ := obj["kind"].(string)
	return strings.EqualFold(kind, "Secret")
}

// IsSensitiveSecretType checks if a secret type contains sensitive data.
// Returns true for types that should always have their data masked.
func IsSensitiveSecretType(secretType string) bool {
	return secretTypes[secretType]
}

// MaskSecretSummary creates a summary of a Secret without exposing data.
// Useful for listing secrets with basic info but no sensitive content.
func MaskSecretSummary(secret map[string]interface{}) map[string]interface{} {
	if secret == nil {
		return nil
	}

	// Create a minimal summary
	result := make(map[string]interface{})

	// Copy basic metadata
	if kind, ok := secret["kind"].(string); ok {
		result["kind"] = kind
	}
	if apiVersion, ok := secret["apiVersion"].(string); ok {
		result["apiVersion"] = apiVersion
	}

	// Copy metadata (selectively)
	if metadata, ok := secret["metadata"].(map[string]interface{}); ok {
		metaCopy := make(map[string]interface{})
		if name, ok := metadata["name"].(string); ok {
			metaCopy["name"] = name
		}
		if namespace, ok := metadata["namespace"].(string); ok {
			metaCopy["namespace"] = namespace
		}
		if creationTimestamp, ok := metadata["creationTimestamp"].(string); ok {
			metaCopy["creationTimestamp"] = creationTimestamp
		}
		// Copy labels (useful for filtering)
		if labels, ok := metadata["labels"].(map[string]interface{}); ok {
			metaCopy["labels"] = labels
		}
		result["metadata"] = metaCopy
	}

	// Add type but mask data
	if secretType, ok := secret["type"].(string); ok {
		result["type"] = secretType
	}

	// Add key count without actual keys or values
	if data, ok := secret["data"].(map[string]interface{}); ok {
		result["dataKeys"] = len(data)
	}

	// Add a marker that data was redacted
	result["_dataRedacted"] = true

	return result
}

// ContainsSensitiveData checks if a resource might contain sensitive data.
// This includes Secrets, ConfigMaps with certain names, and ServiceAccounts.
func ContainsSensitiveData(obj map[string]interface{}) bool {
	if obj == nil {
		return false
	}

	kind, _ := obj["kind"].(string)
	kind = strings.ToLower(kind)

	switch kind {
	case "secret":
		return true
	case "configmap":
		// Some ConfigMaps contain sensitive data based on naming patterns
		metadata, ok := obj["metadata"].(map[string]interface{})
		if !ok {
			return false
		}
		name, ok := metadata["name"].(string)
		if !ok {
			return false
		}
		name = strings.ToLower(name)
		for _, pattern := range sensitiveConfigMapPatterns {
			if strings.Contains(name, pattern) {
				return true
			}
		}
		return false
	case "serviceaccount":
		// ServiceAccounts can contain token references
		return true
	}

	return false
}
