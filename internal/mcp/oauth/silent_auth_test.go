package oauth

import (
	"errors"
	"fmt"
	"testing"
)

func TestIsSilentAuthError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "login_required SilentAuthError",
			err:      &SilentAuthError{Code: ErrorCodeLoginRequired},
			expected: true,
		},
		{
			name:     "consent_required SilentAuthError",
			err:      &SilentAuthError{Code: ErrorCodeConsentRequired},
			expected: true,
		},
		{
			name:     "interaction_required SilentAuthError",
			err:      &SilentAuthError{Code: ErrorCodeInteractionRequired},
			expected: true,
		},
		{
			name:     "account_selection_required SilentAuthError",
			err:      &SilentAuthError{Code: ErrorCodeAccountSelectionRequired},
			expected: true,
		},
		{
			name:     "wrapped SilentAuthError",
			err:      fmt.Errorf("wrapped: %w", &SilentAuthError{Code: ErrorCodeLoginRequired}),
			expected: true,
		},
		{
			name:     "ErrSilentAuthFailed sentinel",
			err:      ErrSilentAuthFailed,
			expected: true,
		},
		{
			name:     "wrapped ErrSilentAuthFailed",
			err:      fmt.Errorf("wrapped: %w", ErrSilentAuthFailed),
			expected: true,
		},
		{
			name:     "other OAuth error - invalid_grant",
			err:      fmt.Errorf("oauth error: invalid_grant"),
			expected: false,
		},
		{
			name:     "generic error",
			err:      fmt.Errorf("something went wrong"),
			expected: false,
		},
		{
			name:     "error string containing login_required",
			err:      fmt.Errorf("error: login_required"),
			expected: true,
		},
		{
			name:     "error string containing consent_required",
			err:      fmt.Errorf("error: consent_required"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSilentAuthError(tt.err); got != tt.expected {
				t.Errorf("IsSilentAuthError() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParseOAuthError(t *testing.T) {
	tests := []struct {
		name             string
		errorCode        string
		errorDescription string
		wantNil          bool
		wantSilentAuth   bool
	}{
		{
			name:      "empty error code returns nil",
			errorCode: "",
			wantNil:   true,
		},
		{
			name:             "login_required returns SilentAuthError",
			errorCode:        ErrorCodeLoginRequired,
			errorDescription: "User must authenticate",
			wantSilentAuth:   true,
		},
		{
			name:           "consent_required returns SilentAuthError",
			errorCode:      ErrorCodeConsentRequired,
			wantSilentAuth: true,
		},
		{
			name:             "interaction_required returns SilentAuthError",
			errorCode:        ErrorCodeInteractionRequired,
			errorDescription: "UI required",
			wantSilentAuth:   true,
		},
		{
			name:           "account_selection_required returns SilentAuthError",
			errorCode:      ErrorCodeAccountSelectionRequired,
			wantSilentAuth: true,
		},
		{
			name:             "invalid_grant returns generic error",
			errorCode:        "invalid_grant",
			errorDescription: "Token expired",
			wantSilentAuth:   false,
		},
		{
			name:           "access_denied returns generic error",
			errorCode:      "access_denied",
			wantSilentAuth: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ParseOAuthError(tt.errorCode, tt.errorDescription)

			if tt.wantNil {
				if err != nil {
					t.Errorf("ParseOAuthError() = %v, want nil", err)
				}
				return
			}

			if err == nil {
				t.Fatal("ParseOAuthError() = nil, want error")
			}

			if tt.wantSilentAuth {
				var silentErr *SilentAuthError
				if !errors.As(err, &silentErr) {
					t.Errorf("ParseOAuthError() should return *SilentAuthError")
				}
				if silentErr.Code != tt.errorCode {
					t.Errorf("SilentAuthError.Code = %q, want %q", silentErr.Code, tt.errorCode)
				}
				if !IsSilentAuthError(err) {
					t.Errorf("IsSilentAuthError() should return true for parsed error")
				}
			} else {
				var silentErr *SilentAuthError
				if errors.As(err, &silentErr) {
					t.Errorf("ParseOAuthError() should not return *SilentAuthError for %q", tt.errorCode)
				}
			}
		})
	}
}

func TestParseCallbackQuery(t *testing.T) {
	tests := []struct {
		name             string
		code             string
		state            string
		errorCode        string
		errorDescription string
		errorURI         string
		wantIsError      bool
		wantSilentAuth   bool
	}{
		{
			name:        "successful callback",
			code:        "auth_code_123",
			state:       "state_456",
			wantIsError: false,
		},
		{
			name:             "login_required error",
			state:            "state_456",
			errorCode:        ErrorCodeLoginRequired,
			errorDescription: "User must authenticate",
			wantIsError:      true,
			wantSilentAuth:   true,
		},
		{
			name:             "access_denied error",
			state:            "state_456",
			errorCode:        "access_denied",
			errorDescription: "User denied the request",
			wantIsError:      true,
			wantSilentAuth:   false,
		},
		{
			name:             "error callback with all fields",
			state:            "state_456",
			errorCode:        ErrorCodeConsentRequired,
			errorDescription: "Consent required",
			errorURI:         "https://docs.example.com/errors",
			wantIsError:      true,
			wantSilentAuth:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseCallbackQuery(tt.code, tt.state, tt.errorCode, tt.errorDescription, tt.errorURI)

			if result.Code != tt.code {
				t.Errorf("Code = %q, want %q", result.Code, tt.code)
			}
			if result.State != tt.state {
				t.Errorf("State = %q, want %q", result.State, tt.state)
			}
			if result.Error != tt.errorCode {
				t.Errorf("Error = %q, want %q", result.Error, tt.errorCode)
			}
			if result.ErrorDescription != tt.errorDescription {
				t.Errorf("ErrorDescription = %q, want %q", result.ErrorDescription, tt.errorDescription)
			}
			if result.ErrorURI != tt.errorURI {
				t.Errorf("ErrorURI = %q, want %q", result.ErrorURI, tt.errorURI)
			}

			if result.IsError() != tt.wantIsError {
				t.Errorf("IsError() = %v, want %v", result.IsError(), tt.wantIsError)
			}

			if tt.wantIsError {
				err := result.Err()
				if err == nil {
					t.Error("Err() returned nil for error callback")
				} else if IsSilentAuthError(err) != tt.wantSilentAuth {
					t.Errorf("IsSilentAuthError(Err()) = %v, want %v", IsSilentAuthError(err), tt.wantSilentAuth)
				}
			} else {
				if err := result.Err(); err != nil {
					t.Errorf("Err() = %v, want nil for successful callback", err)
				}
			}
		})
	}
}

func TestSilentAuthErrorConstants(t *testing.T) {
	// Verify the constants are correctly defined
	tests := []struct {
		name     string
		code     string
		expected string
	}{
		{"login_required", ErrorCodeLoginRequired, "login_required"},
		{"consent_required", ErrorCodeConsentRequired, "consent_required"},
		{"interaction_required", ErrorCodeInteractionRequired, "interaction_required"},
		{"account_selection_required", ErrorCodeAccountSelectionRequired, "account_selection_required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.code != tt.expected {
				t.Errorf("constant %s = %q, want %q", tt.name, tt.code, tt.expected)
			}
		})
	}
}

