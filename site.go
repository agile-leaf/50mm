package main

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/go-ini/ini"
)

type Site struct {
	Domain          string
	CanonicalSecure bool

	AuthUser string
	AuthPass string

	S3Host       string
	BucketRegion string
	BucketName   string

	UseImgix bool
	BaseUrl  string

	AWS_SECRET_KEY_ID string `ini:"AWSKeyId"`
	AWS_SECRET_KEY    string `ini:"AWSKey"`

	SiteTitle string
	MetaTitle string

	HasAlbumIndex bool
	Albums        []*Album

	awsSession *session.Session
}

func LoadSiteFromFile(path string) (*Site, error) {
	cfg, err := ini.Load(path)
	if err != nil {
		return nil, err
	}

	defaultSection, err := cfg.GetSection("")
	if err != nil {
		return nil, err
	}

	s := &Site{}
	if err := defaultSection.MapTo(s); err != nil {
		return nil, err
	}

	if s.BucketRegion == "" && s.BucketName == "" {
		s.BucketRegion = defaultSection.Key("Region").String()
		s.BucketName = defaultSection.Key("Bucket").String()
	}

	for _, section := range cfg.Sections() {
		if section.Name() == "DEFAULT" {
			continue
		}

		if album, err := NewAlbumFromConfig(section, s); err != nil {
			return nil, err
		} else {
			s.Albums = append(s.Albums, album)
		}
	}

	if len(s.Albums) == 0 { // Check if the config is old style and create default album
		if album, err := NewAlbum(s, "/", defaultSection.Key("Prefix").String(),
			defaultSection.Key("AuthUser").String(), defaultSection.Key("AuthPass").String(),
			defaultSection.Key("MetaTitle").String(), defaultSection.Key("AlbumTitle").String()); err != nil {
			return nil, err
		} else {
			s.Albums = append(s.Albums, album)
		}
	}

	if err := s.IsValid(); err != nil {
		return nil, err
	}

	sess_config := &aws.Config{
		Region:      aws.String(s.BucketRegion),
		Credentials: credentials.NewStaticCredentials(s.AWS_SECRET_KEY_ID, s.AWS_SECRET_KEY, ""),
	}
	if s.S3Host != "" {
		sess_config.Endpoint = aws.String(s.S3Host)
	}

	if sess, err := session.NewSession(sess_config); err != nil {
		return nil, err
	} else {
		s.awsSession = sess
	}

	if err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Site) IsValid() error {
	if s.Domain == "" || s.BucketRegion == "" || s.BucketName == "" || s.AWS_SECRET_KEY_ID == "" || s.AWS_SECRET_KEY == "" {
		return errors.New("Domain, BucketRegion, BucketName, AWSKeyId, and AWSKey are required parameters that must have valid values")
	}

	if len(s.Albums) == 0 {
		return errors.New("Can't have a site with 0 albums")
	}

	if s.HasAlbumIndex {
		for _, a := range s.Albums {
			if a.Path == "/" {
				return errors.New("Site can't have an index and an album at path '/'")
			}
		}
	}

	return nil
}

func (s *Site) HasAuth() bool {
	return s.AuthUser != "" && s.AuthPass != ""
}

func (s *Site) GetAuthUser() string {
	return s.AuthUser
}

func (s *Site) GetAuthPass() string {
	return s.AuthPass
}

func (s *Site) GetCanonicalUrl() *url.URL {
	proto, domain := "http", s.Domain
	if s.CanonicalSecure {
		proto = "https"
	}

	return &url.URL{
		Scheme: proto,
		Host:   domain,
	}
}

func (s *Site) GetAlbumsForIndex() []*Album {
	indexAlbums := make([]*Album, 0)

	for _, a := range s.Albums {
		if a.InIndex {
			indexAlbums = append(indexAlbums, a)
		}
	}

	return indexAlbums
}

func (s *Site) GetS3Service() (*s3.S3, error) {
	return s3.New(s.awsSession), nil
}

func (s *Site) GetPhotoForKey(key string) Renderable {
	if s.UseImgix {
		return s.GetImgixPhoto(key)
	} else {
		return s.GetS3Photo(key)
	}
}

func (s *Site) GetS3Photo(key string) *S3Photo {
	return &S3Photo{
		key,
		s.BucketName,
		s.awsSession,
	}
}

func (s *Site) GetImgixPhoto(key string) *ImgixPhoto {
	if baseUrl, err := url.Parse(s.BaseUrl); err != nil {
		fmt.Printf("Error trying to parse site base URL. Error: %s\n", err.Error())
		return nil
	} else {
		return &ImgixPhoto{
			key,
			baseUrl,
		}
	}
}

func (s *Site) GetAlbumForPath(path string) (*Album, error) {
	for _, album := range s.Albums {
		if album.Path == path {
			return album, nil
		}
	}

	return nil, fmt.Errorf("Could not find album in site %s for path '%s'", s.Domain, path)
}
