package mongofiles

import (
	"fmt"
	"time"

	"github.com/mongodb/mongo-go-driver/bson"
	"github.com/mongodb/mongo-go-driver/bson/primitive"
	"github.com/mongodb/mongo-go-driver/mongo/gridfs"
	driverOptions "github.com/mongodb/mongo-go-driver/mongo/options"
	"github.com/mongodb/mongo-go-driver/x/bsonx"
)

// Struct representing a GridFS files collection document.
type gfsFile struct {
	ID         interface{}     `bson:"_id"`
	Name       string          `bson:"filename"`
	Length     int64           `bson:"length"`
	Md5        string          `bson:"md5"`
	UploadDate time.Time       `bson:"uploadDate"`
	Metadata   gfsFileMetadata `bson:"metadata"`
	ChunkSize   int            `bson:"chunkSize"`

	// Storage required for reading and writing GridFS files
	mf         *MongoFiles
	downStream *gridfs.DownloadStream
	upStream   *gridfs.UploadStream
}

// Struct representing the metadata associated with a GridFS files collection document.
type gfsFileMetadata struct {
	ContentType string             `bson:"contentType,omitempty"`
}

// Write data to GridFS Upload Stream. If this file has not been written to before, this function will open a new stream that must be closed.
// Note: the go driver buffers data until it hits a chunk size before writing to the database, so if the amount of data is written < chunkSize,
// the actual write to the database will occur in Close.
func (file *gfsFile) Write(p []byte) (int, error) {
	if file.upStream == nil {
		rawBSON, err := bson.Marshal(file.Metadata)
		if err != nil {
			return 0, fmt.Errorf("could not marshal metadata to BSON: %v", err)
		}

		doc, err := bsonx.ReadDoc(rawBSON)
		if err != nil {
			return 0, fmt.Errorf("could not read metadata to document: %v", err)
		}

		// TODO: remove this (GO-815)
		objectID, ok := file.ID.(primitive.ObjectID)
		if !ok {
			return 0, fmt.Errorf("need to use objectid for _id")
		}

		stream, err := file.mf.bucket.OpenUploadStreamWithID(objectID, file.Name, &driverOptions.UploadOptions{Metadata: doc})
		if err != nil {
			return 0, err
		}
		file.upStream = stream
	}

	return file.upStream.Write(p)
}

// Reads data from GridFS download stream. If this file has not been read from before, this function will open a new stream that must be closed.
func (file *gfsFile) Read(buf []byte) (int, error) {
	if file.downStream == nil {
		// TODO: remove this (GO-815)
		objectID, ok := file.ID.(primitive.ObjectID)
		if !ok {
			return 0, fmt.Errorf("need to use objectid for _id")
		}

		stream, err := file.mf.bucket.OpenDownloadStream(objectID)
		if err != nil {
			return 0, fmt.Errorf("could not open download stream: %v", err)
		}
		file.downStream = stream
	}

	return file.downStream.Read(buf)
}

// Deletes the corresponding GridFS file in the database and its chunks.
// Note: this file must be closed if it had been written to before being deleted. Any download streams will be closed as part of this deletion.
func (file *gfsFile) Delete() error {
	if file.upStream != nil {
		return fmt.Errorf("this file (%v) must be closed for writing before being deleted", file.Name)
	}

	if err := file.Close(); err != nil {
		return err
	}

	// TODO: remove this (GO-815)
	objectID, ok := file.ID.(primitive.ObjectID)
	if !ok {
		return fmt.Errorf("must be objectid")
	}

	if err := file.mf.bucket.Delete(objectID); err != nil {
		return fmt.Errorf("error while removing '%v' from GridFS: %v", file.Name, err)
	}

	return nil
}

// Closes any opened Download or Upload streams.
func (file *gfsFile) Close() error {
	if file.downStream != nil {
		if err := file.downStream.Close(); err != nil {
			return fmt.Errorf("could not close download stream: %v", err)
		}
	}

	if file.upStream != nil {
		if err := file.upStream.Close(); err != nil {
			return fmt.Errorf("could not close upload stream: %v", err)
		}
	}

	return nil
}
