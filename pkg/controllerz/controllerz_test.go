package controllerz

import "testing"

var pi *int

func noIf(i int) int {
	return i + 1
}

func withIf(i int) int {
	if pi == nil {
		panic("bad")
	}
	return i + 1
}

func BenchmarkAndy(b *testing.B) {
	one := 1
	pi = &one
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		withIf(i)
	}
}
