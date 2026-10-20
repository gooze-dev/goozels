package controller

import (
	"encoding/json"
	"strings"
	"testing"

	"gooze.dev/pkg/goozels/pkg/lsp"
)

func TestMergeCompletionResult(t *testing.T) {
	ours := []lsp.CompletionItem{{Label: "arithmetic"}}

	cases := map[string]json.RawMessage{
		"null":   json.RawMessage("null"),
		"array":  json.RawMessage(`[{"label":"fmt"}]`),
		"object": json.RawMessage(`{"isIncomplete":true,"items":[{"label":"fmt"}]}`),
	}
	for name, gopls := range cases {
		t.Run(name, func(t *testing.T) {
			merged := mergeCompletionResult(gopls, ours)
			var list struct {
				Items []struct {
					Label string `json:"label"`
				} `json:"items"`
			}
			if err := json.Unmarshal(merged, &list); err != nil {
				t.Fatalf("decode merged: %v", err)
			}
			has := map[string]bool{}
			for _, it := range list.Items {
				has[it.Label] = true
			}
			if !has["arithmetic"] {
				t.Error("our item missing after merge")
			}
			if name != "null" && !has["fmt"] {
				t.Error("gopls item dropped after merge")
			}
		})
	}
}

func TestDecodeCompletion(t *testing.T) {
	if inc, items := decodeCompletion(json.RawMessage("null")); inc || items != nil {
		t.Errorf("null decode = %v, %v", inc, items)
	}
	if _, items := decodeCompletion(json.RawMessage(`[{"label":"a"},{"label":"b"}]`)); len(items) != 2 {
		t.Errorf("array decode len = %d, want 2", len(items))
	}
	if inc, items := decodeCompletion(json.RawMessage(`{"isIncomplete":true,"items":[{"label":"a"}]}`)); !inc || len(items) != 1 {
		t.Errorf("object decode = %v, len %d", inc, len(items))
	}
}

func TestWithGoozeCapabilities(t *testing.T) {
	root := map[string]json.RawMessage{
		"capabilities": json.RawMessage(`{"completionProvider":{"triggerCharacters":["."]},"executeCommandProvider":{"commands":["gopls.x"]}}`),
	}
	merged, ok := withGoozeCapabilities(root)
	if !ok {
		t.Fatal("withGoozeCapabilities returned false")
	}
	var caps struct {
		Capabilities struct {
			CompletionProvider struct {
				TriggerCharacters []string `json:"triggerCharacters"`
			} `json:"completionProvider"`
			ExecuteCommandProvider struct {
				Commands []string `json:"commands"`
			} `json:"executeCommandProvider"`
		} `json:"capabilities"`
	}
	raw, _ := json.Marshal(merged)
	if err := json.Unmarshal(raw, &caps); err != nil {
		t.Fatalf("decode: %v", err)
	}
	tc := strings.Join(caps.Capabilities.CompletionProvider.TriggerCharacters, "")
	if !strings.Contains(tc, ".") || !strings.Contains(tc, ":") {
		t.Errorf("trigger chars not merged: %q", tc)
	}
	cmds := strings.Join(caps.Capabilities.ExecuteCommandProvider.Commands, ",")
	if !strings.Contains(cmds, "gopls.x") || !strings.Contains(cmds, "gooze.run") {
		t.Errorf("commands not merged: %q", cmds)
	}
}

func TestUnionStrings(t *testing.T) {
	got := unionStrings([]string{"a", "b"}, []string{"b", "c"})
	want := "a,b,c"
	if strings.Join(got, ",") != want {
		t.Errorf("unionStrings = %v, want %s", got, want)
	}
}
