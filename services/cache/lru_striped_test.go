// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package cache

import (
	"fmt"
	"hash/maphash"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/cespare/xxhash/v2"
	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"
)

func makeLRUPredictibleTestData(num int) [][2]string {
	kv := make([][2]string, num)
	for i := 0; i < len(kv); i++ {
		kv[i] = [2]string{
			fmt.Sprintf("%d-key-%d", i, i),
			fmt.Sprintf("%d-val-%d", i, i),
		}
	}
	return kv
}

func makeLRURandomTestData(num int) [][2]string {
	kv := make([][2]string, num)
	for i := 0; i < len(kv); i++ {
		kv[i] = [2]string{
			uuid.New().String(),
			fmt.Sprintf("%d-val-%d", i, i),
		}
	}
	return kv
}

func TestNewLRUStriped(t *testing.T) {
	cache := NewLRUStriped(&LRUOptions{StripedBuckets: 3, Size: 20}).(*LRUStriped)
	require.Len(t, cache.buckets, 3)
	assert.Equal(t, 8, cache.buckets[0].size)
	assert.Equal(t, 8, cache.buckets[1].size)
	assert.Equal(t, 8, cache.buckets[2].size)
}

func TestLRUStriped_DistributionPredictible(t *testing.T) {
	cache := NewLRUStriped(&LRUOptions{StripedBuckets: 4, Size: 10000}).(*LRUStriped)
	kv := makeLRUPredictibleTestData(10000)
	for _, v := range kv {
		require.NoError(t, cache.Set(v[0], v[1]))
	}

	require.Len(t, cache.buckets, 4)
	assert.GreaterOrEqual(t, cache.buckets[0].len, 2300)
	assert.GreaterOrEqual(t, cache.buckets[1].len, 2300)
	assert.GreaterOrEqual(t, cache.buckets[2].len, 2300)
	assert.GreaterOrEqual(t, cache.buckets[3].len, 2300)
}

func TestLRUStriped_DistributionRandom(t *testing.T) {
	cache := NewLRUStriped(&LRUOptions{StripedBuckets: 4, Size: 10000}).(*LRUStriped)
	kv := makeLRURandomTestData(10000)
	for _, v := range kv {
		require.NoError(t, cache.Set(v[0], v[1]))
	}

	require.Len(t, cache.buckets, 4)
	assert.GreaterOrEqual(t, cache.buckets[0].len, 2300)
	assert.GreaterOrEqual(t, cache.buckets[1].len, 2300)
	assert.GreaterOrEqual(t, cache.buckets[2].len, 2300)
	assert.GreaterOrEqual(t, cache.buckets[3].len, 2300)
}

func TestLRUStriped_HashKey(t *testing.T) {
	cache := NewLRUStriped(&LRUOptions{StripedBuckets: 2, Size: 128}).(*LRUStriped)
	first := cache.hashkeyMapHash("key")
	second := cache.hashkeyMapHash("key")
	require.Equal(t, first, second)
}

func TestLRUStriped_Get(t *testing.T) {
	cache := NewLRUStriped(&LRUOptions{StripedBuckets: 4, Size: 128})
	var out string
	require.Equal(t, ErrKeyNotFound, cache.Get("key", &out))
	require.Zero(t, out)

	require.NoError(t, cache.Set("key", "value"))
	require.NoError(t, cache.Get("key", &out))
	require.Equal(t, "value", out)
}

var sum uint64

func BenchmarkMaphashSum64(b *testing.B) {
	seed := (&maphash.Hash{}).Seed()
	for i := 0; i < b.N; i++ {
		var h maphash.Hash
		h.SetSeed(seed)
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

			kv := makeLRUPredictibleTestData(b.N)

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
