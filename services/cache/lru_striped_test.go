// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package cache

import (
	"fmt"
	"hash/maphash"
	"runtime"
	"sync"
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

var out string

func BenchmarkLRU_Concurrent(b *testing.B) {
	type cacheMakeAndName struct {
		Name string
		Make func(options *LRUOptions) Cache
	}
	type benchCase struct {
		Size           int
		WriteRoutines  int
		WritePause     time.Duration
		AccessFraction int
		MakeLRU        cacheMakeAndName
		Buckets        int
	}

	type matrix struct {
		Size           []int
		WriteRoutines  []int
		WritePause     []time.Duration
		AccessFraction []int
		MakeLRU        []cacheMakeAndName
		Buckets        []int
	}

	parameters := matrix{
		Size:           []int{512, 10000},
		WriteRoutines:  []int{1, runtime.NumCPU() / 2, runtime.NumCPU() - 1, runtime.NumCPU()},
		WritePause:     []time.Duration{time.Millisecond * 10},
		AccessFraction: []int{1, 16, 128}, // read all data, read 32 elements, read 4 elements
		MakeLRU:        []cacheMakeAndName{{Name: "lru", Make: NewLRU}, {Name: "striped", Make: NewLRUStriped}},
		Buckets:        []int{1, runtime.NumCPU() / 2, runtime.NumCPU() - 1, runtime.NumCPU()},
	}

	benchCases := make([]benchCase, 0)

	for _, makelru := range parameters.MakeLRU {
		for _, buckets := range parameters.Buckets {
			for _, af := range parameters.AccessFraction {
				for _, wp := range parameters.WritePause {
					for _, wr := range parameters.WriteRoutines {
						for _, size := range parameters.Size {
							benchCases = append(benchCases, benchCase{
								Size:           size,
								WriteRoutines:  wr,
								WritePause:     wp,
								AccessFraction: af,
								MakeLRU:        makelru,
								Buckets:        buckets,
							})
						}
					}
				}
			}
		}
	}

	for _, benchCase := range benchCases {
		name := fmt.Sprintf("%s_buckets-%d_af-%d_wp-%v_wr-%d_size-%d",
			benchCase.MakeLRU.Name,
			benchCase.Buckets,
			benchCase.AccessFraction,
			benchCase.WritePause,
			benchCase.WriteRoutines,
			benchCase.Size,
		)
		b.Run(name, func(b *testing.B) {
			b.StopTimer()
			b.ResetTimer()
			cache := benchCase.MakeLRU.Make(&LRUOptions{
				StripedBuckets: benchCase.Buckets,
				Size:           benchCase.Size,
				Name:           benchCase.MakeLRU.Name,
			})
			stops := make([]chan bool, benchCase.WriteRoutines)

			kv := makeLRUPredictibleTestData(benchCase.Size)

			for i := 0; i < len(kv); i++ {
				if err := cache.Set(kv[i][0], kv[i][1]); err != nil {
					b.Fatalf("preflight cache set: %v", err)
				}
			}

			if err := cache.Get(kv[len(kv)-1][0], &out); err != nil {
				b.Fatalf("preflight cache get: %v", err)
			}

			wg := &sync.WaitGroup{}
			set := func(stop <-chan bool, start int) {
				defer wg.Done()
				kvc := make([][2]string, len(kv))
				copy(kvc, kv)
				pause := benchCase.WritePause
				for i := start; true; i = (i + 1) % (len(kvc)) {
					select {
					case <-stop:
						return
					default:
					}
					if err := cache.Set(kvc[i][0], kvc[i][1]); err != nil {
						panic(fmt.Sprintf("set error: %v", err)) // pass ci checks, shouldn’t fail anyway.
					}
					time.Sleep(pause)
				}
			}

			for i := 0; i < benchCase.WriteRoutines; i++ {
				stops[i] = make(chan bool)
				wg.Add(1)
				go set(stops[i], benchCase.Size/((i+1)*2))
			}
			b.StartTimer()
			max := len(kv) / benchCase.AccessFraction
			for i := 0; i < b.N; i++ {
				cache.Get(kv[i%max][0], &out)
			}
			b.StopTimer()
			for i := 0; i < benchCase.WriteRoutines; i++ {
				stops[i] <- true
			}
			wg.Wait()
		})
	}
}
