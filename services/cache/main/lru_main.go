package main

import (
	"flag"
	"fmt"
	"log"
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
	writeIntensive := flag.Bool("writeIntensive", false, "true for 3 get and 1 set, false for the opposite")
	traceOut := flag.String("trace", "trace.out", "trace output path")
	flag.Parse()

	ftrace, err := os.OpenFile(*traceOut, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
	if err != nil {
		log.Fatal(err)
	}
	defer ftrace.Close()

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
		fmt.Println("Using LRU cache")
		cache = lru(opts)
	} else {
		fmt.Println("Using Striped LRU cache")
		cache = striped(opts)
	}

	// prepare keys
	keys := make([]string, 0, M)
	for i := 0; i < M; i++ {
		keys = append(keys, fmt.Sprintf("%d-key-%d", i, i))
		cache.Set(keys[i%128], "preflight")
	}

	wg := &sync.WaitGroup{}
	set := func(start int) {
		defer wg.Done()
		for i := start; i < M; i++ {
			cache.Set(keys[i], "ignored")
		}
	}

	get := func(start int) {
		defer wg.Done()
		var out string
		for i := 0; i < M; i++ {
			cache.Get(keys[i%10000], &out)
		}
	}

	if err := trace.Start(ftrace); err != nil {
		log.Fatal(err)
	}
	defer trace.Stop()

	wg.Add(4)
	if *writeIntensive {
		fmt.Println("Write intensive")
		go set(0)
		go set(10)
		go set(100)
		go get(1000)
	} else {
		fmt.Println("Read intensive")
		go set(0)
		go get(0)
		go get(10)
		go get(50)
	}
	wg.Wait()
}
