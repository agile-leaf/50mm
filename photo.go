package main

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"net/url"
	"time"
)

/*
Used when we can't get the photo required, and have to return something, forexample in methods used by templates
*/
type ErrorPhoto struct {
}

type ImgixPhoto struct {
	Key     string
	BaseUrl *url.URL
}

type S3Photo struct {
	Key        string
	BucketName string
	awsSession *session.Session
}

type Renderable interface {
	GetUrlForWidth(int) string
}

func (p *ImgixPhoto) GetUrlForWidth(w int) string {
	keyPathUrl, err := url.Parse(p.Key)
	if err != nil {
		return ""
	}

	fullUrl := p.BaseUrl.ResolveReference(keyPathUrl)
	queryValues := fullUrl.Query()
	queryValues.Add("w", fmt.Sprint(w))
	fullUrl.RawQuery = queryValues.Encode()

	return fullUrl.String()
}

func (p *S3Photo) GetUrlForWidth(w int) string {
	req, _ := s3.New(p.awsSession).GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(p.BucketName),
		Key:    aws.String(p.Key),
	})

	signedUrl, err := req.Presign(24 * time.Hour)
	if err != nil {
		fmt.Printf("Unable to sign URL for S3Photo. Error: %s\n", err.Error())
		return ""
	}

	return signedUrl
}

func (p *ErrorPhoto) GetUrlForWidth(w int) string {
	return ""
}
