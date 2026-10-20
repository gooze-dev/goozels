package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The agent integrations under editors/ call goozels; these tests keep them in
// step with the command line.

func readRepoFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestCodexHookGuardsMissingBinary(t *testing.T) {
	var cfg struct {
		Hooks struct {
			PostToolUse []struct {
				Hooks []struct {
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"PostToolUse"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(readRepoFile(t, "editors/codex/hooks.json")), &cfg); err != nil {
		t.Fatal(err)
	}
	command := cfg.Hooks.PostToolUse[0].Hooks[0].Command
	for _, want := range []string{"goozels version", "goozels*)", "exec goozels hook codex", "exit 0"} {
		if !strings.Contains(command, want) {
			t.Errorf("hooks.json command %q lacks %q: it must do nothing when goozels is missing", command, want)
		}
	}

	readme := readRepoFile(t, "editors/codex/README.md")
	if !strings.Contains(readme, "command = "+strconv.Quote(command)) {
		t.Errorf("the TOML example in editors/codex/README.md must use the same command as hooks.json:\n%s", command)
	}
	if !strings.Contains(readme, "https://raw.githubusercontent.com/gooze-dev/goozels/main/editors/codex/hooks.json") {
		t.Error("editors/codex/README.md should give a copy command that needs no clone")
	}
}

func TestOpencodePluginsPassTheHookCap(t *testing.T) {
	for _, path := range []string{"editors/opencode/v1/goozels.ts", "editors/opencode/v2/goozels.ts"} {
		src := readRepoFile(t, path)
		if !strings.Contains(src, strconv.Itoa(maxHookLines)) || !strings.Contains(src, "--max") {
			t.Errorf("%s should run goozels check --max %d, like the hooks", path, maxHookLines)
		}
		if !strings.Contains(src, hookHeader) {
			t.Errorf("%s should introduce results with %q, like the hooks", path, hookHeader)
		}
	}
}
