package server

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	dexProxyChildEnv = "MCP_KUBERNETES_DEX_PROXY_CHILD"
	dexProxyHost     = "dex.invalid"
)

// http.ProxyFromEnvironment reads the environment once per process, so each
// case runs in its own test process with the proxy variables set before the
// first request.
func TestDexProviderWithCAFollowsProxyEnvironment(t *testing.T) {
	if mode := os.Getenv(dexProxyChildEnv); mode != "" {
		runDexProxyChild(t, mode)
		return
	}
	for _, mode := range []string{"proxied", "no_proxy"} {
		t.Run(mode, func(t *testing.T) {
			child := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestDexProviderWithCAFollowsProxyEnvironment$", "-test.count=1") //nolint:gosec // this test binary
			child.Env = append(os.Environ(), dexProxyChildEnv+"="+mode)
			output, err := child.CombinedOutput()
			require.NoError(t, err, string(output))
		})
	}
}

func runDexProxyChild(t *testing.T, mode string) {
	dexAddress, caFile := newDexBehindProxy(t)
	proxy := newConnectProxy(t, dexAddress)

	t.Setenv("HTTPS_PROXY", proxy.url)
	t.Setenv("https_proxy", proxy.url)
	if mode == "no_proxy" {
		t.Setenv("NO_PROXY", dexProxyHost)
		t.Setenv("no_proxy", dexProxyHost)
	}

	oauthServer, _, err := createOAuthServer(OAuthConfig{
		BaseURL:         "https://mcp.example.com",
		Provider:        OAuthProviderDex,
		DexIssuerURL:    "https://" + dexProxyHost,
		DexClientID:     "mcp-kubernetes",
		DexClientSecret: "test-secret",
		DexCAFile:       caFile,
	})

	switch mode {
	case "proxied":
		require.NoError(t, err)
		require.NotNil(t, oauthServer)
		require.Contains(t, proxy.targets(), dexProxyHost+":443")
	case "no_proxy":
		require.Error(t, err)
		require.Contains(t, err.Error(), dexProxyHost)
		require.Empty(t, proxy.targets())
	default:
		t.Fatalf("unknown mode %q", mode)
	}
}

// newDexBehindProxy serves Dex discovery for https://dex.invalid over a
// private CA. The name never resolves, so only a proxy can reach it.
func newDexBehindProxy(t *testing.T) (string, string) {
	t.Helper()
	issuer := "https://" + dexProxyHost
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                issuer,
			"authorization_endpoint":                issuer + "/auth",
			"token_endpoint":                        issuer + "/token",
			"userinfo_endpoint":                     issuer + "/userinfo",
			"jwks_uri":                              issuer + "/keys",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	}))
	certTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		DNSNames: []string{dexProxyHost}, IsCA: true, BasicConstraintsValid: true,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, certTemplate, certTemplate, &key.PublicKey, key)
	require.NoError(t, err)
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS12}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	caFile := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))
	return srv.Listener.Addr().String(), caFile
}

type connectProxy struct {
	url       string
	mu        sync.Mutex
	requested []string
}

func (p *connectProxy) targets() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.requested...)
}

// newConnectProxy answers CONNECT for any target by tunnelling to upstream,
// recording the target the client asked for.
func newConnectProxy(t *testing.T, upstream string) *connectProxy {
	t.Helper()
	proxy := &connectProxy{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		proxy.mu.Lock()
		proxy.requested = append(proxy.requested, r.Host)
		proxy.mu.Unlock()

		upstreamConn, err := net.Dial("tcp", upstream)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			_ = upstreamConn.Close()
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		clientConn, buffered, err := hijacker.Hijack()
		if err != nil {
			_ = upstreamConn.Close()
			return
		}
		if _, err := clientConn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n")); err != nil {
			_ = upstreamConn.Close()
			_ = clientConn.Close()
			return
		}
		go func() {
			_, _ = io.Copy(upstreamConn, buffered)
			_ = upstreamConn.Close()
		}()
		_, _ = io.Copy(clientConn, upstreamConn)
		_ = clientConn.Close()
	}))
	t.Cleanup(srv.Close)
	proxy.url = srv.URL
	return proxy
}
