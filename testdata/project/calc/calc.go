// Package calc is a tiny fixture project for goozels' report tests.
package calc

// Add returns the sum of a and b.
func Add(a, b int) int {
	return a + b
}

// IsPositive reports whether n is greater than zero.
func IsPositive(n int) bool {
	return n > 0
}

// Double returns n times two.
func Double(n int) int {
	return n * 2
}
