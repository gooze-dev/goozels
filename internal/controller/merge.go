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
// nothing changes on screen until the user merges.
func (s *Server) offerMerge() {
	runDir := filepath.Join(config.DefaultReportsDir, command.RunReportsDir)

	msg := "gooze: function run done"
	if reports, err := s.reports.ReadDir(filepath.Join(s.getWorkdir(), runDir)); err == nil {
		msg += " — " + report.ScoreText(report.Summarize(reports))
	}

	msg += ". Its reports are in " + filepath.ToSlash(runDir) + " until you merge them into the main reports."

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
