package federation

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateUserInfo(t *testing.T) {
	tests := []struct {
		name          string
		user          *UserInfo
		expectedError error
		errorContains string
	}{
		{
			name:          "nil user returns ErrUserInfoRequired",
			user:          nil,
			expectedError: ErrUserInfoRequired,
		},
		{
			name: "valid user with email and groups",
			user: &UserInfo{
				Email:  "user@example.com",
				Groups: []string{"developers", "team-alpha"},
			},
			expectedError: nil,
		},
		{
			name: "valid user with empty email",
			user: &UserInfo{
				Email:  "",
				Groups: []string{"developers"},
			},
			expectedError: nil,
		},
		{
			name: "valid user with extra headers",
			user: &UserInfo{
				Email:  "user@example.com",
				Groups: []string{"developers"},
				Extra: map[string][]string{
					"organization": {"acme-corp"},
					"tenant_id":    {"tenant-123"},
				},
			},
			expectedError: nil,
		},
		{
			name: "email too long",
			user: &UserInfo{
				Email: strings.Repeat("a", 300) + "@example.com",
			},
			expectedError: ErrInvalidEmail,
			errorContains: "email too long",
		},
		{
			name: "email with control characters",
			user: &UserInfo{
				Email: "user\x00@example.com",
			},
			expectedError: ErrInvalidEmail,
			errorContains: "control characters",
		},
		{
			name: "invalid email format",
			user: &UserInfo{
				Email: "not-an-email",
			},
			expectedError: ErrInvalidEmail,
			errorContains: "email format is invalid",
		},
		{
			name: "too many groups",
			user: &UserInfo{
				Email:  "user@example.com",
				Groups: make([]string, MaxGroupCount+1),
			},
			expectedError: ErrInvalidGroupName,
			errorContains: "too many groups",
		},
		{
			name: "empty group name",
			user: &UserInfo{
				Email:  "user@example.com",
				Groups: []string{"valid", ""},
			},
			expectedError: ErrInvalidGroupName,
			errorContains: "cannot be empty",
		},
		{
			name: "group name too long",
			user: &UserInfo{
				Email:  "user@example.com",
				Groups: []string{strings.Repeat("a", MaxGroupNameLength+1)},
			},
			expectedError: ErrInvalidGroupName,
			errorContains: "too long",
		},
		{
			name: "group with control characters",
			user: &UserInfo{
				Email:  "user@example.com",
				Groups: []string{"valid", "invalid\x00group"},
			},
			expectedError: ErrInvalidGroupName,
			errorContains: "control characters",
		},
		{
			name: "too many extra headers",
			user: func() *UserInfo {
				extra := make(map[string][]string, MaxExtraCount+1)
				for i := 0; i <= MaxExtraCount; i++ {
					extra[fmt.Sprintf("key%d", i)] = []string{"value"}
				}
				return &UserInfo{
					Email: "user@example.com",
					Extra: extra,
				}
			}(),
			expectedError: ErrInvalidExtraHeader,
			errorContains: "too many extra headers",
		},
		{
			name: "empty extra header key",
			user: &UserInfo{
				Email: "user@example.com",
				Extra: map[string][]string{
					"": {"value"},
				},
			},
			expectedError: ErrInvalidExtraHeader,
			errorContains: "cannot be empty",
		},
		{
			name: "extra header key too long",
			user: &UserInfo{
				Email: "user@example.com",
				Extra: map[string][]string{
					strings.Repeat("a", MaxExtraKeyLength+1): {"value"},
				},
			},
			expectedError: ErrInvalidExtraHeader,
			errorContains: "too long",
		},
		{
			name: "extra header key with invalid characters",
			user: &UserInfo{
				Email: "user@example.com",
				Extra: map[string][]string{
					"invalid/key": {"value"},
				},
			},
			expectedError: ErrInvalidExtraHeader,
			errorContains: "invalid characters",
		},
		{
			name: "extra header value too long",
			user: &UserInfo{
				Email: "user@example.com",
				Extra: map[string][]string{
					"valid-key": {strings.Repeat("a", MaxExtraValueLength+1)},
				},
			},
			expectedError: ErrInvalidExtraHeader,
			errorContains: "too long",
		},
		{
			name: "extra header value with control characters",
			user: &UserInfo{
				Email: "user@example.com",
				Extra: map[string][]string{
					"valid-key": {"invalid\x00value"},
				},
			},
			expectedError: ErrInvalidExtraHeader,
			errorContains: "control characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUserInfo(tt.user)

			if tt.expectedError != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.expectedError), "expected error %v, got %v", tt.expectedError, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateClusterName(t *testing.T) {
	tests := []struct {
		name          string
		clusterName   string
		expectedError error
		errorContains string
	}{
		{
			name:          "valid cluster name",
			clusterName:   "my-cluster",
			expectedError: nil,
		},
		{
			name:          "valid cluster name with numbers",
			clusterName:   "cluster-123",
			expectedError: nil,
		},
		{
			name:          "valid single character",
			clusterName:   "a",
			expectedError: nil,
		},
		{
			name:          "valid all lowercase",
			clusterName:   "mycluster",
			expectedError: nil,
		},
		{
			name:          "empty cluster name",
			clusterName:   "",
			expectedError: ErrInvalidClusterName,
			errorContains: "cannot be empty",
		},
		{
			name:          "cluster name too long",
			clusterName:   strings.Repeat("a", MaxClusterNameLength+1),
			expectedError: ErrInvalidClusterName,
			errorContains: "too long",
		},
		{
			name:          "cluster name with uppercase",
			clusterName:   "My-Cluster",
			expectedError: ErrInvalidClusterName,
			errorContains: "lowercase",
		},
		{
			name:          "cluster name starting with hyphen",
			clusterName:   "-my-cluster",
			expectedError: ErrInvalidClusterName,
			errorContains: "start with alphanumeric",
		},
		{
			name:          "cluster name ending with hyphen",
			clusterName:   "my-cluster-",
			expectedError: ErrInvalidClusterName,
			errorContains: "end with alphanumeric",
		},
		{
			name:          "cluster name with path traversal",
			clusterName:   "../etc/passwd",
			expectedError: ErrInvalidClusterName,
			errorContains: "invalid path characters",
		},
		{
			name:          "cluster name with forward slash",
			clusterName:   "my/cluster",
			expectedError: ErrInvalidClusterName,
			errorContains: "invalid path characters",
		},
		{
			name:          "cluster name with backslash",
			clusterName:   "my\\cluster",
			expectedError: ErrInvalidClusterName,
			errorContains: "invalid path characters",
		},
		{
			name:          "cluster name with underscore",
			clusterName:   "my_cluster",
			expectedError: ErrInvalidClusterName,
			errorContains: "lowercase alphanumeric",
		},
		{
			name:          "cluster name with spaces",
			clusterName:   "my cluster",
			expectedError: ErrInvalidClusterName,
			errorContains: "lowercase alphanumeric",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateClusterName(tt.clusterName)

			if tt.expectedError != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.expectedError), "expected error %v, got %v", tt.expectedError, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationError(t *testing.T) {
	t.Run("error with value", func(t *testing.T) {
		err := &ValidationError{
			Field:  "email",
			Value:  "test@example.com",
			Reason: "too long",
			Err:    ErrInvalidEmail,
		}

		assert.Contains(t, err.Error(), "email")
		assert.Contains(t, err.Error(), "test@example.com")
		assert.Contains(t, err.Error(), "too long")
		assert.True(t, errors.Is(err, ErrInvalidEmail))
	})

	t.Run("error without value", func(t *testing.T) {
		err := &ValidationError{
			Field:  "groups",
			Reason: "too many groups",
			Err:    ErrInvalidGroupName,
		}

		assert.Contains(t, err.Error(), "groups")
		assert.Contains(t, err.Error(), "too many groups")
		assert.True(t, errors.Is(err, ErrInvalidGroupName))
	})

	t.Run("user facing error", func(t *testing.T) {
		err := &ValidationError{
			Field:  "email",
			Value:  "sensitive@email.com",
			Reason: "format is invalid",
			Err:    ErrInvalidEmail,
		}

		userFacing := err.UserFacingError()
		assert.NotContains(t, userFacing, "sensitive@email.com")
		assert.Contains(t, userFacing, "email")
	})
}

func TestAnonymizeEmail(t *testing.T) {
	tests := []struct {
		name     string
		email    string
		expected string
	}{
		{
			name:     "empty email returns empty string",
			email:    "",
			expected: "",
		},
		{
			name:  "email is hashed",
			email: "user@example.com",
		},
		{
			name:  "different emails produce different hashes",
			email: "other@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AnonymizeEmail(tt.email)

			if tt.expected != "" {
				assert.Equal(t, tt.expected, result)
			} else if tt.email != "" {
				// Verify format and that email is not in result
				assert.True(t, strings.HasPrefix(result, "user:"), "hash should start with 'user:'")
				assert.NotContains(t, result, tt.email)
				assert.Len(t, result, 5+16) // "user:" + 16 hex characters

				// Verify deterministic
				assert.Equal(t, result, AnonymizeEmail(tt.email))
			}
		})
	}

	// Verify different emails produce different hashes
	hash1 := AnonymizeEmail("user1@example.com")
	hash2 := AnonymizeEmail("user2@example.com")
	assert.NotEqual(t, hash1, hash2)
}

func TestAnonymizeUserInfo(t *testing.T) {
	t.Run("nil user", func(t *testing.T) {
		result := AnonymizeUserInfo(nil)

		assert.Equal(t, "", result["user_hash"])
		assert.Equal(t, 0, result["group_count"])
	})

	t.Run("user with data", func(t *testing.T) {
		user := &UserInfo{
			Email:  "user@example.com",
			Groups: []string{"group1", "group2", "group3"},
		}

		result := AnonymizeUserInfo(user)

		assert.NotEmpty(t, result["user_hash"])
		assert.NotContains(t, result["user_hash"], "user@example.com")
		assert.Equal(t, 3, result["group_count"])
	})
}

func TestContainsControlCharacters(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "normal string",
			input:    "hello world",
			expected: false,
		},
		{
			name:     "string with null byte",
			input:    "hello\x00world",
			expected: true,
		},
		{
			name:     "string with newline",
			input:    "hello\nworld",
			expected: true,
		},
		{
			name:     "string with tab",
			input:    "hello\tworld",
			expected: true,
		},
		{
			name:     "string with carriage return",
			input:    "hello\rworld",
			expected: true,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "unicode string",
			input:    "hello \u4e16\u754c",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsControlCharacters(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTruncateForError(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "short string unchanged",
			input:    "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "exact length unchanged",
			input:    "hello",
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "long string truncated",
			input:    "hello world",
			maxLen:   5,
			expected: "hello...",
		},
		{
			name:     "empty string",
			input:    "",
			maxLen:   10,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateForError(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidEmailFormats(t *testing.T) {
	validEmails := []string{
		"simple@example.com",
		"user.name@example.com",
		"user+tag@example.com",
		"user@subdomain.example.com",
		"123@example.com",
		"user@example.co.uk",
	}

	for _, email := range validEmails {
		t.Run(email, func(t *testing.T) {
			user := &UserInfo{Email: email}
			err := ValidateUserInfo(user)
			assert.NoError(t, err, "expected %s to be valid", email)
		})
	}
}

func TestInvalidEmailFormats(t *testing.T) {
	invalidEmails := []string{
		"not-an-email",
		"@example.com",
		"user@",
		"user@.com",
		"user name@example.com",
	}

	for _, email := range invalidEmails {
		t.Run(email, func(t *testing.T) {
			user := &UserInfo{Email: email}
			err := ValidateUserInfo(user)
			assert.Error(t, err, "expected %s to be invalid", email)
			assert.True(t, errors.Is(err, ErrInvalidEmail))
		})
	}
}
