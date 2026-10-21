package controller

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"

	"gooze.dev/pkg/gooze/pkg/config"

	"gooze.dev/pkg/goozels/internal/domain/command"
	"gooze.dev/pkg/goozels/internal/domain/report"
	"gooze.dev/pkg/goozels/pkg/jsonrpc"
	"gooze.dev/pkg/goozels/pkg/lsp"
)

// offerMerge tells the editor a function run's score and offers to fold its
// reports into the main reports, which the editor shows. A function run
// writes to the reports' run directory, which goozels does not read, so
// nothing changes on screen until the user merges. A run that re-tested
// nothing (the function is cached, or has no mutants) only says so. args
// are the run's gooze arguments: run --run <pattern> <package>.
func (s *Server) offerMerge(args []string) {
	runDir := filepath.Join(config.DefaultReportsDir, command.RunReportsDir)

	reports, err := s.reports.ReadDir(filepath.Join(s.getWorkdir(), runDir))
	if err != nil || len(reports) == 0 {
		pattern, pkg := args[2], args[3]
		s.notify("window/showMessage", lsp.ShowMessageParams{
			Type: lsp.MessageInfo,
			Message: "gooze: nothing re-tested for " + pattern + ": it is unchanged since the last run, so its results are already shown, or it has nothing to mutate. " +
				"To test it again anyway: gooze run --no-cache --run '" + pattern + "' " + pkg,
		})

		return
	}

	msg := "gooze: function run done — " + report.ScoreText(report.Summarize(reports)) +
		". Its reports are in " + filepath.ToSlash(runDir) + " until you merge them into the main reports."

	s.requestClient("window/showMessageRequest", lsp.ShowMessageRequestParams{
		Type:    lsp.MessageInfo,
		Message: msg,
		Actions: []lsp.MessageActionItem{{Title: command.MergeTitle}},
	}, func(resp *jsonrpc.Message) {
		var chosen lsp.MessageActionItem
		if json.Unmarshal(resp.Result, &chosen) == nil && chosen.Title == command.MergeTitle {
			go s.runMerge()
		}
	})
}

// runMerge folds the run directory into the main reports and refreshes the
// editor's markers.
func (s *Server) runMerge() {
	ctx, cancel := context.WithTimeout(context.Background(), goozeRunTimeout)
	defer cancel()

	out, err := s.gooze.Run(ctx, s.getWorkdir(), command.MergeArgs()...)
	s.reportGoozeResult("gooze report merge", "./...", out, err)
}

// requestClient sends the editor a request and calls onResponse with its
// reply. Our ids are strings ("goozels-1"), so they never collide with the
// numeric ids of gopls's requests that pass through.
func (s *Server) requestClient(method string, params any, onResponse func(*jsonrpc.Message)) {
	raw, err := json.Marshal(params)
	if err != nil {
		return
	}

	s.mu.Lock()
	s.nextRequest++
	id := json.RawMessage(strconv.Quote("goozels-" + strconv.Itoa(s.nextRequest)))

	if s.clientRequests == nil {
		s.clientRequests = map[string]func(*jsonrpc.Message){}
	}

	s.clientRequests[string(id)] = onResponse
	s.mu.Unlock()

	s.sendClient(&jsonrpc.Message{JSONRPC: jsonrpc.Version, ID: id, Method: method, Params: raw})
}

// takeClientRequest pops the response handler of one of our requests.
func (s *Server) takeClientRequest(id string) (func(*jsonrpc.Message), bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fn, ok := s.clientRequests[id]
	delete(s.clientRequests, id)

	return fn, ok
}
