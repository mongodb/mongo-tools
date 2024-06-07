package aws

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"log"
	"os"
	"path"
	"regexp"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/mongodb/mongo-tools/release/download"
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

// GetClient returns the global AWS client.
// It initializes the AWS client if it hasn't already been initialized.
func GetClient() (*AWS, error) {
	if awsClient == nil {
		err := initializeClient()
		if err != nil {
			return nil, err
		}
	}
	return awsClient, nil
}

// UploadFile will upload a file from the filesystem to the bucket and
// path specified. The uploaded file keeps its filename.
func (a *AWS) UploadFile(bucket, objPath, filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}

	return a.UploadBytes(bucket, objPath, filename, file)
}

// UploadBytes uploads data from a reader to the bucket, path, and filename specified.
func (a *AWS) UploadBytes(bucket, objPath, filename string, reader io.Reader) error {
	key := path.Join(objPath, filename)

	uploader := s3manager.NewUploader(a.session)
	_, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		ACL:    aws.String("public-read"),
		Body:   reader,
	})
	if err != nil {
		return fmt.Errorf("failed to upload: %w", err)
	}

	return nil
}

// DownloadFile downloads filename from bucket and
func (a *AWS) DownloadFile(bucket, filename string) ([]byte, error) {
	downloader := s3manager.NewDownloader(a.session)

	input := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(filename),
	}

	buff := &aws.WriteAtBuffer{}

	_, err := downloader.Download(buff, input)
	if err != nil {
		return nil, err
	}

	return buff.Bytes(), nil
}

// GenerateFullReleaseFeedFromObjects will download all release artifacts from
// the tools s3 bucket, caluclate their md5, sha1, and sha256 digests, and create
// a download.JSONFeed object representing every artifact for every tools version.
func (a *AWS) GenerateFullReleaseFeedFromObjects() (*download.JSONFeed, error) {
	svc := s3.New(a.session)
	downloader := s3manager.NewDownloader(a.session)
	// It is vital that we set the downloader Concurrency to 1 so that
	// HashWriterAt can safely convert WriteAt calls to Write calls.
	// This is necessary because hash.Hash is a Writer, but not a WriterAt.
	downloader.Concurrency = 1

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String("downloads.mongodb.org"),
		Prefix: aws.String("tools/db/"),
	}

	feed := &download.JSONFeed{}

	handlePage := func(page *s3.ListObjectsV2Output, lastPage bool) bool {
		for _, obj := range page.Contents {
			fmt.Printf("\nWorking on object: %v\n", *obj.Key)

			artifactMetadata := extractArtifactMetadata(obj)
			if artifactMetadata == nil {
				fmt.Printf("Could not match regex for filename, skipping...\n")
				continue
			}

			fmt.Printf("platform: %v, arch: %v, version: %v, ext: %v\n",
				artifactMetadata.Platform,
				artifactMetadata.Arch,
				artifactMetadata.Version,
				artifactMetadata.Ext)

			hashes := downloadAndGenerateHashes(downloader, *obj.Key)

			addDownloadToFeed(feed, artifactMetadata, hashes)
		}

		return *page.IsTruncated
	}

	err := svc.ListObjectsV2Pages(input, handlePage)
	if err != nil {
		return nil, err
	}

	feed.Sort()

	return feed, nil
}

func newHashWriterAt() HashWriterAt {
	md5HashWriter := md5.New()
	sha1HashWriter := sha1.New()
	sha256HashWriter := sha256.New()

	return HashWriterAt{
		MD5:    md5HashWriter,
		SHA1:   sha1HashWriter,
		SHA256: sha256HashWriter,
		w:      io.MultiWriter(sha256HashWriter, sha1HashWriter, md5HashWriter),
	}
}

// HashWriterAt is used to calculate md5, sha1, and sha256 hashes in parallel.
// w is a MulitWriter that writes to all the Hash interfaces.
type HashWriterAt struct {
	MD5    hash.Hash
	SHA1   hash.Hash
	SHA256 hash.Hash
	w      io.Writer
}

// WriteAt fakes the io.WriterAt interface because s3manager.Downloarder.Download()
// expects an io.WriterAt. Since we set the concurrency of the downloader to 1,
// we can safely convert WriteAt calls to Write calls by ignoring the offset.
func (fw HashWriterAt) WriteAt(p []byte, offset int64) (n int, err error) {
	return fw.w.Write(p)
}

// ArtifactMetadata is a container to easily pass around some metadata extracted
// from s3 object filenames.
type ArtifactMetadata struct {
	Name     string
	Version  string
	Platform string
	Arch     string
	Ext      string
}

func extractArtifactMetadata(obj *s3.Object) *ArtifactMetadata {
	name := *obj.Key

	artifactParts := regexp.MustCompile(`^tools\/db\/mongodb-database-tools-(.*)-(.*)-([0-9]+\.[0-9]+\.[0-9]+-?.*)\.(zip|tgz|deb|rpm|msi)$`)
	parts := artifactParts.FindStringSubmatch(name)

	if parts == nil {
		return nil
	}

	return &ArtifactMetadata{
		Name:     name,
		Platform: parts[1],
		Arch:     parts[2],
		Version:  parts[3],
		Ext:      parts[4],
	}
}

// downloadAndGenerateHashes will exit if the download fails
func downloadAndGenerateHashes(downloader *s3manager.Downloader, name string) HashWriterAt {
	input := &s3.GetObjectInput{
		Bucket: aws.String("downloads.mongodb.org"),
		Key:    aws.String(name),
	}
	hashWriter := newHashWriterAt()

	_, err := downloader.Download(hashWriter, input)
	if err != nil {
		log.Fatal(err)
	}

	return hashWriter
}

func addDownloadToFeed(feed *download.JSONFeed, am *ArtifactMetadata, hashes HashWriterAt) {
	md5Hash := hex.EncodeToString(hashes.MD5.Sum(nil))
	sha1Hash := hex.EncodeToString(hashes.SHA1.Sum(nil))
	sha256Hash := hex.EncodeToString(hashes.SHA256.Sum(nil))

	fmt.Printf("MD5: %v\nSHA1: %v\nSHA256: %v\n", md5Hash, sha1Hash, sha256Hash)

	dl := feed.FindOrCreateDownload(am.Version, am.Platform, am.Arch)

	if am.Ext == "zip" || am.Ext == "tgz" {
		dl.Archive = download.ToolsArchive{
			URL:    fmt.Sprintf("https://fastdl.mongodb.org/%s", am.Name),
			Md5:    md5Hash,
			Sha1:   sha1Hash,
			Sha256: sha256Hash,
		}
	} else {
		dl.Package = &download.ToolsPackage{
			URL:    fmt.Sprintf("https://fastdl.mongodb.org/%s", am.Name),
			Md5:    md5Hash,
			Sha1:   sha1Hash,
			Sha256: sha256Hash,
		}
	}

	fmt.Printf("Added %s info to feed\n", am.Name)
}
