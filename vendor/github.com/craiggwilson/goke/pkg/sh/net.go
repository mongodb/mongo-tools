package sh

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/craiggwilson/goke/task"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

// DownloadHTTP issues a GET request against the provided url and downloads the contents to the toPath.
func DownloadHTTP(ctx *task.Context, url string, toPath string) error {
	ctx.Logf("download: %s -> %s\n", url, toPath)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed creating GET request: %v", err)
	}
	req.Header.Add("cache-control", "no-cache")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed issuing GET request: %v", err)
	}
	if res.Body == nil {
		return errors.New("no body from the GET request")
	}
	defer res.Body.Close()

	return copyTo(url, res.Body, toPath, 0666)
}

// S3Object defines an S3 object.
type S3Object struct {
	Region string
	Bucket string
	Key    string
}

// DownloadS3 downloads an object from S3.
func DownloadS3(ctx *task.Context, from S3Object, toPath string, profile string) error {
	ctx.Logf("s3 download: %s/%s\n", from.Bucket, from.Key)

	var err error
	sess := session.Must(session.NewSession(&aws.Config{
		Region:      aws.String(from.Region),
		Credentials: s3Credentials(ctx, profile),
	}))

	var f *os.File
	f, err = os.Create(toPath)
	if err != nil {
		return err
	}
	defer f.Close()

	downloader := s3manager.NewDownloader(sess)
	_, err = downloader.Download(f, &s3.GetObjectInput{
		Bucket: aws.String(from.Bucket),
		Key:    aws.String(from.Key),
	})

	return err
}

// UploadS3 uploads a file to S3.
func UploadS3(ctx *task.Context, fromPath string, to S3Object, profile string) error {
	ctx.Logf("s3 upload: %s -> %s/%s\n", fromPath, to.Bucket, to.Key)

	var err error
	sess := session.Must(session.NewSession(&aws.Config{
		Region:      aws.String(to.Region),
		Credentials: s3Credentials(ctx, profile),
	}))

	uploader := s3manager.NewUploader(sess)
	var f *os.File
	f, err = os.Open(fromPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = uploader.UploadWithContext(ctx, &s3manager.UploadInput{
		Bucket: aws.String(to.Bucket),
		Key:    aws.String(to.Key),
		Body:   f,
	})

	return err
}

func s3Credentials(ctx *task.Context, profile string) *credentials.Credentials {
	if profile == "" {
		profile = Env("AWS_DEFAULT_PROFILE", "")
	}

	if profile != "" {
		return credentials.NewSharedCredentials("", profile)
	}

	return nil
}
