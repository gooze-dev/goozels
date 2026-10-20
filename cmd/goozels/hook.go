package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"gooze.dev/pkg/goozels/internal/adapter/hookstate"
)

// Hook output limits: the agent reads this after every edit, and Claude Code
// truncates additionalContext beyond 10,000 characters.
const (
	maxHookLines = 40
	maxHookBytes = 4000
)

// hookHeader introduces the hook's lines in the agent's context.
const hookHeader = "gooze results from the last run:"

// hookInput is the part of a PostToolUse hook payload goozels reads. Claude Code
// names the edited file in tool_input.file_path; Codex sends the apply_patch
// text (or a shell command) in tool_input.command. Both send session_id.
type hookInput struct {
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
	ToolInput struct {
		FilePath string          `json:"file_path"`
		Command  json.RawMessage `json:"command"`
	} `json:"tool_input"`
}

type hookOutput struct {
	HookSpecificOutput struct {
		HookEventName     string `json:"hookEventName"`
		AdditionalContext string `json:"additionalContext"`
	} `json:"hookSpecificOutput"`
}

// patchFileMarkers prefix the lines of a Codex apply_patch envelope that name a
// file left on disk after the patch ("*** Delete File:" is skipped).
var patchFileMarkers = []string{"*** Add File: ", "*** Update File: ", "*** Move to: "}

const hookUsage = "usage: goozels hook claude|codex  (reads a PostToolUse hook payload on stdin)"

// runHook implements `goozels hook claude|codex`: a PostToolUse hook that finds
// the files an edit touched and, when gooze has something to say about them,
// returns the diagnostics as additionalContext. Both agents read that same
// reply. Within one agent session, results already sent in full are only
// summarized until they change. It never blocks the edit; any problem just
// yields no output.
func runHook(args []string, stdin io.Reader, stdout, stderr io.Writer, state hookstate.Store) int {
	if len(args) != 1 || (args[0] != "claude" && args[0] != "codex") {
		_, _ = fmt.Fprintln(stderr, hookUsage)

		return 2
	}

	var in hookInput
	if err := json.NewDecoder(stdin).Decode(&in); err != nil {
		return 0
	}

	workdir := in.Cwd
	if workdir == "" {
		workdir = envOr("CLAUDE_PROJECT_DIR", ".")
	}

	files := []string{in.ToolInput.FilePath}
	// Equivalent: args[0] is "claude" or "codex" here, and "claude" sorts below "codex", so >= is ==.
	if args[0] == "codex" { //gooze:ignore comparison
		files = patchedFiles(commandText(in.ToolInput.Command))
	}

	entries := hookEntries(workdir, files)

	seen := state.Load(in.SessionID)
	markSeen(entries, seen)
	state.Save(in.SessionID, seen)

	out := render(entries, renderOptions{maxLines: maxHookLines, maxBytes: maxHookBytes, hint: true})

	lines := slices.Concat(out.notes, out.lines)
	if len(lines) == 0 {
		return 0
	}

	var reply hookOutput

	reply.HookSpecificOutput.HookEventName = "PostToolUse"
	reply.HookSpecificOutput.AdditionalContext = hookHeader + "\n" + strings.Join(lines, "\n")

	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)

	if err := enc.Encode(reply); err != nil {
		_, _ = fmt.Fprintf(stderr, "goozels: %v\n", err)
	}

	return 0
}

// hookEntries checks each file; relative paths resolve against workdir, which
// is also where paths are shown from and the fallback workspace root.
func hookEntries(workdir string, files []string) []entry {
	base, err := filepath.Abs(workdir)
	if err != nil {
		base = workdir
	}

	checker := newChecker(base)

	var entries []entry

	for _, f := range files {
		// Equivalent: an empty name resolves to the workspace directory itself, which cannot be read as a file to check.
		if f == "" { //gooze:ignore comparison
			continue
		}

		if !filepath.IsAbs(f) {
			f = filepath.Join(base, f)
		}

		if found, err := checkFile(checker, "", base, f); err == nil {
			entries = append(entries, found...)
		}
	}

	return uniqueEntries(entries)
}

// markSeen compares each entry's results with what this session was last sent
// for the same file, recorded in seen. Unchanged results are reduced to their
// summary (brief), and a repeated "no results" note is dropped. seen is
// updated with what is sent now.
func markSeen(entries []entry, seen map[string]string) {
	for i := range entries {
		e := &entries[i]

		d := digest(e)
		// Equivalent: an entry without a digest has neither results nor a note, so marking it brief prints the same lines.
		if d == "" { //gooze:ignore comparison
			continue
		}

		if seen[e.abs] == d {
			e.brief = true
			e.note = ""
		}

		seen[e.abs] = d
	}
}

// digest fingerprints an entry's gooze results (or its "no results" note), or
// returns "" when it has neither.
func digest(e *entry) string {
	var b strings.Builder

	b.WriteString(e.note)

	for _, d := range e.check.Results {
		b.WriteString("\n" + formatDiagnostic(e.shown, d))
	}

	// Equivalent: Builder.Len is never negative, so <= 0 is == 0.
	if b.Len() == 0 { //gooze:ignore comparison
		return ""
	}

	sum := sha256.Sum256([]byte(b.String()))

	return hex.EncodeToString(sum[:16])
}

// commandText decodes tool_input.command, which is a string for apply_patch and
// may be a string or an argv array for shell tools.
func commandText(raw json.RawMessage) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}

	var argv []string
	if json.Unmarshal(raw, &argv) == nil {
		return strings.Join(argv, "\n")
	}

	return ""
}

// patchedFiles lists, in order and without duplicates, the files an apply_patch
// envelope adds, updates or moves to.
func patchedFiles(patch string) []string {
	var files []string

	seen := map[string]bool{}

	for line := range strings.SplitSeq(patch, "\n") {
		for _, marker := range patchFileMarkers {
			if f, ok := strings.CutPrefix(strings.TrimSpace(line), marker); ok && !seen[f] {
				seen[f] = true

				files = append(files, f)
			}
		}
	}

	return files
}
