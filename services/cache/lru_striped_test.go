// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package cache

import (
	"fmt"
	"hash/maphash"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"
)

func TestNewLRUStriped(t *testing.T) {
	cache := NewLRUStriped(&LRUOptions{StripedBuckets: 3, Size: 20}).(*LRUStriped)
	require.Len(t, cache.buckets, 3)
	assert.Equal(t, 6, cache.buckets[0].size)
	assert.Equal(t, 6, cache.buckets[1].size)
	assert.Equal(t, 8, cache.buckets[2].size)
}

var sum uint64

func BenchmarkMaphashSum64(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var h maphash.Hash
		if _, err := h.WriteString(fmt.Sprint("superduperkey")); err != nil {
			panic(err)
		}
		sum = h.Sum64()
	}
}

func BenchmarkXXHashSum64(b *testing.B) {
	for i := 0; i < b.N; i++ {
		sum = xxhash.Sum64String("superduperkey")
	}
}

func BenchmarkLRUStriped_Concurrent(b *testing.B) {
	warmup := NewLRU(&LRUOptions{Size: 128, Name: "warmup"})
	striped := NewLRUStriped(&LRUOptions{Size: 128, Name: "lru-striped"})
	lru := NewLRU(&LRUOptions{Size: 128, Name: "lru"})

	benchCases := []Cache{warmup, striped, lru}

	for _, cache := range benchCases {
		b.Run(cache.Name(), func(b *testing.B) {
			run := int32(0)
			atomic.StoreInt32(&run, 1)
			defer atomic.StoreInt32(&run, 0)

			kv := make([][2]string, b.N)
			for i := 0; i < len(kv); i++ {
				kv[i] = [2]string{
					fmt.Sprintf("%d-key-%d", i, i),
					fmt.Sprintf("%d-val-%d", i, i),
				}
			}

			for _, cache := range []Cache{striped, lru} {
				if err := cache.SetWithExpiry("testkey", "testvalue", time.Hour); err != nil {
					b.Fatalf("preflight check failure: %v", err)
				}
			}

			go func() {
				i := 0
				for atomic.LoadInt32(&run) > 0 {
					if i >= len(kv) {
						i = 0
					}
					if err := cache.SetWithExpiry(kv[i][0], kv[i][1], time.Millisecond*100); err != nil {
						panic(fmt.Sprintf("set error: %v", err)) // pass ci checks, shouldn’t fail anyway.
					}
					i++
				}
			}()

			time.Sleep(time.Second)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var out string
				if err := cache.Get(kv[i][0], &out); err != nil && err != ErrKeyNotFound {
					b.Fatalf("get error: %v", err)
				}
			}
		})
		time.Sleep(time.Second)
	}
}
