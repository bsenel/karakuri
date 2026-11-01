// Package coverfixture is the package the coverage verifier's test measures:
// Sign has four statements and its test reaches two of them, so its coverage
// is exactly 50 percent.
package coverfixture

// Sign returns -1 for a negative n and 1 otherwise.
func Sign(n int) int {
	if n < 0 {
		neg := -1
		return neg
	}
	return 1
}
