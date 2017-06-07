package main

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/go-ini/ini"
	"time"
)

type Site struct {
	Domain string

	BucketRegion string
	BucketName   string

	BucketBaseUrl string

	AWS_SECRET_KEY_ID string
	AWS_SECRET_KEY    string
}

func LoadSiteFromFile(path string) (*Site, error) {
	cfg, err := ini.Load(path)
	if err != nil {
		return nil, err
	}

	section, err := cfg.GetSection("")
	if err != nil {
		return nil, err
	}

	for _, v := range []string{"Domain", "Region", "Bucket", "AWSKeyId", "AWSKey"} {
		if !section.HasKey(v) {
			return nil, fmt.Errorf("Config file %s does not contain value of required key %s", path, v)
		}
	}

	bucketName := section.Key("Bucket").String()
	return &Site{
		Domain:       section.Key("Domain").String(),
		BucketRegion: section.Key("Region").String(),
		BucketName:   bucketName,

		// For now we only deal with AWS buckets
		BucketBaseUrl: fmt.Sprintf("http://%s.s3.amazonaws.com", bucketName),

		AWS_SECRET_KEY_ID: section.Key("AWSKeyId").String(),
		AWS_SECRET_KEY:    section.Key("AWSKey").String(),
	}, nil
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

	objects, err := svc.ListObjects(&s3.ListObjectsInput{Bucket: aws.String(s.BucketName)})
	if err != nil {
		return nil, err
	}

	return objects.Contents, nil
}

func (s *Site) GetAllImageKeys() ([]string, error) {
	svc, err := s.GetS3Service()
	if err != nil {
		return nil, err
	}

	objects, err := s.GetAllObjects()
	if err != nil {
		return nil, err
	}

	var imageKeys []string
	var IMAGE_CONTENT_TYPES []string = []string{"image/jpeg", "image/png"}

	for _, obj := range objects {
		headOutput, err := svc.HeadObject(&s3.HeadObjectInput{
			Bucket: aws.String(s.BucketName),
			Key:    obj.Key,
		})
		if err != nil {
			continue
		}

		for _, CT := range IMAGE_CONTENT_TYPES {
			if CT == *headOutput.ContentType {
				imageKeys = append(imageKeys, *obj.Key)
			}
		}
	}

	return imageKeys, nil
}

func (s *Site) GetAllImageUrls() []string {
	var imageUrls []string = []string{}

	imageKeys, err := s.GetAllImageKeys()
	if err != nil {
		fmt.Printf("Unable to get image keys from S3. Error: %s", err.Error())
		return imageUrls
	}

	for _, v := range imageKeys {
		url, err := s.SignUrlForGet(v)
		if err != nil {
			fmt.Printf("Unable to get signed URL for %s. Error: %s", v, err.Error())
			continue
		}
		imageUrls = append(imageUrls, url)
	}

	return imageUrls
}

func (s *Site) SignUrlForGet(key string) (string, error) {
	svc, err := s.GetS3Service()
	if err != nil {
		return "", err
	}

	req, _ := svc.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(s.BucketName),
		Key: aws.String(key),
	})

	signedUrl, err := req.Presign(24 * time.Hour)
	if err != nil {
		return "", err
	}

	return signedUrl, nil
}