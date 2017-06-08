/*
A wrapper on top of the Site struct that caches the list of image URLs in memory. Cache retrieval is via channels as
the cache code runs in a go-routine to allow access from multiple go-routines and we wan't to avoid the "thundering herd"
problem by only letting one process refresh the cache.

It also allows us to update the cache in the backend while serving stale copies until a new cache is built.
*/
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const CACHE_INTERVAL = 10 * time.Second

type CachedSite struct {
	*Site

	KeyCache        atomic.Value
	LastCacheUpdate time.Time

	CacheUpdateMutex sync.Mutex
}

type GetFromCache struct {
	keys []string
	err  error
}

func NewCachedSiteFromSite(s *Site) *CachedSite {
	cs := &CachedSite{s, atomic.Value{}, time.Time{}, sync.Mutex{}}
	<-cs.GetImageKeysFromCache()
	return cs
}

func (cs *CachedSite) GetAllImageUrls() ([]string, error) {
	var imageUrls []string = []string{}

	imageKeys, err := cs.GetAllImageKeys()
	if err != nil {
		fmt.Printf("Unable to get image keys from S3. Error: %s", err.Error())
		return imageUrls, err
	}

	for _, v := range imageKeys {
		imageUrl, err := cs.GetUrlForImage(v)
		if err != nil {
			fmt.Printf("Unable to create URL for key %s. Error: %s", v, err.Error())
			continue
		}
		imageUrls = append(imageUrls, imageUrl)
	}

	return imageUrls, nil
}

func (cs *CachedSite) GetAllImageKeys() ([]string, error) {
	result := <-cs.GetImageKeysFromCache()
	if result.err != nil {
		return nil, result.err
	} else {
		return result.keys, result.err
	}
}

func (cs *CachedSite) NeedsUpdate() bool {
	return time.Now().Sub(cs.LastCacheUpdate) > CACHE_INTERVAL
}

func (cs *CachedSite) GetImageKeysFromCache() chan *GetFromCache {
	cs.CacheUpdateMutex.Lock()

	c := make(chan *GetFromCache)
	go func() {
		var keys []string
		var err error

		if cs.KeyCache.Load() != nil {
			c <- &GetFromCache{cs.KeyCache.Load().([]string), nil}

			if cs.NeedsUpdate() {
				keys, err = cs.Site.GetAllImageKeys()
				if err == nil {
					cs.KeyCache.Store(keys)
					cs.LastCacheUpdate = time.Now()
				}
			}
		} else {
			keys, err = cs.Site.GetAllImageKeys()
			if err == nil {
				cs.KeyCache.Store(keys)
				cs.LastCacheUpdate = time.Now()
			}
			c <- &GetFromCache{keys, err}
		}

		cs.CacheUpdateMutex.Unlock()
	}()

	return c
}
