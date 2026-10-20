package calc

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Fatal("Add(2, 3) != 5")
	}
}

func TestIsPositive(t *testing.T) {
	if !IsPositive(5) {
		t.Fatal("IsPositive(5) = false")
	}
}
