package main

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/go-ini/ini"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"github.com/aws/aws-sdk-go/aws/awserr"
)

const CACHE_INTERVAL = 1 * time.Hour
const ORDERING_YAML_NAME = "ordering.yaml"

type Album struct {
	site *Site

	Path         string
	BucketPrefix string

	AuthUser string
	AuthPass string

	MetaTitle  string
	AlbumTitle string
	Ordering AlbumOrdering

	InIndex bool

	KeyCache        atomic.Value
	OrderingCache        atomic.Value
	LastKeyCacheUpdate time.Time
	LastOrderingCacheUpdate time.Time

	KeyCacheUpdateMutex      sync.Mutex
	AlbumOrderingUpdateMutex sync.Mutex
}

type AlbumOrdering struct {
	Cover             string
	Thumbnails        []string
	Ordering          []string
	negativeCacheThis bool
}

type GetFromKeyCacheResult struct {
	keys []string
	err  error
}

type GetFromOrderingCacheResult struct {
	ordering AlbumOrdering
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

	album.Canonicalize()
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

	album.Canonicalize()
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

func (a *Album) Canonicalize() {
	if a.Path[len(a.Path)-1] != '/' {
		a.Path = a.Path + "/"
	}
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

func (a *Album) GetCoverPhoto() (Renderable, error) {
	//TODO pick this up through the ordering struct
	if photos, err := a.GetOrderedPhotos(); err != nil {
		return nil, err
	} else {
		if len(photos) > 0 {
			return photos[0], nil
		}
	}

	return &ErrorPhoto{}, nil
}

func (a *Album) GetCoverPhotoForTemplate() Renderable {
	if photo, err := a.GetCoverPhoto(); err != nil {
		fmt.Printf("Unable to get cover photo. Error: %s\n", err.Error())
		return &ErrorPhoto{}
	} else {
		return photo
	}
}

func (a *Album) GetThumbnailPhotosForTemplate() []Renderable {
	if photos, err := a.GetOrderedPhotos(); err != nil {
		fmt.Printf("Unable to get thumbnail photos. Error: %s\n", err.Error())
		return nil
	} else {
		//TODO pick this up through the ordering struct
		if len(photos) > 6 {
			return photos[1:6]
		} else if len(photos) > 0 {
			return photos[1:]
		} else {
			return nil
		}
	}
}

//lowest level, gets the list of objects in the bucket and prefix that
//corresponds to the album it is acting on, it's an object with multiple
//fields.
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

//wrapper around the lowest level method to extract out the fields of relevance, namely
//the key of an object, also drops prefixes (i.e: the folder path) from that output.
func (a *Album) GetAllObjectKeysFromBucket() ([]string, error) {
	objects, err := a.GetAllObjects()
	if err != nil {
		return nil, err
	}

	var imageKeys []string
	for _, obj := range objects {
		key := *obj.Key
		if key[len(*obj.Key)-1] != '/' {
			//check for 'folder' name vs actual object - objects end without trailing /
			imageKeys = append(imageKeys, key)
		}
	}

	return imageKeys, nil
}

//highest level, acts on an album to return processed renderable imageurls, here we must also
//filter out any non-renderables and process any other metadata we expect to find.
func (a *Album) GetOrderedPhotos() ([]Renderable, error) {
	var imageUrls []Renderable

	albumOrdering, err := a.GetAlbumOrdering()

	if err != nil {
		fmt.Printf("Unable to pick up album ordering from S3 ordering.yaml because: %s", err.Error())
	}

	fmt.Print(albumOrdering)

	imageKeys, err := a.GetAllObjectKeys()
	if err != nil {
		fmt.Printf("Unable to get object keys from S3. Error: %s\n", err.Error())
		return imageUrls, err
	}

	for _, v := range imageKeys {
		if strings.HasSuffix(v, "ordering.yaml") {
			//for now, just do nothing, we simply want to avoid appending,
			//when we agree on a list of valid formats, we can ditch this check.
		} else {
			imageUrl := a.site.GetPhotoForKey(v)
			imageUrls = append(imageUrls, imageUrl)
		}
	}

	return imageUrls, nil
}

//wrapper around GetAllObjectKeysFromBucket to add in a caching layer, nothing below
//this layer filters or reorders the list of **objects** returned from S3.
//note that this DOES filter out the album prefix.
func (a *Album) GetAllObjectKeys() ([]string, error) {
	c := make(chan *GetFromKeyCacheResult)
	go func() {
		var keys []string
		var err error

		if a.KeyCache.Load() != nil {
			c <- &GetFromKeyCacheResult{a.KeyCache.Load().([]string), nil}

			a.KeyCacheUpdateMutex.Lock()
			if a.NeedsKeyCacheUpdate() {
				keys, err = a.GetAllObjectKeysFromBucket()
				if err == nil {
					a.KeyCache.Store(keys)
					a.LastKeyCacheUpdate = time.Now()
				}
			}

			a.KeyCacheUpdateMutex.Unlock()
		} else {
			a.KeyCacheUpdateMutex.Lock()

			keys, err = a.GetAllObjectKeysFromBucket()
			if err == nil {
				a.KeyCache.Store(keys)
				a.LastKeyCacheUpdate = time.Now()
			}
			c <- &GetFromKeyCacheResult{keys, err}

			a.KeyCacheUpdateMutex.Unlock()
		}
	}()

	result := <-c
	if result.err != nil {
		return nil, result.err
	} else {
		return result.keys, result.err
	}
}

//retrieves the actual album ordering from s3, it expects a file as hard-coded in
// the constant ORDERING_YAML_NAME
func (a *Album) GetAlbumOrderingFromS3() (AlbumOrdering, error) {
	var albumOrdering AlbumOrdering
	svc, err := a.site.GetS3Service()
	if err != nil {
		return albumOrdering, err
	}

	orderingYAMLKey := strings.Join([]string{a.BucketPrefix, ORDERING_YAML_NAME}, "/")
	yaml_object, err := svc.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(a.site.BucketName),
		Key:    aws.String(orderingYAMLKey),
	})

	if err != nil {
		if aerr, ok := err.(awserr.RequestFailure); ok {
			if aerr.StatusCode() == 404 {
				albumOrdering.negativeCacheThis = true
			}
		}
		//basically, we only want to negatively cache 404's, so we can mark this as such.
		//should be retried later, but exception handling is up to the caller.
		return albumOrdering, err
	}

	//extract the contents from what we read so we can then parse the yaml
	data_bytes, err := ioutil.ReadAll(yaml_object.Body)
	//data_string := string(data_bytes)
	err = yaml.Unmarshal(data_bytes, &albumOrdering)

	if err != nil {
		//we were unable to read what the yaml was, it's likely malformed, and that may not change
		//anytime soon, so we negatively cache it, the caller should be aware
		//that it's going to be a bad result though, so raise the error
		albumOrdering.negativeCacheThis = true
		return albumOrdering, err
	}

	return albumOrdering, nil
}

