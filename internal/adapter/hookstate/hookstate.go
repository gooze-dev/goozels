// Package hookstate remembers, per coding-agent session, what the post-edit
// hook last reported for each file, so unchanged gooze results are summarized
// instead of resent after every edit. It is an interface adapter over small
// files in the user's cache directory. Everything is best effort: a state that
// cannot be read or written just means the full results are sent again.
package hookstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// maxAge is how long a session's state is kept after its last update.
const maxAge = 7 * 24 * time.Hour

// Store keeps session states as JSON files in one directory.
type Store struct {
	dir string
	now func() time.Time
}

// New returns a Store keeping its files in dir. An empty dir disables it.
func New(dir string) Store {
	return Store{dir: dir, now: time.Now}
}

// Default returns a Store in the user's cache directory
// (os.UserCacheDir()/goozels/hook), or a disabled one when there is none.
func Default() Store {
	cache, err := os.UserCacheDir()
	if err != nil {
		return New("")
	}

	return New(filepath.Join(cache, "goozels", "hook"))
}

// Load returns what was recorded for session: a digest per file key. It is
// empty for a new session, an empty session id or an unreadable state.
func (s Store) Load(session string) map[string]string {
	seen := map[string]string{}

	path, ok := s.path(session)
	if !ok {
		return seen
	}

	if data, err := os.ReadFile(path); err == nil { //nolint:gosec // the path is derived from a hash
		_ = json.Unmarshal(data, &seen)
	}

	return seen
}

// Save records seen for session and removes states of sessions idle for
// longer than maxAge. Errors are ignored.
func (s Store) Save(session string, seen map[string]string) {
	path, ok := s.path(session)
	if !ok {
		return
	}

	data, err := json.Marshal(seen)
	if err != nil || os.MkdirAll(s.dir, 0o700) != nil {
		return
	}

	_ = os.WriteFile(path, data, 0o600)

	s.prune()
}

// path names the state file of a session, hashing the id so any string makes
// a safe file name.
func (s Store) path(session string) (string, bool) {
	if s.dir == "" || session == "" {
		return "", false
	}

	sum := sha256.Sum256([]byte(session))

	return filepath.Join(s.dir, hex.EncodeToString(sum[:8])+".json"), true
}

// prune removes state files older than maxAge.
func (s Store) prune() {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}

	for _, e := range entries {
		info, err := e.Info()
		if err != nil || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}

		if s.now().Sub(info.ModTime()) <= maxAge {
			continue
		}

		// Another hook may prune the same file; any other failure means the
		// directory is not writable, so stop trying.
		if err := os.Remove(filepath.Join(s.dir, e.Name())); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return
		}
	}
}