func TestAuthorizationURLOptions(t *testing.T) {
	t.Run("zero value is usable", func(t *testing.T) {
		opts := AuthorizationURLOptions{}
		if opts.Prompt != "" {
			t.Error("zero value Prompt should be empty")
		}
		if opts.LoginHint != "" {
			t.Error("zero value LoginHint should be empty")
		}
		if opts.MaxAge != nil {
			t.Error("zero value MaxAge should be nil")
		}
	})

	t.Run("all fields can be set", func(t *testing.T) {
		maxAge := 3600
		opts := AuthorizationURLOptions{
			Prompt:      "none",
			LoginHint:   "user@example.com",
			MaxAge:      &maxAge,
			ACRValues:   "urn:mace:incommon:iap:silver",
			IDTokenHint: "eyJhbGciOiJSUzI1NiJ9...",
			Extra: map[string]string{
				"hd": "example.com",
			},
		}

		if opts.Prompt != "none" {
			t.Errorf("Prompt = %q, want %q", opts.Prompt, "none")
		}
		if opts.LoginHint != "user@example.com" {
			t.Errorf("LoginHint = %q, want %q", opts.LoginHint, "user@example.com")
		}
		if *opts.MaxAge != 3600 {
			t.Errorf("MaxAge = %d, want %d", *opts.MaxAge, 3600)
		}
		if opts.ACRValues != "urn:mace:incommon:iap:silver" {
			t.Errorf("ACRValues = %q, want %q", opts.ACRValues, "urn:mace:incommon:iap:silver")
		}
		if opts.Extra["hd"] != "example.com" {
			t.Errorf("Extra[hd] = %q, want %q", opts.Extra["hd"], "example.com")
		}
	})
}

func TestSilentAuthWorkflow(t *testing.T) {
	// This test demonstrates the typical silent auth workflow:
	// 1. Client sends authorization request with prompt=none
	// 2. IdP returns error because user session doesn't exist
	// 3. Client detects silent auth failure and falls back to interactive login

	// Simulate IdP returning login_required error
	result := ParseCallbackQuery(
		"",           // No code because auth failed
		"csrf_state", // State preserved
		"login_required",
		"The user is not logged in",
		"",
	)

	// Check if it's an error
	if !result.IsError() {
		t.Fatal("Expected callback to be an error")
	}

	// Get the typed error
	err := result.Err()
	if err == nil {
		t.Fatal("Expected non-nil error")
	}

	// Detect that it's a silent auth error
	if !IsSilentAuthError(err) {
		t.Error("Expected IsSilentAuthError to return true")
	}

	// Extract the SilentAuthError details
	var silentErr *SilentAuthError
	if !errors.As(err, &silentErr) {
		t.Fatal("Expected error to be *SilentAuthError")
	}

	if silentErr.Code != "login_required" {
		t.Errorf("SilentAuthError.Code = %q, want %q", silentErr.Code, "login_required")
	}
	if silentErr.Description != "The user is not logged in" {
		t.Errorf("SilentAuthError.Description = %q, want %q", silentErr.Description, "The user is not logged in")
	}
}
