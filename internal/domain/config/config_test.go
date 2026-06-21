package config

import "testing"

func TestIsConfigFile(t *testing.T) {
	for _, name := range []string{"gooze.yaml", "gooze.yml", ".gooze.yaml", ".gooze.yml"} {
		if !IsConfigFile(name) {
			t.Errorf("IsConfigFile(%q) = false, want true", name)
		}
	}
	for _, name := range []string{"config.yaml", "gooze.json", "main.go"} {
		if IsConfigFile(name) {
			t.Errorf("IsConfigFile(%q) = true, want false", name)
		}
	}
}

func TestChildrenAt(t *testing.T) {
	if got := ChildrenAt(nil); len(got) != len(Schema) {
		t.Errorf("top-level children = %d, want %d", len(got), len(Schema))
	}
	logChildren := ChildrenAt([]string{"log"})
	if len(logChildren) != 7 {
		t.Errorf("log children = %d, want 7", len(logChildren))
	}
	if ChildrenAt([]string{"output"}) != nil {
		t.Error("scalar field should have no children")
	}
}

func TestLookup(t *testing.T) {
	f, ok := Lookup([]string{"run", "parallel"})
	if !ok || f.Kind != KindInt {
		t.Fatalf("run.parallel lookup failed: ok=%v kind=%v", ok, f.Kind)
	}
	if _, ok := Lookup([]string{"run", "nope"}); ok {
		t.Error("unknown key should not resolve")
	}
}

func TestValidateScalar(t *testing.T) {
	level, _ := Lookup([]string{"log", "level"})
	if msg := level.ValidateScalar("info"); msg != "" {
		t.Errorf("valid enum rejected: %s", msg)
	}
	if msg := level.ValidateScalar("-4"); msg != "" {
		t.Errorf("numeric slog level rejected: %s", msg)
	}
	if msg := level.ValidateScalar("loud"); msg == "" {
		t.Error("invalid enum accepted")
	}

	parallel, _ := Lookup([]string{"run", "parallel"})
	if msg := parallel.ValidateScalar("4"); msg != "" {
		t.Errorf("valid int rejected: %s", msg)
	}
	if msg := parallel.ValidateScalar("lots"); msg == "" {
		t.Error("invalid int accepted")
	}

	noCache, _ := Lookup([]string{"no-cache"})
	if msg := noCache.ValidateScalar("maybe"); msg == "" {
		t.Error("invalid bool accepted")
	}
}
