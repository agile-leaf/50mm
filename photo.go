package main

import (
	"crypto/rsa"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudfront/sign"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/globocom/gothumbor"
)

type RescaledPhoto struct {
	Key     string
	BaseUrl *url.URL
}

type ImgixRescaledPhoto struct {
	*RescaledPhoto
}

// for use with thumbor as a basic setup, URL signing mandatory.
// quick implementation available at: https://github.com/APSL/docker-thumbor
// be warned - it's a resource hungry beast.
type ThumborRaw struct {
	*RescaledPhoto
	Secret string
}

// for use with thumbor backed by AWS lambda with *cloudfront* URL signing (on unsafe thumbor urls)
// see: https://docs.aws.amazon.com/solutions/latest/serverless-image-handler/welcome.html
type ThumborCloudfront struct {
	*RescaledPhoto                          //it's distinct enough (doesn't have a need for thumbor url signing)
	AWSCloudfrontKeyPairId  string          //required for URL signing
	AWSCloudfrontPrivateKey *rsa.PrivateKey //required for URL signing
}

type S3Photo struct {
	Key        string
	BucketName string
	awsSession *session.Session
}

type Renderable interface {
	Slug() string
	GetPhotoForWidth(int) string
	GetThumbnailForWidthAndHeight(int, int) string
}

func (p *RescaledPhoto) Slug() string {
	parts := strings.Split(p.Key, "/")
	return parts[len(parts)-1]
}

func (p *ImgixRescaledPhoto) GetPhotoForWidth(w int) string {
	keyPathUrl, err := url.Parse(p.Key)
	if err != nil {
		fmt.Print(err)
		return ""
	}

	fullUrl := p.BaseUrl.ResolveReference(keyPathUrl)
	queryValues := fullUrl.Query()
	queryValues.Add("w", fmt.Sprint(w))
	fullUrl.RawQuery = queryValues.Encode()

	return fullUrl.String()
}

func (p *ImgixRescaledPhoto) GetThumbnailForWidthAndHeight(w, h int) string {
	keyPathUrl, err := url.Parse(p.Key)
	if err != nil {
		fmt.Print(err)
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

func (p *ThumborRaw) GetPhotoForWidth(w int) string {
	thumborOptions := gothumbor.ThumborOptions{Width: w, Smart: true}
	thumborPath, err := gothumbor.GetCryptedThumborPath(p.Secret, p.Key, thumborOptions)
	if err != nil {
		fmt.Print(err)
		return ""
	}

	parsedPath, err := url.Parse(thumborPath)
	fullUrl := p.BaseUrl.ResolveReference(parsedPath)

	return fullUrl.String()
}

func (p *ThumborRaw) GetThumbnailForWidthAndHeight(w, h int) string {
	thumborOptions := gothumbor.ThumborOptions{Width: w, Height: h, Smart: true}
	thumborPath, err := gothumbor.GetCryptedThumborPath(p.Secret, p.Key, thumborOptions)
	if err != nil {
		fmt.Print(err)
		return ""
	}

	parsedPath, err := url.Parse(thumborPath)
	fullUrl := p.BaseUrl.ResolveReference(parsedPath)

	return fullUrl.String()
}

func (p *ThumborCloudfront) SignCloudfrontURL(path string) string {

	parsedPath, err := url.Parse(path)
	if err != nil {
		fmt.Printf("Failed to parse URL for signing, err: %s\n", err.Error())
		return ""
	}
	fullUrl := p.BaseUrl.ResolveReference(parsedPath)

	// now sign for cloudfront
	signer := sign.NewURLSigner(p.AWSCloudfrontKeyPairId, p.AWSCloudfrontPrivateKey)
	signedURL, err := signer.Sign(fullUrl.String(), time.Now().Add(1*time.Hour))
	if err != nil {
		fmt.Printf("Failed to sign url, err: %s\n", err.Error())
		return ""
	}
	return signedURL
}

func (p *ThumborCloudfront) GetPhotoForWidth(w int) string {
	// get thumbor path without signing
	thumborOptions := gothumbor.ThumborOptions{Width: w, Smart: true}
	thumborPath, err := gothumbor.GetThumborPath(p.Key, thumborOptions)
	if err != nil {
		fmt.Print(err)
		return ""
	}

	return p.SignCloudfrontURL(thumborPath)
}

func (p *ThumborCloudfront) GetThumbnailForWidthAndHeight(w, h int) string {
	thumborOptions := gothumbor.ThumborOptions{Width: w, Height: h, Smart: true}
	thumborPath, err := gothumbor.GetThumborPath(p.Key, thumborOptions)
	if err != nil {
		fmt.Print(err)
		return ""
	}

	return p.SignCloudfrontURL(thumborPath)
}

func (p *S3Photo) Slug() string {
	parts := strings.Split(p.Key, "/")
	return parts[len(parts)-1]
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

func (p *ErrorPhoto) Slug() string {
	return ""
}

func (p *ErrorPhoto) GetPhotoForWidth(w int) string {
	return ""
}

func (p *ErrorPhoto) GetThumbnailForWidthAndHeight(w, h int) string {
	return ""
}
