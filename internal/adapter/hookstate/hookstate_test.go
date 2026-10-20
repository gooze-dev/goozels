package hookstate

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestLoadSave(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "state"))
	if got := s.Load("abc"); len(got) != 0 {
		t.Errorf("a new session should be empty, got %v", got)
	}

	s.Save("abc", map[string]string{"/w/x.go": "d1"})
	if got := s.Load("abc"); got["/w/x.go"] != "d1" {
		t.Errorf("Load after Save = %v", got)
	}
	if got := s.Load("other"); len(got) != 0 {
		t.Errorf("sessions must not share state, got %v", got)
	}
}

func TestDisabled(t *testing.T) {
	for _, s := range []Store{New(""), New(t.TempDir())} {
		s.Save("", map[string]string{"k": "v"})
		if got := s.Load(""); len(got) != 0 {
			t.Errorf("an empty session id keeps no state, got %v", got)
		}
	}
	if Default().dir == "" && os.Getenv("HOME") != "" {
		t.Log("no user cache directory; the default store is disabled")
	}
}

func TestCorruptStateIsIgnored(t *testing.T) {
	s := New(t.TempDir())
	path, _ := s.path("abc")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := s.Load("abc"); len(got) != 0 {
		t.Errorf("corrupt state should load empty, got %v", got)
	}
}

func TestSavePrunesOldSessions(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	s.Save("old", map[string]string{})
	old, _ := s.path("old")
	stale := time.Now().Add(-maxAge - time.Hour)
	if err := os.Chtimes(old, stale, stale); err != nil {
		t.Fatal(err)
	}

	s.Save("new", map[string]string{})
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("an idle session's state should be pruned, stat err = %v", err)
	}
	if fresh, _ := s.path("new"); fileMissing(fresh) {
		t.Error("the saved session's state was removed")
	}
}

func fileMissing(path string) bool {
	_, err := os.Stat(path)
	return err != nil
}

func TestDefaultWithoutCacheDir(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")
	if d := Default().dir; d != "" {
		t.Errorf("without a cache directory the store should be disabled, got dir %q", d)
	}
}

func TestDefaultUsesCacheDir(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/cache")
	if d := Default().dir; d != filepath.Join("/cache", "goozels", "hook") {
		t.Errorf("Default().dir = %q", d)
	}
}

func TestSessionsSharingAFirstHashByte(t *testing.T) {
	// Find two session ids whose hashes share their first byte.
	first := map[string]string{}
	var a, b string
	for i := 0; a == ""; i++ {
		id := "s" + strconv.Itoa(i)
		p, _ := New("/d").path(id)
		key := filepath.Base(p)[:2]
		if other, ok := first[key]; ok {
			a, b = other, id
		}
		first[key] = id
	}

	s := New(t.TempDir())
	s.Save(a, map[string]string{"k": "a"})
	s.Save(b, map[string]string{"k": "b"})
	if got := s.Load(a)["k"]; got != "a" {
		t.Errorf("session %s reads %q, want its own state", a, got)
	}
}

func TestPruneAges(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	s := Store{dir: dir, now: func() time.Time { return now }}
	files := map[string]time.Duration{
		"recent.json":   6 * 24 * time.Hour,
		"edge.json":     maxAge, // exactly maxAge old is kept
		"old.json":      maxAge + 30*time.Minute,
		"older.json":    maxAge + 48*time.Hour,
		"unrelated.txt": maxAge + 48*time.Hour,
	}
	for name, age := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}

	s.prune()
	for name, kept := range map[string]bool{"recent.json": true, "edge.json": true, "old.json": false, "older.json": false, "unrelated.txt": true} {
		if fileMissing(filepath.Join(dir, name)) == kept {
			t.Errorf("%s: kept = %v, want %v", name, !kept, kept)
		}
	}
}
