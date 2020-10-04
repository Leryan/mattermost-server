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
	buckets []*LRU
	opts    *LRUOptions
}

func (L *LRUStriped) Purge() error {
	for _, bucket := range L.buckets {
		bucket.Purge() // errors from purging LRU can be ignored as they always return nil
	}
	return nil
}

func (L *LRUStriped) Set(key string, value interface{}) error {
	return L.SetWithExpiry(key, value, 0)
}

func (L *LRUStriped) SetWithDefaultExpiry(key string, value interface{}) error {
	return L.SetWithExpiry(key, value, L.opts.DefaultExpiry)
}

func (L *LRUStriped) hashkeyMapHash(key string) uint64 {
	var h maphash.Hash
	if _, err := h.WriteString(key); err != nil {
		panic(err)
	}
	return h.Sum64()
}

func (L *LRUStriped) hashkeyXXHash(key string) uint64 {
	return xxhash.Sum64String(key) // seems to be equivalent with maphash
}

func (L *LRUStriped) keyBucket(key string) *LRU {
	return L.buckets[L.hashkeyMapHash(key)%uint64(len(L.buckets))]
}

func (L *LRUStriped) SetWithExpiry(key string, value interface{}, ttl time.Duration) error {
	return L.keyBucket(key).SetWithExpiry(key, value, ttl)
}

func (L *LRUStriped) Get(key string, value interface{}) error {
	return L.keyBucket(key).Get(key, value)
}

func (L *LRUStriped) Remove(key string) error {
	return L.keyBucket(key).Remove(key)
}

func (L *LRUStriped) Keys() ([]string, error) {
	keys := make([]string, 0)
	for _, bucket := range L.buckets {
		k, err := bucket.Keys()
		if err != nil {
			return nil, err
		}
		keys = append(keys, k...)
	}
	return keys, nil
}

func (L *LRUStriped) Len() (int, error) {
	size := 0
	for _, bucket := range L.buckets {
		s, err := bucket.Len()
		if err != nil {
			return 0, err
		}
		size += s
	}
	return size, nil
}

func (L *LRUStriped) GetInvalidateClusterEvent() string {
	return L.opts.InvalidateClusterEvent
}

func (L *LRUStriped) Name() string {
	return L.opts.Name
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
	baseSize := opts.Size / opts.StripedBuckets
	lastSize := baseSize + (opts.Size % opts.StripedBuckets)
	opts.Size = baseSize
	for i := 0; i < opts.StripedBuckets; i++ {
		if i == opts.StripedBuckets-1 {
			opts.Size = lastSize
		}
		buckets = append(buckets, NewLRU(opts).(*LRU))
	}

	opts.Size = backupSize
	return &LRUStriped{buckets: buckets, opts: opts}
}

func NewDefaultLRU(opts *LRUOptions) Cache {
	return NewLRUStriped(opts)
}
