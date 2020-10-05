// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package cache

import (
	"fmt"
	"hash/maphash"
	"log"
	"runtime"
	"sync"
	"testing"
	"time"
	"unsafe"

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
	assert.Equal(t, 8, cache.buckets[0].lru.size)
	assert.Equal(t, 8, cache.buckets[1].lru.size)
	assert.Equal(t, 8, cache.buckets[2].lru.size)
}

func TestLRUStriped_DistributionPredictible(t *testing.T) {
	cache := NewLRUStriped(&LRUOptions{StripedBuckets: 4, Size: 10000}).(*LRUStriped)
	kv := makeLRUPredictibleTestData(10000)
	for _, v := range kv {
		require.NoError(t, cache.Set(v[0], v[1]))
	}

	require.Len(t, cache.buckets, 4)
	assert.GreaterOrEqual(t, cache.buckets[0].lru.len, 2300)
	assert.GreaterOrEqual(t, cache.buckets[1].lru.len, 2300)
	assert.GreaterOrEqual(t, cache.buckets[2].lru.len, 2300)
	assert.GreaterOrEqual(t, cache.buckets[3].lru.len, 2300)
}

func TestLRUStriped_DistributionRandom(t *testing.T) {
	cache := NewLRUStriped(&LRUOptions{StripedBuckets: 4, Size: 10000}).(*LRUStriped)
	kv := makeLRURandomTestData(10000)
	for _, v := range kv {
		require.NoError(t, cache.Set(v[0], v[1]))
	}

	require.Len(t, cache.buckets, 4)
	assert.GreaterOrEqual(t, cache.buckets[0].lru.len, 2300)
	assert.GreaterOrEqual(t, cache.buckets[1].lru.len, 2300)
	assert.GreaterOrEqual(t, cache.buckets[2].lru.len, 2300)
	assert.GreaterOrEqual(t, cache.buckets[3].lru.len, 2300)
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

func TestLRUStuff(t *testing.T) {
	log.Println(unsafe.Sizeof(LRU{}))
	log.Println(unsafe.Sizeof(&LRU{}))
	log.Println(unsafe.Sizeof(wraplru{}))
	log.Println(unsafe.Sizeof(LRUStriped{}))
	log.Println(unsafe.Sizeof(&LRUStriped{}))
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

type cacheMakeAndName struct {
	Name string
	Make func(options *LRUOptions) Cache
}
type benchCase struct {
	Size           int
	WriteRoutines  int
	AccessFraction int
	MakeLRU        cacheMakeAndName
	Buckets        int
	Encoder        Encoder
}

type parameters struct {
	Size           []int
	WriteRoutines  []int
	WritePause     []time.Duration
	AccessFraction []int
	MakeLRU        []cacheMakeAndName
	Buckets        []int
}

func generateLRU_Concurrent_Cases(params parameters) []benchCase {
	benchCases := make([]benchCase, 0)
	for _, makelru := range params.MakeLRU {
		for _, buckets := range params.Buckets {
			for _, af := range params.AccessFraction {
				for _, wr := range params.WriteRoutines {
					for _, size := range params.Size {
						benchCases = append(benchCases, benchCase{
							Size:           size,
							WriteRoutines:  wr,
							AccessFraction: af,
							MakeLRU:        makelru,
							Buckets:        buckets,
							Encoder:        NilEncoder{},
						})
					}
				}
			}
		}
	}
	return benchCases
}

func automaticParams() []benchCase {
	paramsStriped := parameters{
		Size:           []int{512, 10000},
		WriteRoutines:  []int{1, runtime.NumCPU() / 2, runtime.NumCPU() - 1, runtime.NumCPU()},
		WritePause:     []time.Duration{0, time.Millisecond * 10},
		AccessFraction: []int{1, 16}, // read all data, read 32 elements
		MakeLRU:        []cacheMakeAndName{{Name: "striped", Make: NewLRUStriped}},
		Buckets:        []int{runtime.NumCPU() / 2, runtime.NumCPU() - 1, runtime.NumCPU()},
	}
	paramsLRU := parameters{
		Size:           []int{512, 10000},
		WriteRoutines:  []int{1, runtime.NumCPU() / 2, runtime.NumCPU() - 1, runtime.NumCPU()},
		WritePause:     []time.Duration{0, time.Millisecond * 10},
		AccessFraction: []int{1, 16}, // read all data, read 32 elements
		MakeLRU:        []cacheMakeAndName{{Name: "lru", Make: NewLRU}},
		Buckets:        []int{1},
	}

	benchCases := generateLRU_Concurrent_Cases(paramsLRU)
	benchCases = append(benchCases, generateLRU_Concurrent_Cases(paramsStriped)...)

	return benchCases
}

func staticParams() []benchCase {
	lru := benchCase{
		Size:           128,
		WriteRoutines:  1,
		AccessFraction: 4,
		MakeLRU:        cacheMakeAndName{Name: "lru", Make: NewLRU},
		Buckets:        2,
		Encoder:        NilEncoder{},
	}
	striped := lru
	striped.Buckets = 2
	striped.MakeLRU = cacheMakeAndName{Name: "str", Make: NewLRUStriped}
	return []benchCase{lru, striped}
}

func BenchmarkLRU_Concurrent(b *testing.B) {
	benchCases := automaticParams()
	benchCases = staticParams()
	for _, benchCase := range benchCases {
		name := fmt.Sprintf("%s_buckets-%d_af-%d_wr-%d_size-%d",
			benchCase.MakeLRU.Name,
			benchCase.Buckets,
			benchCase.AccessFraction,
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
				Encoder:        benchCase.Encoder,
			})

			for i := 0; i < benchCase.Size; i++ {
				if err := cache.Set(fmt.Sprintf("%d-key-%d", i, i), "ignored"); err != nil {
					b.Fatalf("preflight cache set: %v", err)
				}
			}

			if err := cache.Get(fmt.Sprintf("%d-key-%d", benchCase.Size-1, benchCase.Size-1), &out); err != nil {
				b.Fatalf("preflight cache get: %v", err)
			}

			wg := &sync.WaitGroup{}
			set := func(start int) {
				defer wg.Done()
				for i := start; i < b.N; i++ {
					k := i % benchCase.Size
					if err := cache.Set(fmt.Sprintf("%d-key-%d", k, k), "ignored"); err != nil {
						panic(fmt.Sprintf("set error: %v", err)) // pass ci checks, shouldn’t fail anyway.
					}
				}
			}

			for i := 0; i < benchCase.WriteRoutines; i++ {
				wg.Add(1)
				go set(benchCase.Size / ((i + 1) * 2))
			}
			max := benchCase.Size / benchCase.AccessFraction
			keys := make([]string, max)
			for i := 0; i < len(keys); i++ {
				keys[i] = fmt.Sprintf("%d-key-%d", i, i)
			}
			b.StartTimer()
			for i := 0; i < b.N; i++ {
				cache.Get(keys[i%max], &out)
			}
			b.StopTimer()
			wg.Wait()
		})
	}
}
