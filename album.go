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

	InIndex bool

	KeyCache                           atomic.Value
	OrderingCache                      atomic.Value
	LastKeyCacheUpdate                 time.Time
	LastAlbumOrderingConfigCacheUpdate time.Time

	KeyCacheUpdateMutex                 sync.Mutex
	AlbumAlbumOrderingConfigUpdateMutex sync.Mutex
}

//this struct will store the _configuration_ as read from a yaml file
type AlbumOrderingConfig struct {
	Cover             string
	Thumbnails        []string
	Ordering          []string
	negativeCacheThis bool
}

//this struct will store our actual renderable orderings, as processed
//by reading the config, the actual file index, and doing some merging
type AlbumOrdering struct {
	Cover             Renderable
	Thumbnails        []Renderable
	Ordering          []Renderable
}

type GetFromKeyCacheResult struct {
	keys []string
	err  error
}

type GetFromOrderingCacheResult struct {
	albumOrderingConfig AlbumOrderingConfig
	err                 error
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

func mergeList(bucketKeys []string, configKeys []string, album_name string) []string {
	var mergedKeys []string

	//set up map for faster searching of set existence in the config keys
	bucketMembership := make(map[string]bool)
	for _, v := range bucketKeys {
		bucketMembership[strings.TrimLeft(v, "/")] = true
	}

	for _, configKey := range configKeys {
		// keys in the config come first, silently drop non-existents
		if bucketMembership[strings.TrimLeft(configKey, "/")] {
			mergedKeys = append(mergedKeys, configKey)
		} else {
			fmt.Printf("\nCould not find ordering-specified image %s in album %s", configKey, album_name)
		}
	}

	//now set up the membership map for the just-made mergedKeys
	//set up map for faster searching of set existence in the merged keys
	mergedMembership := make(map[string]bool)
	for _, v := range mergedKeys {
		mergedMembership[strings.TrimLeft(v, "/")] = true
	}

	for _, bucketKey := range bucketKeys {
		// all keys not yet processed previously (by config) are appended
		// makes sure that keys seen before do not re-appear.
		if !mergedMembership[strings.TrimLeft(bucketKey, "/")] {
			mergedKeys = append(mergedKeys, bucketKey)
		}
	}
	return mergedKeys
}

func (a *Album) GetCoverPhoto() (Renderable, error) {
	albumOrdering, err := a.GetOrderedPhotos()
	return albumOrdering.Cover, err
}

func (a *Album) GetCoverPhotoForTemplate() Renderable {
	cover, _ := a.GetCoverPhoto()
	return cover
}

func (a *Album) GetThumbnailPhotosForTemplate() []Renderable {
	albumOrdering, _ := a.GetOrderedPhotos()
	return albumOrdering.Thumbnails
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
func (a *Album) GetOrderedPhotos() (AlbumOrdering, error) {

	//TODO cache this, probably in config.
	var albumOrdering AlbumOrdering

	//pick up our configuration, note that this may be all empties if there's an err in retrieval/parsing.
	albumOrderingConfig, err := a.GetAlbumOrderingConfig()

	if err != nil {
		if aerr, ok := err.(awserr.RequestFailure); ok {
			if aerr.StatusCode() != 404 {
				//regular 404's add too much noise, we couldn't find the file we shouldn't say anything.
				fmt.Printf("\nUnable to pick up album ordering for album %s from S3, Error: %s", a.Path, err.Error())
			}
		}
	}

	// pick up the raw keys, ready for comparison to our configuration
	imageKeys, err := a.GetAllObjectKeys()

	if err != nil {
		fmt.Printf("\nUnable to get object keys from S3 for album %s. Error: %s", a.Path, err.Error())
		//note albumOrdering would be empty, error checking matters!
		return albumOrdering, err
	}

	var cleanImageKeys []string
	//clean out the keys, we don't want the yaml interfering with the yaml :P
	for _, v := range imageKeys {
		if strings.HasSuffix(v, ORDERING_YAML_NAME) {
			//for now, just do nothing, we simply want to avoid appending,
			//when we agree on a list of valid formats, we can ditch this check.
		} else {
			cleanImageKeys = append(cleanImageKeys, v)
		}
	}

	//okay, now we're ready for processing and merging.
	//some ground rules:
	//0) if there is no ordering file, or an error retrieving/parsing the file, everything must work as
	//   if there was never an ordering file in the first place.
	//1) if an image is in the config but not in the bucket, it's silently dropped
	//2) ordering of keys in config come before ordering of keys in bucket (i.e: listed in config THEN non-listed)
	//3) each section is independent and optional but has some interlinkages, (this gets difficult because you
	//   don't want to pick out the same photo for both cover and thumbnail.

	//let's start with the cover photo, there's only one, this should be easy.

	if albumOrderingConfig.Cover != "" {

		// not the most efficient way of checking for existence, but it's one off.
		var coverKeyInBucket = false
		for _, bucketKey := range cleanImageKeys {
			if strings.TrimLeft(bucketKey, "/") == strings.TrimLeft(albumOrderingConfig.Cover, "/") {
				coverKeyInBucket = true
				break
			}
		}

		if coverKeyInBucket {
			albumOrdering.Cover = a.site.GetPhotoForKey(albumOrderingConfig.Cover)
		} else {
			fmt.Printf("\ncover photo specified in ordering file not found in bucket, check %s exists. "+
				"Falling back to first photo", albumOrderingConfig.Cover)
			albumOrdering.Cover = a.site.GetPhotoForKey(cleanImageKeys[0])
		}
	} else {
		albumOrdering.Cover = a.site.GetPhotoForKey(cleanImageKeys[0])
	}

	//thumbnails - there is a bit of duplicate code here, but it was clearer to do it
	//this way rather than to reduce code and be opaque
	var thumbKeys []string
	if len(albumOrderingConfig.Thumbnails) > 0 {
		thumbKeys = mergeList(cleanImageKeys, albumOrderingConfig.Thumbnails, a.Path)
		if len(thumbKeys) > 5 {
			thumbKeys = thumbKeys[0:5]
		} else if len(thumbKeys) > 0 {
			thumbKeys = thumbKeys[0:]
		}
	} else {
		//take care of the offset here (i.e: cover is index 0 if we're not using the config)
		thumbKeys = make([]string, len(cleanImageKeys))
		copy(thumbKeys, cleanImageKeys)
		if len(thumbKeys) > 6 {
			thumbKeys = thumbKeys[1:6]
		} else if len(thumbKeys) > 1 {
			thumbKeys = thumbKeys[1:]
		}
	}

	for _, v := range thumbKeys {
		albumOrdering.Thumbnails = append(albumOrdering.Thumbnails, a.site.GetPhotoForKey(v))
	}

	//the actual album ordering
	mergedOrdering := mergeList(cleanImageKeys, albumOrderingConfig.Ordering, a.Path)
	for _, v := range mergedOrdering {
		albumOrdering.Ordering = append(albumOrdering.Ordering, a.site.GetPhotoForKey(v))
	}

	return albumOrdering, nil
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
// we do a bit of preprocessing in order to take images from relative to a bucket in
// to being absolute in the bucket (in that, in order to compare keys, we have bucket-name/image.jpg
// instead of just image.jpg in the orderings/definitions. Since the config is per-bucket, we'll do that at
// the lowest level in order to avoid confusion/difficulty later. (i.e: consistent from inception at the
// cost of hiding a bit of reality)
func (a *Album) GetAlbumOrderingConfigFromS3AndPreprocess() (AlbumOrderingConfig, error) {
	var albumOrdering AlbumOrderingConfig
	svc, err := a.site.GetS3Service()
	if err != nil {
		return albumOrdering, err
	}

	orderingYAMLKey := strings.Join([]string{a.BucketPrefix, ORDERING_YAML_NAME}, "")
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
		fmt.Printf("\nCould not parse yaml for album %s, it's likely malformed. error: %s", a.Path, err)
		albumOrdering.negativeCacheThis = true
		return albumOrdering, err
	}

	//we want to prepend the album path to every supported key, this is simply for later consistency.
	if albumOrdering.Cover != "" {
		parsedAlbumPrefix, _ := url.Parse(a.Path)
		parsedCoverKey, _ := url.Parse(albumOrdering.Cover)

		fullPath := parsedAlbumPrefix.ResolveReference(parsedCoverKey).String()
		albumOrdering.Cover = strings.TrimLeft(fullPath, "/")
	}

	//cool, now let's do the same for thumbnails
	if len(albumOrdering.Thumbnails) > 0 {
		for index, v := range albumOrdering.Thumbnails {
			parsedAlbumPrefix, _ := url.Parse(a.Path)
			parsedCoverKey, _ := url.Parse(v)

			fullPath := parsedAlbumPrefix.ResolveReference(parsedCoverKey).String()
			albumOrdering.Thumbnails[index] = strings.TrimLeft(fullPath, "/")
		}
	}

	//and finally, for the overall order.
	if len(albumOrdering.Ordering) > 0 {
		for index, v := range albumOrdering.Ordering {
			parsedAlbumPrefix, _ := url.Parse(a.Path)
			parsedCoverKey, _ := url.Parse(v)

			fullPath := parsedAlbumPrefix.ResolveReference(parsedCoverKey).String()
			albumOrdering.Ordering[index] = strings.TrimLeft(fullPath, "/")
		}
	}

	return albumOrdering, nil
}

//note that this also caches negative values, i.e: adding a ordering file may take an hour
//to be rechecked.
func (a *Album) GetAlbumOrderingConfig() (AlbumOrderingConfig, error) {
	c := make(chan *GetFromOrderingCacheResult)
	go func() {
		if a.OrderingCache.Load() != nil {
			c <- &GetFromOrderingCacheResult{a.OrderingCache.Load().(AlbumOrderingConfig), nil}

			a.AlbumAlbumOrderingConfigUpdateMutex.Lock()
			if a.NeedsOrderingCacheUpdate() {

				albumOrdering, err := a.GetAlbumOrderingConfigFromS3AndPreprocess()
				if err == nil || albumOrdering.negativeCacheThis {
					// whether the item is valid or we should be negatively
					// caching this result (probs err!=nil, but the value
					// should be there and a valid boolean.
					a.OrderingCache.Store(albumOrdering)
					a.LastAlbumOrderingConfigCacheUpdate = time.Now()
				}

			}
			a.AlbumAlbumOrderingConfigUpdateMutex.Unlock()
		} else {
			a.AlbumAlbumOrderingConfigUpdateMutex.Lock()
			albumOrdering, err := a.GetAlbumOrderingConfigFromS3AndPreprocess()
			if err == nil || albumOrdering.negativeCacheThis {
				// whether the item is valid or we should be negatively
				// caching this result (probs err!=nil, but the value
				// should be there and a valid boolean.
				a.OrderingCache.Store(albumOrdering)
				a.LastAlbumOrderingConfigCacheUpdate = time.Now()
			}

			c <- &GetFromOrderingCacheResult{albumOrdering, err}

			a.AlbumAlbumOrderingConfigUpdateMutex.Unlock()
		}
	}()

	var albumOrdering AlbumOrderingConfig
	result := <-c
	if result.err != nil {
		//consumer should be checking err
		return albumOrdering, result.err
	} else {
		return result.albumOrderingConfig, result.err
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
	return time.Now().Sub(a.LastAlbumOrderingConfigCacheUpdate) > CACHE_INTERVAL
}
