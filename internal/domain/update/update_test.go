package update

import "testing"

func TestIsNewer(t *testing.T) {
	tests := []struct {
		candidate, current string
		want               bool
	}{
		{"v0.3.0", "v0.2.0", true},
		{"v1.0.0", "v0.9.9", true},
		{"v0.2.1", "v0.2.0", true},
		{"v0.10.0", "v0.9.0", true},
		{"v0.2.0", "v0.2.0", false},
		{"v0.2.0", "v0.3.0", false},
		{"v0.2.0", "v1.0.0", false},
		{"v0.2.0", "v0.2.1", false},
		{"v0.2.0", "v0.2.0-rc.1", true},
		{"v0.2.0-rc.1", "v0.2.0", false},
		{"v0.2.0-rc.2", "v0.2.0-rc.1", true},
		{"v0.2.0-rc.1", "v0.2.0-rc.1", false},
		// A pseudo-version built after v0.2.0 is not behind v0.2.0...
		{"v0.2.0", "v0.2.1-0.20261020134510-2e8a43a00000+dirty", false},
		// ...but is behind v0.2.1.
		{"v0.2.1", "v0.2.1-0.20261020134510-2e8a43a00000", true},
		{"v0.3.0", "v0.2.0+dirty", true},
		{"", "v0.2.0", false},
		{"v0.3.0", "", false},
		{"v0.3.0", "(devel)", false},
		{"0.3.0", "v0.2.0", false},
		{"v0.3", "v0.2.0", false},
		{"v0.3.0.1", "v0.2.0", false},
		{"v0.x.0", "v0.2.0", false},
		{"v0.-3.0", "v0.2.0", false},
	}

	for _, tt := range tests {
		if got := IsNewer(tt.candidate, tt.current); got != tt.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", tt.candidate, tt.current, got, tt.want)
		}
	}
}

func TestNotice(t *testing.T) {
	got, ok := Notice("v0.2.0", "v0.3.0")
	want := "goozels v0.3.0 is available (you have v0.2.0). Update: go install gooze.dev/pkg/goozels/cmd/goozels@latest"

	if !ok || got != want {
		t.Errorf("Notice = %q, %v; want %q, true", got, ok, want)
	}

	if got, ok := Notice("v0.3.0", "v0.3.0"); ok || got != "" {
		t.Errorf("Notice for an up-to-date build = %q, %v; want none", got, ok)
	}

	if _, ok := Notice("(devel)", "v0.3.0"); ok {
		t.Error("Notice for a devel build should be none")
	}
}
