package main

import (
	"errors"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/go-ini/ini"
)

const CACHE_INTERVAL = 1 * time.Hour

type Album struct {
	site *Site

	Path         string
	BucketPrefix string

	AuthUser string
	AuthPass string

	MetaTitle  string
	AlbumTitle string

	InIndex bool

	KeyCache        atomic.Value
	LastCacheUpdate time.Time

	CacheUpdateMutex sync.Mutex
}

type GetFromCacheResult struct {
	keys []string
	err  error
}

func NewAlbumFromConfig(section *ini.Section, s *Site) (*Album, error) {
	album := &Album{site: s, InIndex: true}
	if err := section.MapTo(album); err != nil {
		return nil, err
	}

	if err := album.IsValid(); err != nil {
		return nil, err
	}

	return album, nil
}

func NewAlbum(s *Site, path string, bucketPrefix string, authUser string, authPass string, metaTitle string, albumTitle string) (*Album, error) {
	album := &Album{
		site:         s,
		Path:         path,
		BucketPrefix: bucketPrefix,
		AuthUser:     authUser,
		AuthPass:     authPass,
		MetaTitle:    metaTitle,
		AlbumTitle:   albumTitle,
		InIndex:      true,
	}

	if err := album.IsValid(); err != nil {
		return nil, err
	}

	return album, nil
}

func (a *Album) IsValid() error {
	if a.Path == "" {
		return errors.New("'Path' is a required parameters that must have a valid value.")
	}

	if a.InIndex && a.HasOwnAuth() {
		return errors.New("An album that requires authentication can't be shown in the index. If you need authentication please add it to the site.")
	}
	return nil
}

func (a *Album) HasOwnAuth() bool {
	return a.AuthUser != "" && a.AuthPass != ""
}

// An album inherits it's sites auth settings if the album config doesn't override them. If both the site and album have
// auth enabled, the album auth takes precedence
func (a *Album) HasAuth() bool {
	return a.site.HasAuth() || a.HasOwnAuth()
}

func (a *Album) GetAuthUser() string {
	if a.AuthUser != "" {
		return a.AuthUser
	} else {
		return a.site.AuthUser
	}
}

func (a *Album) GetAuthPass() string {
	if a.AuthPass != "" {
		return a.AuthPass
	} else {
		return a.site.AuthPass
	}
}

func (a *Album) GetCanonicalUrl() *url.URL {
	u := a.site.GetCanonicalUrl()
	u.Path = a.Path
	return u
}

func (a *Album) GetCoverPhotoUrl() (string, error) {
	if photoUrls, err := a.GetAllImageUrls(); err != nil {
		return "", err
	} else {
		if len(photoUrls) > 0 {
			return photoUrls[0], nil
		}
	}

	return "", nil
}

func (a *Album) GetAllObjects() ([]*s3.Object, error) {
	svc, err := a.site.GetS3Service()
	if err != nil {
		return nil, err
	}

	objects, err := svc.ListObjects(&s3.ListObjectsInput{
		Bucket:    aws.String(a.site.BucketName),
		Prefix:    aws.String(a.BucketPrefix),
		Delimiter: aws.String("/"),
	})
	if err != nil {
		return nil, err
	}

	return objects.Contents, nil
}

func (a *Album) GetAllImageKeysFromBucket() ([]string, error) {
	objects, err := a.GetAllObjects()
	if err != nil {
		return nil, err
	}

	var imageKeys []string
	for _, obj := range objects {
		key := *obj.Key
		if key[len(*obj.Key)-1] != '/' {
			imageKeys = append(imageKeys, key)
		}
	}

	return imageKeys, nil
}

func (a *Album) GetAllImageUrls() ([]string, error) {
	var imageUrls []string = []string{}

	imageKeys, err := a.GetAllImageKeys()
	if err != nil {
		fmt.Printf("Unable to get image keys from S3. Error: %s\n", err.Error())
		return imageUrls, err
	}

	for _, v := range imageKeys {
		imageUrl, err := a.site.GetUrlForImage(v)
		if err != nil {
			fmt.Printf("Unable to create URL for key %s. Error: %s\n", v, err.Error())
			continue
		}
		imageUrls = append(imageUrls, imageUrl)
	}

	return imageUrls, nil
}

func (a *Album) GetAllImageKeys() ([]string, error) {
	c := make(chan *GetFromCacheResult)
	go func() {
		var keys []string
		var err error

		if a.KeyCache.Load() != nil {
			c <- &GetFromCacheResult{a.KeyCache.Load().([]string), nil}

			a.CacheUpdateMutex.Lock()
			if a.NeedsUpdate() {
				keys, err = a.GetAllImageKeysFromBucket()
				if err == nil {
					a.KeyCache.Store(keys)
					a.LastCacheUpdate = time.Now()
				}
			}

			a.CacheUpdateMutex.Unlock()
		} else {
			a.CacheUpdateMutex.Lock()

			keys, err = a.GetAllImageKeysFromBucket()
			if err == nil {
				a.KeyCache.Store(keys)
				a.LastCacheUpdate = time.Now()
			}
			c <- &GetFromCacheResult{keys, err}

			a.CacheUpdateMutex.Unlock()
		}
	}()

	result := <-c
	if result.err != nil {
		return nil, result.err
	} else {
		return result.keys, result.err
	}
}

func (a *Album) NeedsUpdate() bool {
	return time.Now().Sub(a.LastCacheUpdate) > CACHE_INTERVAL
}
