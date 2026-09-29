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

// lastAppliedAnnotation carries the full manifest of an object applied with
// kubectl apply, data included.
const lastAppliedAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

// MaskSecrets replaces the data of a Secret, and of a ConfigMap whose name
// flags it as sensitive, with redacted placeholders. Masking is not
// configurable: the server is the only path from a cluster to an agent, so no
// tool may return Secret data.
func MaskSecrets(obj map[string]interface{}) map[string]interface{} {
	if obj == nil {
		return nil
	}

	// Create a deep copy to avoid modifying the original
	result := deepCopyMap(obj)

	switch {
	case IsSecretResource(result):
		redactValues(result, "data", "stringData")
		maskSensitiveAnnotations(result, true)
	case isSensitiveConfigMap(result):
		redactValues(result, "data", "binaryData")
		maskSensitiveAnnotations(result, false)
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

// redactValues replaces every value of the named map fields, keeping the keys
// visible for context.
func redactValues(obj map[string]interface{}, fields ...string) {
	for _, field := range fields {
		values, ok := obj[field].(map[string]interface{})
		if !ok {
			continue
		}
		masked := make(map[string]interface{}, len(values))
		for key := range values {
			masked[key] = RedactedValue
		}
		obj[field] = masked
	}
}

// maskSensitiveAnnotations masks the last-applied configuration, which
// repeats the object's data, and for a Secret the known sensitive annotations.
func maskSensitiveAnnotations(obj map[string]interface{}, secret bool) {
	metadata, ok := obj["metadata"].(map[string]interface{})
	if !ok {
		return
	}

	annotations, ok := metadata["annotations"].(map[string]interface{})
	if !ok {
		return
	}

	for key := range annotations {
		if key == lastAppliedAnnotation || (secret && sensitiveAnnotations[key]) {
			annotations[key] = RedactedValue
		}
	}
}

// isSensitiveConfigMap reports whether obj is a ConfigMap whose name matches
// a sensitive pattern.
func isSensitiveConfigMap(obj map[string]interface{}) bool {
	kind, _ := obj["kind"].(string)
	return strings.EqualFold(kind, "ConfigMap") && ContainsSensitiveData(obj)
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
