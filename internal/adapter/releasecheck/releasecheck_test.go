package releasecheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var releaseTestNow = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

func newTestChecker(t *testing.T, handler http.HandlerFunc) (*Checker, *int) {
	t.Helper()

	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	return &Checker{
		URL:       srv.URL,
		CachePath: filepath.Join(t.TempDir(), "goozels", releaseCacheFileName),
		TTL:       time.Hour,
		Client:    srv.Client(),
		Now:       func() time.Time { return releaseTestNow },
	}, &hits
}

func writeReleaseCache(t *testing.T, c *Checker, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(c.CachePath), 0o750))
	require.NoError(t, os.WriteFile(c.CachePath, []byte(content), 0o600))
}

func serveVersion(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"Version":"` + version + `","Time":"2026-10-20T17:12:32Z"}`))
	}
}

func TestChecker_Cached(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		cache     string // "" means no cache file
		wantTag   string
		wantFresh bool
	}{
		"missing":         {"", "", false},
		"corrupt":         {"{not json", "", false},
		"fresh":           {`{"tag":"v1.2.5","checked_at":"2026-10-21T11:30:00Z"}`, "v1.2.5", true},
		"stale":           {`{"tag":"v1.0.0","checked_at":"2026-10-21T10:59:59Z"}`, "v1.0.0", false},
		"exactly TTL old": {`{"tag":"v1.1.0","checked_at":"2026-10-21T11:00:00Z"}`, "v1.1.0", false},
		"cached failure":  {`{"tag":"","checked_at":"2026-10-21T11:30:00Z"}`, "", true},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			checker, hits := newTestChecker(t, serveVersion("v1.3.0"))
			if tt.cache != "" {
				writeReleaseCache(t, checker, tt.cache)
			}

			tag, fresh := checker.Cached()
			assert.Equal(t, tt.wantTag, tag)
			assert.Equal(t, tt.wantFresh, fresh)
			assert.Equal(t, 0, *hits, "Cached must never touch the network")
		})
	}
}

// A missing or unreadable cache is never fresh, whatever the clock says: even
// a clock at the zero time must not turn "no cache" into a fresh answer.
func TestChecker_CachedMissingIsNeverFresh(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		noCachePath bool
		cache       string // "" means no cache file
	}{
		"no cache path": {noCachePath: true},
		"missing":       {},
		"corrupt":       {cache: "{not json"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			checker, _ := newTestChecker(t, serveVersion("v1.3.0"))
			checker.Now = func() time.Time { return time.Time{} }

			if tt.cache != "" {
				writeReleaseCache(t, checker, tt.cache)
			}

			if tt.noCachePath {
				checker.CachePath = ""
			}

			tag, fresh := checker.Cached()
			assert.Empty(t, tag)
			assert.False(t, fresh)
		})
	}
}

func TestChecker_CachedWithoutCachePath(t *testing.T) {
	t.Parallel()

	tag, fresh := (&Checker{Now: time.Now}).Cached()
	assert.Empty(t, tag)
	assert.False(t, fresh)
}

func TestChecker_RefreshWritesCache(t *testing.T) {
	t.Parallel()

	checker, hits := newTestChecker(t, serveVersion("v1.3.0"))
	writeReleaseCache(t, checker, `{"tag":"v1.0.0","checked_at":"2026-10-21T10:00:00Z"}`)

	tag, err := checker.Refresh(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "v1.3.0", tag)
	assert.Equal(t, 1, *hits)

	tag, fresh := checker.Cached()
	assert.Equal(t, "v1.3.0", tag)
	assert.True(t, fresh)

	entries, err := os.ReadDir(filepath.Dir(checker.CachePath))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "the atomic write must not leave temp files behind")
}

func TestChecker_RefreshCachesFailures(t *testing.T) {
	t.Parallel()

	checker, _ := newTestChecker(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	_, err := checker.Refresh(context.Background())
	require.Error(t, err)

	tag, fresh := checker.Cached()
	assert.Empty(t, tag)
	assert.True(t, fresh, "a failed lookup should not be retried within the TTL")
}

// Only 200 is an answer: a status above or below it is an error even when the
// body holds a version (101 is the one status below 200 a client receives).
func TestChecker_RefreshRejectsNon200EvenWithAVersion(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusSwitchingProtocols, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()

			checker, _ := newTestChecker(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"Version":"v9.9.9"}`))
			})

			tag, err := checker.Refresh(context.Background())
			require.ErrorContains(t, err, http.StatusText(status), "the error names the status")
			assert.Empty(t, tag)
		})
	}
}

func TestChecker_RefreshOffline(t *testing.T) {
	t.Parallel()

	checker, _ := newTestChecker(t, serveVersion("v1.3.0"))
	checker.URL = "http://127.0.0.1:1" // connection refused
	checker.Client = &http.Client{Timeout: time.Second}

	_, err := checker.Refresh(context.Background())
	require.Error(t, err)

	_, fresh := checker.Cached()
	assert.True(t, fresh, "an offline lookup is cached so the next run does not retry")
}

func TestChecker_RefreshBadJSON(t *testing.T) {
	t.Parallel()

	checker, _ := newTestChecker(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	})
	checker.CachePath = ""

	_, err := checker.Refresh(context.Background())
	require.Error(t, err)
}

func TestChecker_RefreshCanceledIsNotCached(t *testing.T) {
	t.Parallel()

	checker, _ := newTestChecker(t, serveVersion("v1.3.0"))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := checker.Refresh(ctx)
	require.ErrorIs(t, err, context.Canceled)

	_, statErr := os.Stat(checker.CachePath)
	assert.True(t, os.IsNotExist(statErr), "a canceled lookup must not be cached")
}

func TestNew(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	t.Setenv("HOME", cacheHome)

	dir, err := os.UserCacheDir()
	require.NoError(t, err)

	checker := New()
	assert.Equal(t, latestReleaseURL, checker.URL)
	assert.Equal(t, filepath.Join(dir, "goozels", releaseCacheFileName), checker.CachePath)
	assert.Equal(t, 24*time.Hour, checker.TTL, "check for a new release at most once a day")
	require.NotNil(t, checker.Client)
	assert.Equal(t, 5*time.Second, checker.Client.Timeout, "a lookup never blocks for long")
	require.NotNil(t, checker.Now)
}

func TestNew_WithoutUserCacheDirDisablesCaching(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")
	t.Setenv("LocalAppData", "")

	if _, err := os.UserCacheDir(); err == nil {
		t.Skip("this platform has a user cache directory without HOME")
	}

	assert.Empty(t, New().CachePath)
}
