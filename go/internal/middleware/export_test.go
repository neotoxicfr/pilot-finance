package middleware

import "io"

// SetRandReader replaces the rand reader for testing.
func SetRandReader(r io.Reader) func() {
	old := randReader
	randReader = r
	return func() { randReader = old }
}
