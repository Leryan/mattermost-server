// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package cache

import (
	"fmt"
	"hash/maphash"
	"runtime"
	"sync"
	"testing"

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
	seed := maphash.MakeSeed()
	b.ResetTimer()
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

func BenchmarkLRU_Concurrent(b *testing.B) {
	type benchCase struct {
		Name          string
		Size          int
		WriteRoutines int
		MakeLRU       func(options *LRUOptions) Cache
		Buckets       int
	}

	benchCases := []benchCase{
		{
			Name:          "warmup-striped",
			Size:          128 * runtime.NumCPU(),
			WriteRoutines: runtime.NumCPU(), // flood cpu for warmup
			MakeLRU:       NewLRUStriped,
		},
		{
			Name:          "lru",
			Size:          128 * (b.N + 1),
			WriteRoutines: runtime.NumCPU() - 1,
			MakeLRU:       NewLRU,
		},
		{
			Name:          "lru",
			Size:          10000,
			WriteRoutines: runtime.NumCPU() - 1,
			MakeLRU:       NewLRU,
		},
		{
			Name:          "lru-noht",
			Size:          10000,
			WriteRoutines: (runtime.NumCPU() / 2) - 1,
			MakeLRU:       NewLRU,
		},
		{
			Name:          "lru",
			Size:          10000,
			WriteRoutines: 1,
			MakeLRU:       NewLRU,
		},
		{
			Name:          "striped",
			Size:          128 * (b.N + 1),
			WriteRoutines: runtime.NumCPU() - 1,
			MakeLRU:       NewLRUStriped,
		},
		{
			Name:          "striped",
			Size:          10000,
			WriteRoutines: runtime.NumCPU() - 1,
			MakeLRU:       NewLRUStriped,
		},
		{
			Name:          "striped-noht",
			Size:          10000,
			WriteRoutines: (runtime.NumCPU() / 2) - 1,
			MakeLRU:       NewLRUStriped,
			Buckets:       (runtime.NumCPU() / 2) - 2,
		},
		{
			Name:          "striped",
			Size:          10000,
			WriteRoutines: 1,
			MakeLRU:       NewLRUStriped,
		},
	}

	for _, benchCase := range benchCases {
		name := fmt.Sprintf("%s__size-%d__routines-%d",
			benchCase.Name,
			benchCase.Size,
			benchCase.WriteRoutines,
		)
		b.Run(name, func(b *testing.B) {
			b.StopTimer()
			b.ResetTimer()
			cache := benchCase.MakeLRU(&LRUOptions{
				StripedBuckets: benchCase.Buckets,
				Size:           benchCase.Size,
				Name:           benchCase.Name,
			})
			stops := make([]chan bool, benchCase.WriteRoutines)

			kv := makeLRUPredictibleTestData(benchCase.Size)

			for i := 0; i < len(kv); i++ {
				if err := cache.Set(kv[i][0], kv[i][1]); err != nil {
					b.Fatalf("preflight cache set: %v", err)
				}
			}

			var out string
			if err := cache.Get(kv[len(kv)-1][0], &out); err != nil {
				b.Fatalf("preflight cache get: %v", err)
			}

			wg := &sync.WaitGroup{}
			set := func(stop <-chan bool, start int) {
				defer wg.Done()
				kv := kv[:]
				for i := start; true; i = (i + 1) % (len(kv)) {
					select {
					case <-stop:
						return
					default:
					}
					if err := cache.Set(kv[i][0], kv[i][1]); err != nil {
						panic(fmt.Sprintf("set error: %v", err)) // pass ci checks, shouldn’t fail anyway.
					}
				}
			}

			for i := 0; i < benchCase.WriteRoutines; i++ {
				stops[i] = make(chan bool)
				wg.Add(1)
				go set(stops[i], benchCase.Size/((i+1)*2))
			}
			b.StartTimer()
			for i := 0; i < b.N; i++ {
				var out string
				cache.Get(kv[i%len(kv)][0], &out)
			}
			b.StopTimer()
			for i := 0; i < benchCase.WriteRoutines; i++ {
				stops[i] <- true
			}
			wg.Wait()
			b.StartTimer()
		})
	}
}
