package main

import "testing"

func TestEnvOr(t *testing.T) {
	const key = "GOOZELS_TEST_ENVOR"
	t.Setenv(key, "set-value")
	if got := envOr(key, "fallback"); got != "set-value" {
		t.Errorf("envOr with set var = %q, want set-value", got)
	}
	t.Setenv(key, "")
	if got := envOr(key, "fallback"); got != "fallback" {
		t.Errorf("envOr with empty var = %q, want fallback", got)
	}
}
