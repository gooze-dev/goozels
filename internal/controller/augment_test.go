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

func TestDecodeCompletionShapes(t *testing.T) {
	tests := map[string]struct {
		result     string
		incomplete bool
		items      int
	}{
		"empty":                {"", false, 0},
		"whitespace only":      {" \n\t", false, 0},
		"array is complete":    {`[{"label":"a"}]`, false, 1},
		"leading whitespace":   {" \r\n\t[{\"label\":\"a\"}]", false, 1},
		"other value is empty": {`5`, false, 0},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			incomplete, items := decodeCompletion(json.RawMessage(tt.result))
			if incomplete != tt.incomplete || len(items) != tt.items {
				t.Errorf("decodeCompletion(%q) = %v, %d items; want %v, %d", tt.result, incomplete, len(items), tt.incomplete, tt.items)
			}
		})
	}
}

func TestSkipSpace(t *testing.T) {
	tests := map[string]string{
		"":              "",
		"   ":           "",
		" \t\n\r[1]":    "[1]",
		" [":            "[",
		"\t[":           "[",
		"\n[":           "[",
		"\r[":           "[",
		"x ":            "x ",
		"\x01[":         "\x01[", // only JSON whitespace is skipped
		"\x08\x0b\x0c[": "\x08\x0b\x0c[",
	}
	for in, want := range tests {
		if got := string(skipSpace(json.RawMessage(in))); got != want {
			t.Errorf("skipSpace(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWithGoozeCapabilitiesRejectsMalformedCapabilities(t *testing.T) {
	tests := map[string]string{
		"capabilities":           `5`,
		"completionProvider":     `{"completionProvider":5}`,
		"executeCommandProvider": `{"executeCommandProvider":5}`,
	}
	for name, caps := range tests {
		t.Run(name, func(t *testing.T) {
			merged, ok := withGoozeCapabilities(map[string]json.RawMessage{"capabilities": json.RawMessage(caps)})
			if ok || merged != nil {
				t.Errorf("withGoozeCapabilities(%s) = %v, %v; want nil, false", caps, merged, ok)
			}
		})
	}
}

func TestWithGoozeCapabilitiesWithoutCapabilities(t *testing.T) {
	merged, ok := withGoozeCapabilities(map[string]json.RawMessage{})
	if !ok {
		t.Fatal("withGoozeCapabilities returned false for a result without capabilities")
	}
	if !strings.Contains(string(merged["capabilities"]), `"gooze.run"`) {
		t.Errorf("capabilities = %s, want gooze's commands", merged["capabilities"])
	}
}

func TestUnionStringsAddsEachStringOnce(t *testing.T) {
	got := unionStrings(nil, []string{"a", "a", "b"})
	if strings.Join(got, ",") != "a,b" {
		t.Errorf("unionStrings = %v, want [a b]", got)
	}
}
