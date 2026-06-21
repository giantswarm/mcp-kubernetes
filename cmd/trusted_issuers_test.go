package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/giantswarm/mcp-kubernetes/internal/server"
)

func TestValidateTrustedIssuers(t *testing.T) {
	const (
		issuerURL = "https://oidc.example.com"
		jwksURL   = "https://oidc.example.com/.well-known/jwks.json"
	)

	tests := []struct {
		name      string
		issuers   []server.TrustedIssuerConfig
		wantError bool
	}{
		{
			name: "sub-keyed issuer is valid",
			issuers: []server.TrustedIssuerConfig{{
				Issuer:        issuerURL,
				JwksURL:       jwksURL,
				Alias:         "glean",
				AllowedClaims: map[string]string{"sub": "system:serviceaccount:kagent:*"},
			}},
		},
		{
			name: "email-remapped issuer gates on allowedClaims.email",
			issuers: []server.TrustedIssuerConfig{{
				Issuer:        issuerURL,
				JwksURL:       jwksURL,
				Alias:         "muster-obo",
				SubjectClaim:  "email",
				AllowedClaims: map[string]string{"email": "*@giantswarm.io"},
			}},
		},
		{
			name: "SubjectClaim set but no pattern under that key is rejected",
			issuers: []server.TrustedIssuerConfig{{
				Issuer:        issuerURL,
				JwksURL:       jwksURL,
				Alias:         "muster-obo",
				SubjectClaim:  "email",
				AllowedClaims: map[string]string{"sub": "*@giantswarm.io"},
			}},
			wantError: true,
		},
		{
			name: "bare wildcard under the effective key is rejected",
			issuers: []server.TrustedIssuerConfig{{
				Issuer:        issuerURL,
				JwksURL:       jwksURL,
				Alias:         "muster-obo",
				SubjectClaim:  "email",
				AllowedClaims: map[string]string{"email": "*"},
			}},
			wantError: true,
		},
		{
			name: "missing subject pattern on default sub key is rejected",
			issuers: []server.TrustedIssuerConfig{{
				Issuer:  issuerURL,
				JwksURL: jwksURL,
				Alias:   "glean",
			}},
			wantError: true,
		},
		{
			name: "duplicate alias is rejected",
			issuers: []server.TrustedIssuerConfig{
				{Issuer: issuerURL, JwksURL: jwksURL, Alias: "glean", AllowedClaims: map[string]string{"sub": "a"}},
				{Issuer: "https://other.example.com", JwksURL: jwksURL, Alias: "glean", AllowedClaims: map[string]string{"sub": "b"}},
			},
			wantError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTrustedIssuers(tc.issuers)
			if tc.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}
