//gooze:ignore numbers
package main

//gooze:ignore
func ignored() int {
	return 1 + 2
}

func main() {
	x := 1 + 2 //gooze:ignore arithmetic
	if x > 0 { //gooze:ignore comparison
		println(x)
	}

	// Try typing a mutagen name after the space below to see completion:
	y := x * 2 //gooze:ignore
	_ = y

	// This references an unknown mutagen and should be flagged:
	z := x - 1 //gooze:ignore bogus
	_ = z
}
