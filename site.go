package main

import (
	"fmt"

	"time"

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

	BucketRegion string
	BucketName   string

	UseImgix bool
	BaseUrl  string

	AWS_SECRET_KEY_ID string
	AWS_SECRET_KEY    string

	MetaTitle string
	SiteTitle string
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

	requiredFields := []string{
		"Domain", "Region", "Bucket", "UseImgix", "BaseUrl", "AWSKeyId", "AWSKey", "MetaTitle",
		"SiteTitle", "AlbumTitle",
	}
	for _, v := range requiredFields {
		if !defaultSection.HasKey(v) {
			return nil, fmt.Errorf("Config file %s does not contain value of required key %s", path, v)
		}
	}

	bucketName := defaultSection.Key("Bucket").String()
	s := &Site{
		Domain: defaultSection.Key("Domain").String(),

		BucketRegion: defaultSection.Key("Region").String(),
		BucketName:   bucketName,

		UseImgix: defaultSection.Key("UseImgix").String() == "1",
		BaseUrl:  defaultSection.Key("BaseUrl").String(),

		AWS_SECRET_KEY_ID: defaultSection.Key("AWSKeyId").String(),
		AWS_SECRET_KEY:    defaultSection.Key("AWSKey").String(),

		MetaTitle:  defaultSection.Key("MetaTitle").String(),
		SiteTitle:  defaultSection.Key("SiteTitle").String(),
		AlbumTitle: defaultSection.Key("AlbumTitle").String(),
	}

	if defaultSection.HasKey("AuthUser") && defaultSection.HasKey("AuthPass") {
		s.AuthUser = defaultSection.Key("AuthUser").String()
		s.AuthPass = defaultSection.Key("AuthPass").String()
	}

	if defaultSection.HasKey("Prefix") {
		s.Prefix = defaultSection.Key("Prefix").String()
	}

	if defaultSection.HasKey("CanonicalSecure") {
		s.CanonicalSecure = defaultSection.Key("CanonicalSecure").String() == "1"
	}

	return s, nil
}

func (s *Site) GetCanonicalUrl() string {
	proto, domain := "http", s.Domain
	if s.CanonicalSecure {
		proto = "https"
	}

	return fmt.Sprintf("%s://%s", proto, domain)
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

func (s *Site) GetAllObjects() ([]*s3.Object, error) {
	svc, err := s.GetS3Service()
	if err != nil {
		return nil, err
	}

	objects, err := svc.ListObjects(&s3.ListObjectsInput{
		Bucket:    aws.String(s.BucketName),
		Prefix:    aws.String(s.Prefix),
		Delimiter: aws.String("/"),
	})
	if err != nil {
		return nil, err
	}

	return objects.Contents, nil
}

func (s *Site) GetAllImageKeys() ([]string, error) {
	objects, err := s.GetAllObjects()
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
