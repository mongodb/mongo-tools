package mongofiles

import (
	"fmt"
	"time"

	"github.com/mongodb/mongo-go-driver/bson"
	"github.com/mongodb/mongo-go-driver/bson/primitive"
	"github.com/mongodb/mongo-go-driver/mongo"
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

func newGfsFile(ID interface{}, name string, mf *MongoFiles) (*gfsFile, error) {
	if ID == nil || mf == nil {
		return nil, fmt.Errorf("invalid gfsFile arguments, one of ID (%v) or MongoFiles (%v) nil", ID, mf)
	}

	return &gfsFile{Name: name, ID: ID, mf: mf}, nil
}

func newGfsFileFromCursor(cursor mongo.Cursor, mf *MongoFiles) (*gfsFile, error) {
	if mf == nil {
		return nil, fmt.Errorf("invalid gfsFile argument, MongoFiles nil")
	}

	var out gfsFile
	if err := cursor.Decode(&out); err != nil {
		return nil, fmt.Errorf("error decoding GFSFile: %v", err)
	}

	if out.ID == nil {
		return nil, fmt.Errorf("invalid gfsFile, ID nil")
	}

	out.mf = mf

	return &out, nil
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
