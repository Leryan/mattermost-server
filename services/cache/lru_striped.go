// Copyright (c) 2015-present Mattermost, Inc. All Rights Reserved.
// See LICENSE.txt for license information.

package cache

import (
	"hash/maphash"
	"runtime"
	"time"

	"github.com/cespare/xxhash/v2"
)

type LRUStriped struct {
	locks                  []paddedRWMutex
	buckets                []*LRU
	seed                   maphash.Seed
	name                   string
	invalidateClusterEvent string
}

func (L LRUStriped) Purge() error {
	for _, lru := range L.buckets {
		lru.Purge() // errors from purging LRU can be ignored as they always return nil
	}
	return nil
}

func (L LRUStriped) Set(key string, value interface{}) error {
	return L.keyBucket(key).Set(key, value)
}

func (L LRUStriped) SetWithDefaultExpiry(key string, value interface{}) error {
	return L.keyBucket(key).SetWithDefaultExpiry(key, value)
}

func (L LRUStriped) hashkeyMapHash(key string) uint64 {
	/*
		h := &maphash.Hash{}
		h.SetSeed(L.seed)
		if _, err := h.WriteString(key); err != nil {
			panic(err)
		}
		return h.Sum64()
	*/
	return xxhash.Sum64String(key)
}

func (L LRUStriped) keyBucket(key string) *LRU {
	return L.buckets[L.hashkeyMapHash(key)%uint64(len(L.buckets))]
}

func (L LRUStriped) SetWithExpiry(key string, value interface{}, ttl time.Duration) error {
	return L.keyBucket(key).SetWithExpiry(key, value, ttl)
}

func (L LRUStriped) Get(key string, value interface{}) error {
	return L.keyBucket(key).Get(key, value)
}

func (L LRUStriped) Remove(key string) error {
	return L.keyBucket(key).Remove(key)
}

func (L LRUStriped) Keys() ([]string, error) {
	keys := make([]string, 0)
	for _, lru := range L.buckets {
		k, err := lru.Keys()
		if err != nil {
			return nil, err
		}
		keys = append(keys, k...)
	}
	return keys, nil
}

func (L LRUStriped) Len() (int, error) {
	size := 0
	for _, lru := range L.buckets {
		s, err := lru.Len()
		if err != nil {
			return 0, err
		}
		size += s
	}
	return size, nil
}

func (L LRUStriped) GetInvalidateClusterEvent() string {
	return L.invalidateClusterEvent
}

func (L LRUStriped) Name() string {
	return L.name
}

func NewLRUStriped(opts *LRUOptions) Cache {
	if opts.StripedBuckets == 0 {
		opts.StripedBuckets = runtime.NumCPU()
	}
	if opts.Size < opts.StripedBuckets {
		opts.StripedBuckets = opts.Size
	}

	buckets := make([]*LRU, 0, opts.StripedBuckets)
	backupSize := opts.Size
	opts.Size = (opts.Size / opts.StripedBuckets) + (opts.Size % opts.StripedBuckets)

	locks := make([]paddedRWMutex, opts.StripedBuckets)
	for i := 0; i < opts.StripedBuckets; i++ {
		bucket := NewLRU(opts).(*LRU)
		bucket.lock = &locks[i] // not sure we need this, waiting tests on linux
		buckets = append(buckets, bucket)
	}

	opts.Size = backupSize

	return &LRUStriped{
		buckets:                buckets,
		seed:                   maphash.MakeSeed(),
		invalidateClusterEvent: opts.InvalidateClusterEvent,
		name:                   opts.Name,
		locks:                  locks,
	}
}

func NewDefaultLRU(opts *LRUOptions) Cache {
	return NewLRUStriped(opts)
}
