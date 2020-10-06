package main

import (
	"flag"
	"fmt"
	"os"
	"runtime/trace"
	"sync"

	cache2 "github.com/mattermost/mattermost-server/v5/services/cache"
)

const M = 500_000

func lru(opts *cache2.LRUOptions) cache2.Cache {
	opts.Name = "lru"
	return cache2.NewLRU(opts)
}

func striped(opts *cache2.LRUOptions) cache2.Cache {
	opts.Name = "str"
	return cache2.NewLRUStriped(opts)
}

func main() {
	cacheType := flag.String("type", "lru", "lru or str")
	flag.Parse()

	opts := &cache2.LRUOptions{
		Name:                   "",
		Size:                   128,
		DefaultExpiry:          0,
		InvalidateClusterEvent: "",
		StripedBuckets:         3,
		Encoder:                cache2.DefaultEncoder{},
	}

	var cache cache2.Cache
	if *cacheType == "lru" {
		cache = lru(opts)
	} else {
		cache = striped(opts)
	}

	keys := make([]string, 0, M)
	for i := 0; i < M; i++ {
		keys = append(keys, fmt.Sprintf("%d-key-%d", i, i))
	}
	wg := &sync.WaitGroup{}
	set := func(start int) {
		defer wg.Done()
		for i := start; i < M; i++ {
			cache.Set(keys[i], "ignored")
		}
	}

	trace.Start(os.Stderr)
	defer trace.Stop()

	wg.Add(4)
	go set(0)
	go set(100)
	go set(1000)

	go func() {
		defer wg.Done()
		var out string
		for i := 0; i < M; i++ {
			cache.Get(keys[i%10000], &out)
		}
	}()

	wg.Wait()
}
