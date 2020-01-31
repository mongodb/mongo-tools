package aws

import (
	"fmt"
	"os"
	"path"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

var awsClient *AWS

type AWS struct {
	session *session.Session
}

func initializeClient() error {
	if awsClient != nil {
		panic("called initializeClient twice")
	}

	s, err := session.NewSession(&aws.Config{
		Region: aws.String("us-east-1"),
	})
	if err != nil {
		return fmt.Errorf("failed to create AWS session: %w", err)
	}

	awsClient = &AWS{
		session: s,
	}
	return nil
}

func GetClient() (*AWS, error) {
	if awsClient == nil {
		err := initializeClient()
		if err != nil {
			return nil, err
		}
	}
	return awsClient, nil
}

func (a *AWS) UploadFile(bucket, objPath, filename string) error {
	key := path.Join(objPath, filename)
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}

	uploader := s3manager.NewUploader(a.session)
	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   file,
	})
	if err != nil {
		return fmt.Errorf("failed to upload: %w", err)
	}

	return nil
}
