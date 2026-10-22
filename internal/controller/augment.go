package controller

import (
	"encoding/json"

	"gooze.dev/pkg/goozels/internal/domain/command"
	"gooze.dev/pkg/goozels/pkg/jsonrpc"
	"gooze.dev/pkg/goozels/pkg/lsp"
)

// goozeTriggerChars are added to gopls's completion trigger characters so the
// editor requests completion where gooze features apply (mutagen lists, YAML
// values).
var goozeTriggerChars = []string{" ", ",", ":"}

// augmentInitialize merges gooze trigger characters into gopls's advertised
// capabilities before relaying the initialize response to the client.
func (s *Server) augmentInitialize(resp *jsonrpc.Message) {
	// Equivalent: an empty or one-byte result fails to unmarshal below and is
	// relayed unchanged the same way, so this check is only a shortcut.
	if len(resp.Result) == 0 { //gooze:ignore comparison, numbers
		s.sendClient(resp)
		return
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(resp.Result, &root); err != nil {
		s.sendClient(resp)
		return
	}

	if merged, ok := withGoozeCapabilities(root); ok {
		if raw, err := json.Marshal(merged); err == nil {
			resp.Result = raw
		}
	}

	s.sendClient(resp)
}

// withGoozeCapabilities returns the initialize result with gooze additions merged
// into gopls's capabilities: completion trigger characters and execute commands.
func withGoozeCapabilities(root map[string]json.RawMessage) (map[string]json.RawMessage, bool) {
	caps := map[string]json.RawMessage{}
	if raw, ok := root["capabilities"]; ok {
		if err := json.Unmarshal(raw, &caps); err != nil {
			return nil, false
		}
	}

	if !addTriggerChars(caps) || !addExecuteCommands(caps) {
		return nil, false
	}

	capsRaw, err := json.Marshal(caps)
	if err != nil {
		// Equivalent: caps holds only valid JSON values, so Marshal can't fail.
		return nil, false //gooze:ignore boolean
	}

	root["capabilities"] = capsRaw

	return root, true
}

// addTriggerChars merges goozeTriggerChars into completionProvider.
func addTriggerChars(caps map[string]json.RawMessage) bool {
	provider := map[string]json.RawMessage{}
	if raw, ok := caps["completionProvider"]; ok {
		if err := json.Unmarshal(raw, &provider); err != nil {
			return false
		}
	}

	var chars []string
	if raw, ok := provider["triggerCharacters"]; ok {
		_ = json.Unmarshal(raw, &chars)
	}

	if raw, err := json.Marshal(unionStrings(chars, goozeTriggerChars)); err == nil {
		provider["triggerCharacters"] = raw
	} else {
		// Equivalent: marshaling a []string can't fail.
		return false //gooze:ignore boolean
	}

	raw, err := json.Marshal(provider)
	if err != nil {
		// Equivalent: provider holds only valid JSON values, so Marshal can't fail.
		return false //gooze:ignore boolean
	}

	caps["completionProvider"] = raw

	return true
}

// addExecuteCommands merges gooze command identifiers into
// executeCommandProvider.commands so clients will dispatch them.
func addExecuteCommands(caps map[string]json.RawMessage) bool {
	provider := map[string]json.RawMessage{}
	if raw, ok := caps["executeCommandProvider"]; ok {
		if err := json.Unmarshal(raw, &provider); err != nil {
			return false
		}
	}

	var cmds []string
	if raw, ok := provider["commands"]; ok {
		_ = json.Unmarshal(raw, &cmds)
	}

	if raw, err := json.Marshal(unionStrings(cmds, command.Names())); err == nil {
		provider["commands"] = raw
	} else {
		// Equivalent: marshaling a []string can't fail.
		return false //gooze:ignore boolean
	}

	raw, err := json.Marshal(provider)
	if err != nil {
		// Equivalent: provider holds only valid JSON values, so Marshal can't fail.
		return false //gooze:ignore boolean
	}

	caps["executeCommandProvider"] = raw

	return true
}

// mergeCompletionResult appends our items to gopls's completion result,
// normalising whatever shape gopls returned (null, array, or CompletionList)
// into a CompletionList. gopls items are preserved verbatim as raw JSON.
func mergeCompletionResult(goplsResult json.RawMessage, ours []lsp.CompletionItem) json.RawMessage {
	isIncomplete, items := decodeCompletion(goplsResult)

	for _, it := range ours {
		if raw, err := json.Marshal(it); err == nil {
			items = append(items, raw)
		}
	}

	out := struct {
		IsIncomplete bool              `json:"isIncomplete"`
		Items        []json.RawMessage `json:"items"`
	}{IsIncomplete: isIncomplete, Items: items}
	if raw, err := json.Marshal(out); err == nil {
		return raw
	}

	return goplsResult
}

// decodeCompletion extracts items (as raw JSON) and the isIncomplete flag from a
// completion result of any shape.
func decodeCompletion(result json.RawMessage) (bool, []json.RawMessage) {
	trimmed := skipSpace(result)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return false, nil
	}

	switch trimmed[0] {
	case '[':
		var arr []json.RawMessage

		_ = json.Unmarshal(trimmed, &arr)

		return false, arr
	case '{':
		var list struct {
			IsIncomplete bool              `json:"isIncomplete"`
			Items        []json.RawMessage `json:"items"`
		}

		_ = json.Unmarshal(trimmed, &list)

		return list.IsIncomplete, list.Items
	}

	return false, nil
}

func skipSpace(b json.RawMessage) json.RawMessage {
	i := 0
	for i < len(b) && (b[i] == ' ' || b[i] == '\t' || b[i] == '\n' || b[i] == '\r') {
		i++
	}

	return b[i:]
}

func unionStrings(base, add []string) []string {
	seen := map[string]bool{}
	for _, s := range base {
		seen[s] = true
	}

	for _, s := range add {
		if !seen[s] {
			base = append(base, s)
			seen[s] = true
		}
	}

	return base
}
