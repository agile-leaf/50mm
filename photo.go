package main

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"net/url"
	"time"
)

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
	GetPhotoForWidth(int) string
	GetThumbnailForWidthAndHeight(int, int) string
}

func (p *ImgixPhoto) GetPhotoForWidth(w int) string {
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

func (p *ImgixPhoto) GetThumbnailForWidthAndHeight(w, h int) string {
	keyPathUrl, err := url.Parse(p.Key)
	if err != nil {
		return ""
	}

	fullUrl := p.BaseUrl.ResolveReference(keyPathUrl)
	queryValues := fullUrl.Query()
	queryValues.Add("w", fmt.Sprint(w))
	queryValues.Add("max-h", fmt.Sprint(h))
	queryValues.Add("fit", "crop")
	queryValues.Add("crop", "faces")

	fullUrl.RawQuery = queryValues.Encode()

	return fullUrl.String()
}

func (p *S3Photo) GetPhotoForWidth(w int) string {
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

func (p *S3Photo) GetThumbnailForWidthAndHeight(w, h int) string {
	return p.GetPhotoForWidth(w)
}

/*
Used when we can't get the photo required, and have to return something, for example in methods used by templates
*/
type ErrorPhoto struct {
}

func (p *ErrorPhoto) GetPhotoForWidth(w int) string {
	return ""
}

func (p *ErrorPhoto) GetThumbnailForWidthAndHeight(w, h int) string {
	return ""
}
