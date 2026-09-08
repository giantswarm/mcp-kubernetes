package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/creativeprojects/go-selfupdate"
	"github.com/stretchr/testify/assert"
)

func TestSelfUpdateCmd(t *testing.T) {
	tests := []struct {
		name          string
		version       string
		expectError   bool
		errorContains string
	}{
		{
			name:          "self-update with dev version should fail",
			version:       "dev",
			expectError:   true,
			errorContains: "cannot self-update a development version",
		},
		{
			name:          "self-update with empty version should fail",
			version:       "",
			expectError:   true,
			errorContains: "cannot self-update a development version",
		},
		// Note: Testing actual updates would require network access and real releases
		// which is not suitable for unit tests
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up the root command version
			originalVersion := rootCmd.Version
			defer func() {
				rootCmd.Version = originalVersion
			}()
			rootCmd.Version = tt.version

			// Create the self-update command
			cmd := newSelfUpdateCmd()

			// Execute the command
			err := cmd.Execute()

			// Assertions
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSelfUpdateCmdProperties(t *testing.T) {
	cmd := newSelfUpdateCmd()

	assert.Equal(t, "self-update", cmd.Use)
	assert.Equal(t, "Update mcp-kubernetes to the latest version", cmd.Short)
	assert.True(t, strings.Contains(cmd.Long, "mcp-kubernetes"))
	assert.True(t, strings.Contains(cmd.Long, "GitHub"))
	assert.True(t, strings.Contains(cmd.Long, "Sigstore bundle"), "the help should say that releases are verified")
}

func TestGithubRepoSlug(t *testing.T) {
	// Ensure the GitHub repository slug is correctly set
	assert.Equal(t, "giantswarm/mcp-kubernetes", githubRepoSlug)
}

// The signature check itself (a bundle that verifies, a tampered binary, a
// bundle for another repository) is tested where it lives, in
// github.com/giantswarm/selfupdate-cosign. What follows checks that
// mcp-kubernetes wires it in so that an unsigned or unverifiable release never
// reaches the disk.

// fakeSource stands in for GitHub: one release, and the bytes every asset
// download returns.
type fakeSource struct {
	release fakeRelease
	assets  map[int64][]byte
}

func (s *fakeSource) ListReleases(context.Context, selfupdate.Repository) ([]selfupdate.SourceRelease, error) {
	return []selfupdate.SourceRelease{s.release}, nil
}

func (s *fakeSource) DownloadReleaseAsset(_ context.Context, _ *selfupdate.Release, id int64) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(s.assets[id])), nil
}

type fakeAsset struct {
	id   int64
	name string
}

func (a fakeAsset) GetID() int64                  { return a.id }
func (a fakeAsset) GetName() string               { return a.name }
func (a fakeAsset) GetSize() int                  { return 3 }
func (a fakeAsset) GetBrowserDownloadURL() string { return "https://example.test/" + a.name }

type fakeRelease struct {
	tag    string
	assets []selfupdate.SourceAsset
}

func (r fakeRelease) GetID() int64              { return 1 }
func (r fakeRelease) GetTagName() string        { return r.tag }
func (r fakeRelease) GetDraft() bool            { return false }
func (r fakeRelease) GetPrerelease() bool       { return false }
func (r fakeRelease) GetPublishedAt() time.Time { return time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC) }
func (r fakeRelease) GetReleaseNotes() string   { return "notes" }
func (r fakeRelease) GetName() string           { return r.tag }
func (r fakeRelease) GetURL() string {
	return "https://github.com/giantswarm/mcp-kubernetes/releases/tag/" + r.tag
}
func (r fakeRelease) GetAssets() []selfupdate.SourceAsset { return r.assets }

// binaryAsset is the asset name architect publishes for this platform.
func binaryAsset() string {
	name := "mcp-kubernetes-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// selfUpdateFixture points the command at src instead of GitHub and at a
// throwaway file instead of the running executable; it returns that file's
// path and its content, so a test can assert the file survived.
func selfUpdateFixture(t *testing.T, src *fakeSource) (string, []byte) {
	t.Helper()
	installed := []byte("the mcp-kubernetes that is installed right now")
	exe := filepath.Join(t.TempDir(), "mcp-kubernetes")
	if err := os.WriteFile(exe, installed, 0o755); err != nil { //nolint:gosec // an executable
		t.Fatal(err)
	}
	prevSource, prevExe, prevVersion := selfUpdateSource, selfUpdateExecutable, rootCmd.Version
	selfUpdateSource = src
	selfUpdateExecutable = func() (string, error) { return exe, nil }
	rootCmd.Version = "v1.0.0"
	t.Cleanup(func() {
		selfUpdateSource, selfUpdateExecutable, rootCmd.Version = prevSource, prevExe, prevVersion
	})
	return exe, installed
}

func assertUnchanged(t *testing.T, exe string, installed []byte) {
	t.Helper()
	got, err := os.ReadFile(exe) //nolint:gosec // the test's own temp file
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, installed) {
		t.Fatalf("the installed binary was replaced: %q", got)
	}
}

func TestRunSelfUpdateRefusesAReleaseWithoutASignatureBundle(t *testing.T) {
	src := &fakeSource{
		release: fakeRelease{tag: "v99.0.0", assets: []selfupdate.SourceAsset{fakeAsset{1, binaryAsset()}}},
		assets:  map[int64][]byte{1: []byte("a newer mcp-kubernetes, unsigned")},
	}
	exe, installed := selfUpdateFixture(t, src)

	err := runSelfUpdate(nil, nil)
	if err == nil {
		t.Fatal("a release without a bundle must be refused")
	}
	if !strings.Contains(err.Error(), "no signature bundle") {
		t.Errorf("the error should say what is missing, got: %v", err)
	}
	assertUnchanged(t, exe, installed)
}

func TestRunSelfUpdateRefusesADownloadThatDoesNotVerify(t *testing.T) {
	src := &fakeSource{
		release: fakeRelease{tag: "v99.0.0", assets: []selfupdate.SourceAsset{
			fakeAsset{1, binaryAsset()},
			fakeAsset{2, binaryAsset() + ".bundle"},
		}},
		assets: map[int64][]byte{
			1: []byte("a newer mcp-kubernetes"),
			2: []byte("{}"), // not a Sigstore bundle
		},
	}
	exe, installed := selfUpdateFixture(t, src)

	var out bytes.Buffer
	cmd := newSelfUpdateCmd()
	cmd.SetOut(&out)
	err := runSelfUpdate(cmd, nil)
	if err == nil {
		t.Fatal("a download whose bundle does not verify must be refused")
	}
	if !strings.Contains(err.Error(), "is unchanged") || !strings.Contains(err.Error(), "is not a Sigstore bundle") {
		t.Errorf("the error should say the binary was refused and why, got: %v", err)
	}
	if !strings.Contains(out.String(), "Found newer version: 99.0.0") {
		t.Errorf("the newer release should have been announced before the refusal, got:\n%s", out.String())
	}
	assertUnchanged(t, exe, installed)
}