//note that this also caches negative values, i.e: adding a ordering file may take an hour
//to be rechecked.
func (a *Album) GetAlbumOrdering() (AlbumOrdering, error) {
	c := make(chan *GetFromOrderingCacheResult)
	go func() {
		if a.OrderingCache.Load() != nil {
			c <- &GetFromOrderingCacheResult{a.OrderingCache.Load().(AlbumOrdering), nil}

			a.AlbumOrderingUpdateMutex.Lock()
			if a.NeedsOrderingCacheUpdate() {

				albumOrdering, err := a.GetAlbumOrderingFromS3()
				if err == nil || albumOrdering.negativeCacheThis {
					// whether the item is valid or we should be negatively
					// caching this result (probs err!=nil, but the value
					// should be there and a valid boolean.
					a.OrderingCache.Store(albumOrdering)
					a.LastOrderingCacheUpdate = time.Now()
				}

			}
			a.AlbumOrderingUpdateMutex.Unlock()
		} else {
			a.AlbumOrderingUpdateMutex.Lock()
			albumOrdering, err := a.GetAlbumOrderingFromS3()
			if err == nil || albumOrdering.negativeCacheThis {
				// whether the item is valid or we should be negatively
				// caching this result (probs err!=nil, but the value
				// should be there and a valid boolean.
				a.OrderingCache.Store(albumOrdering)
				a.LastOrderingCacheUpdate = time.Now()
			}

			c <- &GetFromOrderingCacheResult{albumOrdering, err}

			a.AlbumOrderingUpdateMutex.Unlock()
		}
	}()

	var albumOrdering AlbumOrdering
	result := <-c
	if result.err != nil {
		//consumer should be checking err
		return albumOrdering, result.err
	} else {
		return result.ordering, result.err
	}
}

func (a *Album) ImageExists(slug string) bool {
	svc, err := a.site.GetS3Service()
	if err != nil {
		return false
	}

	key := strings.Join([]string{a.BucketPrefix, slug}, "/")
	_, err = svc.HeadObject(&s3.HeadObjectInput{
		Bucket: aws.String(a.site.BucketName),
		Key:    aws.String(key),
	})

	if err != nil {
		return false
	}

	return true
}

func (a *Album) NeedsKeyCacheUpdate() bool {
	return time.Now().Sub(a.LastKeyCacheUpdate) > CACHE_INTERVAL
}

func (a *Album) NeedsOrderingCacheUpdate() bool {
	return time.Now().Sub(a.LastOrderingCacheUpdate) > CACHE_INTERVAL
}
