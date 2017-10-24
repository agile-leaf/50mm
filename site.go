package main

import (
	"errors"
	"fmt"
	"net/url"

	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/go-ini/ini"
	"io/ioutil"
)

type Site struct {
	Domain          string
	CanonicalSecure bool

	AuthUser string
	AuthPass string

	S3Host       string
	BucketRegion string
	BucketName   string

	ResizingService       string
	ResizingServiceSecret string
	BaseUrl               string

	AWS_SECRET_KEY_ID                  string          `ini:"AWSKeyId"`
	AWS_SECRET_KEY                     string          `ini:"AWSKey"`
	AWS_CLOUDFRONT_PRIVATE_KEY_PATH    string          `ini:"AWSCloudfrontKeyPath"`
	AWS_CLOUDFRONT_PRIVATE_KEY_PAIR_ID string          `ini:"AWSCloudfrontKeyPairId"`
	CloudfrontPrivateKey               *rsa.PrivateKey //this is loaded on config read
	//from the path provided in AWS_PRIVATE_KEY_PATH

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

	// set up private key for thumbor+cloudfront, if applicable
	if s.ResizingService == "thumbor+cloudfront" && s.AWS_CLOUDFRONT_PRIVATE_KEY_PATH != "" && s.AWS_CLOUDFRONT_PRIVATE_KEY_PAIR_ID != "" {
		// borrowed from: https://github.com/ianmcmahon/encoding_ssh

		// read in private key from file (private key is PEM encoded PKCS)
		bytes, err := ioutil.ReadFile(s.AWS_CLOUDFRONT_PRIVATE_KEY_PATH)
		if err != nil {
			return nil, err
		}

		// decode PEM encoding to ANS.1 PKCS1 DER
		block, _ := pem.Decode(bytes)
		if block == nil {
			return nil, errors.New("Private Key: No Block found in keyfile")
		}
		if block.Type != "RSA PRIVATE KEY" {
			return nil, errors.New("Private Key: Unsupported key type, should be RSA Private key in pem file")
		}

		// parse DER format to a native type
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)

		if err != nil {
			return nil, errors.New("Private Key: could not parse RSA key")
		}

		s.CloudfrontPrivateKey = key
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

	if s.ResizingService != "imgix" && s.ResizingService != "thumbor" && s.ResizingService != "thumbor+cloudfront" && s.ResizingService != "" {
		return errors.New(fmt.Sprintf("Unrecognized/Unimplemented resizing service '%s',"+
			" valid options are imgix, thumbor, thumbor+cloudfront", s.ResizingService))
	}

	if s.ResizingService == "thumbor" && s.ResizingServiceSecret == "" {
		return errors.New("Thumbor resizing service requires use of a shared secret for URL signing")
	}

	if s.ResizingService == "thumbor+cloudfront" && (s.AWS_CLOUDFRONT_PRIVATE_KEY_PATH == "" || s.AWS_CLOUDFRONT_PRIVATE_KEY_PAIR_ID == "") {
		return errors.New("thumbor+cloudfront resizing service requires you to provision a private key " +
			"and provide the path to the private key(config AWSCloudfrontKeyPath)," +
			" along with the associated key pair id (config AWSCloudfrontKeyPairId)")
	} else if s.ResizingService == "thumbor+cloudfront" && s.AWS_CLOUDFRONT_PRIVATE_KEY_PATH != "" {
		// borrowed from: https://github.com/ianmcmahon/encoding_ssh

		// read in private key from file (private key is PEM encoded PKCS)
		bytes, err := ioutil.ReadFile(s.AWS_CLOUDFRONT_PRIVATE_KEY_PATH)
		if err != nil {
			return err
		}

		// decode PEM encoding to ANS.1 PKCS1 DER
		block, _ := pem.Decode(bytes)
		if block == nil {
			return errors.New("Private Key: No Block found in keyfile")
		}
		if block.Type != "RSA PRIVATE KEY" {
			return errors.New("Private Key: Unsupported key type, should be RSA Private key in pem file")
		}

		// we'll skip parsing the whole damn thing, that'll be done on init.
		// less dev surprises this way
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
	if s.ResizingService == "" {
		//TODO: maybe warn that you're going raw
		return s.GetS3Photo(key)
	} else {
		return s.GetScaledPhoto(key)
	}
}

func (s *Site) GetS3Photo(key string) *S3Photo {
	return &S3Photo{
		key,
		s.BucketName,
		s.awsSession,
	}
}

func (s *Site) GetScaledPhoto(key string) Renderable {
	if baseUrl, err := url.Parse(s.BaseUrl); err != nil {
		fmt.Printf("Error trying to parse site base URL. Error: %s\n", err.Error())
		return nil
	} else {
		if s.ResizingService == "imgix" {
			return &ImgixRescaledPhoto{
				RescaledPhoto: &RescaledPhoto{
					key,
					baseUrl,
				},
			}
		} else if s.ResizingService == "thumbor" {
			return &ThumborRaw{
				RescaledPhoto: &RescaledPhoto{
					key,
					baseUrl,
				},
				Secret: s.ResizingServiceSecret,
			}
		} else if s.ResizingService == "thumbor+cloudfront" {
			return &ThumborCloudfront{
				RescaledPhoto: &RescaledPhoto{
					key,
					baseUrl,
				},
				AWSCloudfrontKeyPairId:  s.AWS_CLOUDFRONT_PRIVATE_KEY_PAIR_ID,
				AWSCloudfrontPrivateKey: s.CloudfrontPrivateKey,
			}
		} else {
			// it should never come to this due to configuration validation,
			// but best to keep the compiler happy.
			return nil
		}
	}
}

func (s *Site) GetAlbumForPath(path string) (*Album, error) {
	if path[len(path)-1] != '/' {
		path = path + "/"
	}
	for _, album := range s.Albums {
		if album.Path == path {
			return album, nil
		}
	}

	return nil, fmt.Errorf("Could not find album in site %s for path '%s'", s.Domain, path)
}
