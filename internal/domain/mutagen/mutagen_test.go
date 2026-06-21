package mutagen

import "testing"

func TestRegistryComplete(t *testing.T) {
	want := []string{"arithmetic", "boolean", "numbers", "comparison", "logical", "unary", "branch", "statement", "loop"}
	got := Names()
	if len(got) != len(want) {
		t.Fatalf("registry has %d mutagens, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("mutagen[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLookupCaseInsensitive(t *testing.T) {
	for _, name := range []string{"arithmetic", "ARITHMETIC", " Arithmetic "} {
		if !IsValid(name) {
			t.Errorf("IsValid(%q) = false, want true", name)
		}
	}
	if IsValid("nonsense") {
		t.Error("IsValid(nonsense) = true, want false")
	}
	mg, ok := Lookup("BOOLEAN")
	if !ok || mg.Name != "boolean" {
		t.Errorf("Lookup(BOOLEAN) = %+v, %v", mg, ok)
	}
}
