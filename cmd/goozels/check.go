package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gooze.dev/pkg/goozels/internal/adapter/reportstore"
	"gooze.dev/pkg/goozels/internal/controller"
	"gooze.dev/pkg/goozels/internal/model"
)

// maxHookLines caps the diagnostics a hook adds to an agent's context, so a
// file with hundreds of surviving mutants does not flood it.
const maxHookLines = 40

// runCheck implements `goozels check [-workdir dir] file...`: it prints gooze
// diagnostics as "path:line:col: severity: message" lines and prints nothing for
// clean files. It returns 2 on usage errors and 1 when a file cannot be read.
func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	workdir := fs.String("workdir", ".", "workspace root containing gooze's .gooze-reports directory")

	if err := fs.Parse(args); err != nil || fs.NArg() == 0 {
		_, _ = fmt.Fprintln(stderr, "usage: goozels check [-workdir dir] file...")

		return 2
	}

	checker := controller.Checker{Reports: reportstore.NewWorkspace()}
	status := 0

	for _, file := range fs.Args() {
		lines, err := checkFile(checker, *workdir, file)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "goozels: %v\n", err)

			status = 1
		}

		if len(lines) > 0 {
			if _, err := fmt.Fprintln(stdout, strings.Join(lines, "\n")); err != nil {
				return 1
			}
		}
	}

	return status
}

// checkFile returns the formatted diagnostics for one file, with paths shown
// relative to workdir when possible.
func checkFile(checker controller.Checker, workdir, file string) ([]string, error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}

	root, err := filepath.Abs(workdir)
	if err != nil {
		return nil, err
	}

	if !checker.Handles(abs) {
		return nil, nil
	}

	text, err := os.ReadFile(abs) //nolint:gosec // the caller names the file to check
	if err != nil {
		return nil, err
	}

	shown := abs
	if rel, err := filepath.Rel(root, abs); err == nil && !strings.HasPrefix(rel, "..") {
		shown = rel
	}

	diags := checker.Check(root, abs, string(text))
	lines := make([]string, 0, len(diags))

	for _, d := range diags {
		lines = append(lines, formatDiagnostic(shown, d))
	}

	return lines, nil
}

// formatDiagnostic renders a diagnostic in the compiler-style form editors and
// agents already parse, with one-based line and column.
func formatDiagnostic(path string, d model.Diagnostic) string {
	return path + ":" + strconv.Itoa(d.Range.Start.Line+1) + ":" + strconv.Itoa(d.Range.Start.Column+1) +
		": " + d.Severity.String() + ": " + d.Message
}

// hookInput is the part of a PostToolUse hook payload goozels reads. Claude Code
// names the edited file in tool_input.file_path; Codex sends the apply_patch
// text (or a shell command) in tool_input.command.
type hookInput struct {
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
// reply. It never blocks the edit; any problem just yields no output.
func runHook(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
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
	if args[0] == "codex" {
		files = patchedFiles(commandText(in.ToolInput.Command))
	}

	lines := hookLines(workdir, files)
	if len(lines) == 0 {
		return 0
	}

	var out hookOutput

	out.HookSpecificOutput.HookEventName = "PostToolUse"
	out.HookSpecificOutput.AdditionalContext = "goozels (gooze mutation testing) diagnostics after this edit:\n" +
		strings.Join(lines, "\n")

	if err := json.NewEncoder(stdout).Encode(out); err != nil {
		_, _ = fmt.Fprintf(stderr, "goozels: %v\n", err)
	}

	return 0
}

// hookLines checks each file (relative paths resolve against workdir) and caps
// the combined output at maxHookLines.
func hookLines(workdir string, files []string) []string {
	checker := controller.Checker{Reports: reportstore.NewWorkspace()}

	var lines []string

	for _, f := range files {
		if f == "" {
			continue
		}

		if !filepath.IsAbs(f) {
			f = filepath.Join(workdir, f)
		}

		found, err := checkFile(checker, workdir, f)
		if err == nil {
			lines = append(lines, found...)
		}
	}

	if extra := len(lines) - maxHookLines; extra > 0 {
		lines = append(lines[:maxHookLines], "… "+strconv.Itoa(extra)+" more; run `goozels check` on the file for all of them")
	}

	return lines
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
