package cache_test

import (
	"sync"
	"testing"
)

const M = 1000000

type cacheMiss struct {
	f1 uint32
	f2 uint32
}

type cacheHit struct {
	f1 uint64
	f2 uint64
}

var out32 uint8
var out64 uint64

type cacheNoPad struct {
	n int
}

type cachePad struct {
	n int
	_ padding
}

type padding struct {
	_ [64]byte
}

func BenchmarkCacheTESTNoPad(b *testing.B) {
	c1 := cacheNoPad{}
	c2 := cacheNoPad{}
	b.ResetTimer()
	wg := sync.WaitGroup{}
	for i := 0; i < b.N; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < M; j++ {
				c1.n += j
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < M; j++ {
				c2.n += j
			}
		}()
		wg.Wait()
	}
}

func BenchmarkCacheTESTPad(b *testing.B) {
	c1 := cachePad{}
	c2 := cachePad{}
	b.ResetTimer()
	wg := sync.WaitGroup{}
	for i := 0; i < b.N; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < M; j++ {
				c1.n += j
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < M; j++ {
				c2.n += j
			}
		}()
		wg.Wait()
	}
}
