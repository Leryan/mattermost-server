package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/trace"
	"sync"
	"time"

	cache2 "github.com/mattermost/mattermost-server/v5/services/cache"
)

const M = 10_000_000

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

	wgRead := &sync.WaitGroup{}
	wgWrite := &sync.WaitGroup{}
	set := func(start int) {
		defer wgWrite.Done()
		for i := start; i < M; i++ {
			cache.Set(keys[i], "ignored")
		}
	}

	get := func(start int) {
		defer wgRead.Done()
		var out string
		for i := 0; i < M; i++ {
			cache.Get(keys[(i+start)%10000], &out)
		}
	}

	if err := trace.Start(ftrace); err != nil {
		log.Fatal(err)
	}
	defer trace.Stop()

	start := time.Now()
	if *writeIntensive {
		wgWrite.Add(3)
		wgRead.Add(1)
		fmt.Println("Write intensive")
		go set(0)
		go set(10)
		go set(100)
		go get(0)
	} else {
		wgWrite.Add(1)
		wgRead.Add(3)
		fmt.Println("Read intensive")
		go set(0)
		go get(0)
		go get(10)
		go get(50)
	}

	wg := &sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		wgRead.Wait()
		done := time.Now()
		fmt.Printf("Get finished in %f seconds\n", done.Sub(start).Seconds())
	}()

	go func() {
		defer wg.Done()
		wgWrite.Wait()
		done := time.Now()
		fmt.Printf("Set finished in %f seconds\n", done.Sub(start).Seconds())
	}()

	wg.Wait()

}
