package main

import (
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/go-ini/ini"
)

type Site struct {
	Domain          string
	CanonicalSecure bool

	BucketRegion string
	BucketName   string

	UseImgix bool
	BaseUrl  string

	AWS_SECRET_KEY_ID string `ini:"AWSKeyId"`
	AWS_SECRET_KEY    string `ini:"AWSKey"`

	SiteTitle string

	HasAlbumIndex bool
	Albums        []*Album
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

func (s *Site) GetS3Service() (*s3.S3, error) {
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(s.BucketRegion),
		Credentials: credentials.NewStaticCredentials(s.AWS_SECRET_KEY_ID, s.AWS_SECRET_KEY, ""),
	})

	if err != nil {
		return nil, err
	}

	return s3.New(sess), nil
}

func (s *Site) GetUrlForImage(key string) (string, error) {
	if s.UseImgix {
		return s.GetImgixUrl(key)
	} else {
		return s.GetAwsUrl(key)
	}
}

func (s *Site) GetAwsUrl(key string) (string, error) {
	svc, err := s.GetS3Service()
	if err != nil {
		return "", err
	}

	req, _ := svc.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(s.BucketName),
		Key:    aws.String(key),
	})

	signedUrl, err := req.Presign(24 * time.Hour)
	if err != nil {
		return "", err
	}

	return signedUrl, nil
}

func (s *Site) GetImgixUrl(key string) (string, error) {
	baseUrl, err := url.Parse(s.BaseUrl)
	if err != nil {
		return "", err
	}

	keyPath, err := url.Parse(key)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s?w=800", baseUrl.ResolveReference(keyPath).String()), nil
}

func (s *Site) GetAlbumForPath(path string) (*Album, error) {
	for _, album := range s.Albums {
		if album.Path == path {
			return album, nil
		}
	}

	return nil, fmt.Errorf("Could not find album in site %s for path '%s'", s.Domain, path)
}
