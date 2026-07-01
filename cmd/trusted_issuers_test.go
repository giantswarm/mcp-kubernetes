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
			name: "issuer and jwksURL only is valid",
			issuers: []server.TrustedIssuerConfig{{
				Issuer:  issuerURL,
				JwksURL: jwksURL,
			}},
		},
		{
			name: "sub-keyed issuer is valid",
			issuers: []server.TrustedIssuerConfig{{
				Issuer:        issuerURL,
				JwksURL:       jwksURL,
				AllowedClaims: map[string]string{"sub": "system:serviceaccount:kagent:*"},
			}},
		},
		{
			name: "email-remapped issuer is valid",
			issuers: []server.TrustedIssuerConfig{{
				Issuer:        issuerURL,
				JwksURL:       jwksURL,
				SubjectClaim:  "email",
				AllowedClaims: map[string]string{"email": "*@giantswarm.io"},
			}},
		},
		{
			name: "SubjectClaim set without matching allowedClaims key is valid",
			issuers: []server.TrustedIssuerConfig{{
				Issuer:        issuerURL,
				JwksURL:       jwksURL,
				SubjectClaim:  "email",
				AllowedClaims: map[string]string{"sub": "*@giantswarm.io"},
			}},
		},
		{
			name: "no allowedClaims is valid",
			issuers: []server.TrustedIssuerConfig{{
				Issuer:  issuerURL,
				JwksURL: jwksURL,
			}},
		},
		{
			name: "bare wildcard in allowedClaims is rejected",
			issuers: []server.TrustedIssuerConfig{{
				Issuer:        issuerURL,
				JwksURL:       jwksURL,
				AllowedClaims: map[string]string{"sub": "*"},
			}},
			wantError: true,
		},
		{
			name: "bare wildcard on non-subject claim is rejected",
			issuers: []server.TrustedIssuerConfig{{
				Issuer:        issuerURL,
				JwksURL:       jwksURL,
				AllowedClaims: map[string]string{"email": "*"},
			}},
			wantError: true,
		},
		{
			name: "duplicate issuer URL is rejected",
			issuers: []server.TrustedIssuerConfig{
				{Issuer: issuerURL, JwksURL: jwksURL, AllowedClaims: map[string]string{"sub": "system:serviceaccount:kagent:*"}},
				{Issuer: issuerURL, JwksURL: jwksURL, SubjectClaim: "email", AllowedClaims: map[string]string{"email": "*@giantswarm.io"}},
			},
			wantError: true,
		},
		{
			name: "same issuer URL with one passthrough entry (no allowedClaims) is valid",
			issuers: []server.TrustedIssuerConfig{
				{Issuer: issuerURL, JwksURL: jwksURL},
			},
		},
		{
			name: "empty issuer URL is rejected",
			issuers: []server.TrustedIssuerConfig{{
				JwksURL: jwksURL,
			}},
			wantError: true,
		},
		{
			name: "empty jwksURL is rejected",
			issuers: []server.TrustedIssuerConfig{{
				Issuer: issuerURL,
			}},
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
