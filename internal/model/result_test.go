package model

import "testing"

func TestSeverityString(t *testing.T) {
	cases := map[Severity]string{
		SeverityError: "error", SeverityWarning: "warning", SeverityInfo: "info", SeverityHint: "hint", 0: "unknown",
	}
	for sev, want := range cases {
		if got := sev.String(); got != want {
			t.Errorf("Severity(%d).String() = %q, want %q", sev, got, want)
		}
	}
}
