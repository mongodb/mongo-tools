package sh

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/craiggwilson/goke/task"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

const numHTTPRetries = 5

// DownloadHTTP issues a GET request against the provided url and downloads the contents to the toPath.
//
// This method will retry HTTP requests that fail.
func DownloadHTTP(ctx *task.Context, url string, toPath string) error {
	var err error
	for i := 0; i < numHTTPRetries; i++ {
		ctx.Logf("attempting HTTP download (%d/%d)\n", i+1, numHTTPRetries)
		// This is a very simplistic retry. Some HTTP response codes do not benefit
		// from being retried, e.g. 4XX errors that are typically the client's
		// fault. Additionally, some errors may occur before the HTTP request is
		// even sent. Despite this, given the use-case for this package (i.e. not
		// production codepaths), we opt for simplicity rather than error-prone
		// case-checking of errors.
		if err = downloadHTTP(ctx, url, toPath); err == nil {
			return nil
		}
	}

	return err
}

func downloadHTTP(ctx *task.Context, url string, toPath string) error {
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
	defer res.Body.Close()
	if res.StatusCode != 200 {
		buf := strings.Builder{}
		_, err := io.Copy(&buf, res.Body)
		if err == nil {
			err = errors.New(buf.String())
		}

		return fmt.Errorf("received non-200 response (%d) for GET: %w", res.StatusCode, err)
	}

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

// HeadS3 checks if an object exists on S3.
func HeadS3(ctx *task.Context, from S3Object, profile string) error {
	ctx.Logf("s3 check: %s/%s\n", from.Bucket, from.Key)

	var err error
	sess := session.Must(session.NewSession(&aws.Config{
		Region:      aws.String(from.Region),
		Credentials: s3Credentials(ctx, profile),
	}))

	svc := s3.New(sess)
	_, err = svc.HeadObject(&s3.HeadObjectInput{
		Bucket: aws.String(from.Bucket),
		Key:    aws.String(from.Key),
	})

	return err
}

// UploadS3 reads the file at the provided path and uploads the contents to S3.
func UploadS3(ctx *task.Context, fromPath string, to S3Object, profile string) error {
	ctx.Logf("s3 upload: %s -> %s/%s\n", fromPath, to.Bucket, to.Key)

	var f *os.File
	f, err := os.Open(fromPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return UploadReaderToS3(ctx, f, to, profile)
}

// UploadReaderToS3 uploads the contents of the provided reader to S3.
func UploadReaderToS3(ctx *task.Context, reader io.Reader, to S3Object, profile string) error {
	sess := session.Must(session.NewSession(&aws.Config{
		Region:      aws.String(to.Region),
		Credentials: s3Credentials(ctx, profile),
	}))

	uploader := s3manager.NewUploader(sess)
	_, err := uploader.UploadWithContext(ctx, &s3manager.UploadInput{
		Bucket: aws.String(to.Bucket),
		Key:    aws.String(to.Key),
		Body:   reader,
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
