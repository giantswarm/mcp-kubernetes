package server

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/giantswarm/mcp-oauth/handler"
	"github.com/giantswarm/mcp-oauth/providers/oidc"
	"github.com/giantswarm/mcp-oauth/security"
	oauthserver "github.com/giantswarm/mcp-oauth/server"
)

// authenticateForwardedBearer restores the operator-configured TrustedAudiences
// contract for upstream ID tokens. mcp-oauth's resource-bearer middleware no
// longer tries this path. Its acceptance helper still verifies issuer,
// audience, signature and lifetime using the configured provider and JWKS CA.
func (s *OAuthHTTPServer) authenticateForwardedBearer(next http.Handler) http.Handler {
	validated := s.oauthHandler.ValidateToken(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bearer, ok := extractBearerToken(r)
		if !ok || len(s.oauthServer.Config.TrustedAudiences) == 0 || !unboundForwardedBearer(bearer) {
			validated.ServeHTTP(w, r)
			return
		}
		// This path skips ValidateToken, so enforce its IP and user limits here.
		// Rejected bearers fall back to its normal responses and security audit.
		if limiter := s.oauthServer.RateLimiter; limiter != nil {
			ip := security.GetClientIP(r, s.oauthServer.Config.TrustProxy, s.oauthServer.Config.TrustedProxyCount)
			if !limiter.Allow(security.RateLimitBucket(ip)) {
				forwardedRateLimit(w)
				return
			}
		}
		acceptance, err := s.oauthServer.AcceptForwardedIDToken(r.Context(), bearer)
		if forwardedProviderUnavailable(err) {
			w.Header().Set("Retry-After", "1")
			http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
			return
		}
		if err != nil {
			validated.ServeHTTP(w, r)
			return
		}
		if limiter := s.oauthServer.UserRateLimiter; limiter != nil && !limiter.Allow(acceptance.UserInfo.ID) {
			forwardedRateLimit(w)
			return
		}
		ctx := handler.ContextWithUserInfo(r.Context(), acceptance.UserInfo)
		ctx = handler.ContextWithSessionID(ctx, acceptance.SessionID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Only ordinary ID tokens may use TrustedAudiences. Unverified claims only
// restrict this path; AcceptForwardedIDToken performs authentication. Delegated
// tokens need an actor policy, and sender-bound tokens need a DPoP proof; both
// remain with mcp-oauth's resource-bearer validation.
func unboundForwardedBearer(bearer string) bool {
	token, err := jwt.ParseSigned(bearer, []jose.SignatureAlgorithm{jose.RS256, jose.RS384, jose.RS512, jose.ES256, jose.ES384, jose.ES512, jose.EdDSA})
	if err != nil {
		return false
	}
	var claims map[string]any
	if err := token.UnsafeClaimsWithoutVerification(&claims); err != nil {
		return false
	}
	_, delegated := claims["act"]
	_, bound := claims["cnf"]
	return !delegated && !bound
}

func forwardedRateLimit(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "1")
	http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
}

// AcceptForwardedIDToken returns JWKS errors directly, without wrapping them
// in ErrProviderUnavailable. Keep a transient key-fetch failure distinct from
// an invalid token, while a restricted address remains a permanent refusal.
func forwardedProviderUnavailable(err error) bool {
	if err == nil || errors.Is(err, oidc.ErrRestrictedAddress) {
		return false
	}
	if errors.Is(err, oauthserver.ErrProviderUnavailable) || errors.Is(err, oauthserver.ErrStorageUnavailable) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var jwksErr *oidc.JWKSStatusError
	if errors.As(err, &jwksErr) {
		return jwksErr.StatusCode >= 500 || jwksErr.StatusCode == http.StatusTooManyRequests
	}
	var discoveryErr *oidc.DiscoveryStatusError
	if errors.As(err, &discoveryErr) {
		return discoveryErr.StatusCode >= 500 || discoveryErr.StatusCode == http.StatusTooManyRequests
	}
	var networkErr net.Error
	return errors.As(err, &networkErr)
}
