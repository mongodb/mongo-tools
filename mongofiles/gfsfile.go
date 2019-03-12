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

// OpenStreamForWriting opens a stream for uploading data to a GridFS file that must be closed.
func (file *gfsFile) OpenStreamForWriting() (*gridfs.UploadStream, error) {
	rawBSON, err := bson.Marshal(file.Metadata)
	if err != nil {
		return nil, fmt.Errorf("could not marshal metadata to BSON: %v", err)
	}

	doc, err := bsonx.ReadDoc(rawBSON)
	if err != nil {
		return nil, fmt.Errorf("could not read metadata to document: %v", err)
	}

	// TODO: remove this (GO-815)
	objectID, ok := file.ID.(primitive.ObjectID)
	if !ok {
		return nil, fmt.Errorf("need to use objectid for _id")
	}

	stream, err := file.mf.bucket.OpenUploadStreamWithID(objectID, file.Name, &driverOptions.UploadOptions{Metadata: doc})
	if err != nil {
		return nil, fmt.Errorf("could not open upload stream: %v", err)
	}

	return stream, nil
}

// OpenStreamForReading opens a stream for reading data from a GridFS file that must be closed.
func (file *gfsFile) OpenStreamForReading() (*gridfs.DownloadStream, error) {
	// TODO: remove this (GO-815)
	objectID, ok := file.ID.(primitive.ObjectID)
	if !ok {
		return nil, fmt.Errorf("need to use objectid for _id")
	}

	stream, err := file.mf.bucket.OpenDownloadStream(objectID)
	if err != nil {
		return nil, fmt.Errorf("could not open download stream: %v", err)
	}

	return stream, nil
}

// Deletes the corresponding GridFS file in the database and its chunks.
// Note: this file must be closed if it had been written to before being deleted. Any download streams will be closed as part of this deletion.
func (file *gfsFile) Delete() error {
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

