// Package releasecheck looks up the latest published goozels release. It is an
// interface adapter over the Go module proxy, with an on-disk cache.
package releasecheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	latestReleaseURL     = "https://proxy.golang.org/gooze.dev/pkg/goozels/@latest"
	releaseCheckTTL      = 24 * time.Hour
	releaseCheckTimeout  = 5 * time.Second
	releaseCacheFileName = "latest-release.json"
)

// Checker looks up the latest goozels release on the Go module proxy, which
// reports the version `go install ...@latest` would install and, unlike the
// GitHub API, is not rate limited. Lookups are split so callers never wait on
// the network: Cached reads the last answer from disk, and Refresh (meant for a
// background goroutine) fetches and caches a new one.
type Checker struct {
	URL       string
	CachePath string // empty disables caching
	TTL       time.Duration
	Client    *http.Client
	Now       func() time.Time
}

// New returns a checker for goozels releases, cached under the user cache
// directory.
func New() *Checker {
	cachePath := ""
	if dir, err := os.UserCacheDir(); err == nil {
		cachePath = filepath.Join(dir, "goozels", releaseCacheFileName)
	}

	return &Checker{
		URL:       latestReleaseURL,
		CachePath: cachePath,
		TTL:       releaseCheckTTL,
		Client:    &http.Client{Timeout: releaseCheckTimeout},
		Now:       time.Now,
	}
}

type releaseCache struct {
	Tag       string    `json:"tag"`
	CheckedAt time.Time `json:"checked_at"`
}

// Cached returns the last looked-up release tag without touching the network,
// and whether that lookup is recent enough to skip a Refresh. A missing or
// unreadable cache is ("", false); a cached failed lookup is ("", true).
func (c *Checker) Cached() (string, bool) {
	cached, ok := c.readCache()
	if !ok {
		return "", false
	}

	return cached.Tag, c.Now().Sub(cached.CheckedAt) < c.TTL
}

// Refresh looks up the latest release tag on the network and caches the result.
// A failed lookup is cached too (as an empty tag) so an offline machine retries
// at most once per TTL; only a canceled lookup is left uncached.
func (c *Checker) Refresh(ctx context.Context) (string, error) {
	now := c.Now()

	tag, err := c.fetch(ctx)
	if errors.Is(ctx.Err(), context.Canceled) {
		return "", ctx.Err()
	}

	c.writeCache(releaseCache{Tag: tag, CheckedAt: now})

	return tag, err
}

func (c *Checker) fetch(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return "", err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("latest release lookup: %s", resp.Status)
	}

	var body struct {
		Version string `json:"Version"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("latest release lookup: %w", err)
	}

	return body.Version, nil
}

func (c *Checker) readCache() (releaseCache, bool) {
	// Equivalent: no path sorts before "", and reading "" fails like a missing file.
	if c.CachePath == "" { //gooze:ignore comparison
		return releaseCache{}, false
	}

	data, err := os.ReadFile(c.CachePath)
	if err != nil {
		return releaseCache{}, false
	}

	var cached releaseCache
	if err := json.Unmarshal(data, &cached); err != nil {
		return releaseCache{}, false
	}

	return cached, true
}

// writeCache stores the lookup result atomically (temp file, then rename), so a
// process killed mid-write never leaves a truncated cache. Failures are ignored
// since the cache is only an optimization.
func (c *Checker) writeCache(cached releaseCache) {
	// Equivalent: no path sorts before "", and renaming onto "" fails, so the
	// temp file is removed and nothing is written.
	if c.CachePath == "" { //gooze:ignore comparison
		return
	}

	data, err := json.Marshal(cached)
	if err != nil {
		return
	}

	if err := os.MkdirAll(filepath.Dir(c.CachePath), 0o750); err != nil {
		return
	}

	tmp, err := os.CreateTemp(filepath.Dir(c.CachePath), releaseCacheFileName+".*.tmp")
	if err != nil {
		return
	}

	// Best-effort cleanup of a failed write; after the rename there is nothing
	// left to remove.
	defer func() { _ = os.Remove(tmp.Name()) }()

	_, writeErr := tmp.Write(data)
	if err := tmp.Close(); err != nil || writeErr != nil {
		return
	}

	_ = os.Rename(tmp.Name(), c.CachePath)
}
