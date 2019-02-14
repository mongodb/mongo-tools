// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Package mongofiles provides an interface to GridFS collections in a MongoDB instance.
package mongofiles

import (
	"context"
	"fmt"
	"github.com/mongodb/mongo-go-driver/bson"
	"github.com/mongodb/mongo-go-driver/bson/primitive"
	"github.com/mongodb/mongo-go-driver/mongo/gridfs"
	driverOptions "github.com/mongodb/mongo-go-driver/mongo/options"
	"github.com/mongodb/mongo-go-driver/x/bsonx"
	"github.com/mongodb/mongo-tools-common/bsonutil"
	"github.com/mongodb/mongo-tools-common/db"
	"github.com/mongodb/mongo-tools-common/json"
	"github.com/mongodb/mongo-tools-common/log"
	"github.com/mongodb/mongo-tools-common/options"
	"github.com/mongodb/mongo-tools-common/util"
	mgobson "gopkg.in/mgo.v2/bson"

	"io"
	"os"
	"regexp"
	"time"
)

// List of possible commands for mongofiles.
const (
	List     = "list"
	Search   = "search"
	Put      = "put"
	PutID    = "put_id"
	Get      = "get"
	GetID    = "get_id"
	Delete   = "delete"
	DeleteID = "delete_id"
)

// MongoFiles is a container for the user-specified options and
// internal state used for running mongofiles.
type MongoFiles struct {
	// generic mongo tool options
	ToolOptions *options.ToolOptions

	// mongofiles-specific storage options
	StorageOptions *StorageOptions

	// mongofiles-specific input options
	InputOptions *InputOptions

	// for connecting to the db
	SessionProvider *db.SessionProvider

	// command to run
	Command string

	// filename in GridFS
	FileName string

	// ID to put into GridFS
	Id string

	// GridFS bucket to operate on
	bucket *gridfs.Bucket
}

// Struct representing a GridFS files collection document.
type gfsFile struct {
	Id         interface{}        `bson:"_id"`
	Name       string             `bson:"filename"`
	Length     int64              `bson:"length"`
	Md5        string             `bson:"md5"`
	UploadDate time.Time          `bson:"uploadDate"`
	Metadata   gfsFileMetadata    `bson:"metadata"`

	// Storage required for reading and writing GridFS files
	mf         *MongoFiles
	downStream *gridfs.DownloadStream
	upStream   *gridfs.UploadStream
}

// Struct representing the metadata associated with a GridFS files collection document.
type gfsFileMetadata struct {
	ChunkSize   int           	   `bson:"chunkSize"`
	ContentType string             `bson:"contentType,omitempty"`
}

// Write data to GridFS Upload Stream. If this file has not been written to before, this function will open a new stream that must be closed.
// Note: if this file already exists, the chunks written here will be orphaned when close is called and an error will be returned.
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
		objectId, ok := file.Id.(primitive.ObjectID)
		if !ok {
			return 0, fmt.Errorf("need to use objectid for _id")
		}

		stream, err := file.mf.bucket.OpenUploadStreamWithID(objectId, file.Name, &driverOptions.UploadOptions{Metadata: doc})
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
		objectId, ok := file.Id.(primitive.ObjectID)
		if !ok {
			return 0, fmt.Errorf("need to use objectid for _id")
		}

		stream, err := file.mf.bucket.OpenDownloadStream(objectId)
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
	objectId, ok := file.Id.(primitive.ObjectID)
	if !ok {
		return fmt.Errorf("must be objectid")
	}

	if err := file.mf.bucket.Delete(objectId); err != nil {
		return fmt.Errorf("error while removing '%v' from GridFS: %v\n", file.Name, err)
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
			return fmt.Errorf("could not close download stream: %v", err)
		}
	}

	return nil
}

// Query GridFS for files and display the results.
func (mf *MongoFiles) findAndDisplay(query bson.M) (string, error) {
	gridFiles, err := mf.getGFSFiles(query)
	if err != nil {
		return "", fmt.Errorf("error retrieving list of GridFS files: %v", err)
	}

	var display string
	for _, gridFile := range gridFiles {
		display += fmt.Sprintf("%s\t%d\n", gridFile.Name, gridFile.Length)
	}

	return display, nil
}

// Return the local filename, as specified by the --local flag. Defaults to
// the GridFile's name if not present. If GridFile is nil, uses the filename
// given on the command line.
func (mf *MongoFiles) getLocalFileName(gridFile *gfsFile) string {
	localFileName := mf.StorageOptions.LocalFileName
	if localFileName == "" {
		if gridFile != nil {
			localFileName = gridFile.Name
		} else {
			localFileName = mf.FileName
		}
	}
	return localFileName
}

// Gets all GridFS files that match the given query.
func (mf *MongoFiles) getGFSFiles(query bson.M) ([]*gfsFile, error) {
	cursor, err := mf.bucket.Find(query)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())

	var files []*gfsFile

	for cursor.Next(context.Background()) {
		var out gfsFile

		if err = cursor.Decode(&out); err != nil {
			return nil, fmt.Errorf("error decoding GFSFile: %v", err)
		}

		if err = cursor.Decode(&out.Metadata); err != nil {
			return nil, fmt.Errorf("error decoding GFSFile metadata: %v", err)
		}

		out.mf = mf
		files = append(files, &out)
	}

	return files, nil
}

// Gets the GridFS file the options specify. Use this for the get family of commands.
func (mf *MongoFiles) getTargetGFSFile() (*gfsFile, error) {
	var gridFiles []*gfsFile
	var err error

	if mf.Id != "" {
		id, err := mf.parseID()
		if err != nil {
			return nil, err
		}
		gridFiles, err = mf.getGFSFiles(bson.M{"_id": id})
		if err != nil {
			return nil, err
		}
	} else {
		gridFiles, err = mf.getGFSFiles(bson.M{"filename": mf.FileName})
		if err != nil {
			return nil, err
		}
	}

	if len(gridFiles) == 0 {
		return nil, fmt.Errorf("no such file with name: %v", mf.FileName)
	}

	return gridFiles[0], nil
}

// Delete all files with the given filename.
func (mf *MongoFiles) deleteAll(filename string) error {
	gridFiles, err := mf.getGFSFiles(bson.M{"filename": mf.FileName})
	if err != nil {
		return err
	}

	for _, gridFile := range gridFiles {
		if err := gridFile.Delete(); err != nil {
			return err
		}
	}
	log.Logvf(log.Always, "successfully deleted all instances of '%v' from GridFS\n", mf.FileName)

	return nil
}

// Write the given GridFS file to the database. Will fail if file already exists.
func (mf *MongoFiles) put(gridFile *gfsFile) error {
	localFileName := mf.getLocalFileName(gridFile)

	var localFile io.ReadCloser
	var err error

	if localFileName == "-" {
		localFile = os.Stdin
	} else {
		localFile, err = os.Open(localFileName)
		if err != nil {
			return fmt.Errorf("error while opening local gridFile '%v' : %v\n", localFileName, err)
		}
		defer localFile.Close()
		log.Logvf(log.DebugLow, "creating GridFS gridFile '%v' from local gridFile '%v'", mf.FileName, localFileName)
	}

	// check if --replace flag turned on
	if mf.StorageOptions.Replace {
		if err := mf.deleteAll(gridFile.Name); err != nil {
			return err
		}
	}

	if mf.StorageOptions.ContentType != "" {
		gridFile.Metadata.ContentType = mf.StorageOptions.ContentType
	}
	n, err := io.Copy(gridFile, localFile)
	if err != nil {
		return fmt.Errorf("error while storing '%v' into GridFS: %v\n", localFileName, err)
	}

	log.Logvf(log.DebugLow, "copied %v bytes to server", n)
	log.Logvf(log.Always, fmt.Sprintf("added gridFile: %v\n", gridFile.Name))

	return nil
}

// writeGFSFileToFile writes a file from gridFS to stdout or the filesystem.
func (mf *MongoFiles) writeGFSFileToFile(gridFile *gfsFile) (err error) {
	localFileName := mf.getLocalFileName(gridFile)
	var localFile io.WriteCloser
	if localFileName == "-" {
		localFile = os.Stdout
	} else {
		if localFile, err = os.Create(localFileName); err != nil {
			return fmt.Errorf("error while opening local file '%v': %v\n", localFileName, err)
		}
		defer localFile.Close()
		log.Logvf(log.DebugLow, "created local file '%v'", localFileName)
	}

	if _, err = io.Copy(localFile, gridFile); err != nil {
		return fmt.Errorf("error while writing Data into local file '%v': %v\n", localFileName, err)
	}

	log.Logvf(log.Always, fmt.Sprintf("finished writing to %s\n", localFileName))
	return nil
}

// parse and convert input extended JSON _id.
func (mf *MongoFiles) parseID() (interface{}, error) {
	if mf.Id == "" {
		return primitive.NewObjectID(), nil
	}

	var asJSON interface{}
	if err := json.Unmarshal([]byte(mf.Id), &asJSON); err != nil {
		return nil, fmt.Errorf("error parsing provided extJSON: %v", err)
	}

	// legacy extJSON parser
	id, err := bsonutil.ConvertJSONValueToBSON(asJSON)
	if err != nil {
		return nil, fmt.Errorf("error converting extJSON vlaue to bson: %v", err)
	}

	// TODO: fix this (GO-815)
	mgoId, ok := id.(mgobson.ObjectId)
	if !ok {
		return nil, fmt.Errorf("only use ObjectIds as input _id")
	}
	objectId, err := primitive.ObjectIDFromHex(mgoId.Hex())
	if err != nil {
		return nil, err
	}

	return objectId, nil
}

// ValidateCommand ensures the arguments supplied are valid.
func (mf *MongoFiles) ValidateCommand(args []string) error {
	// make sure a command is specified and that we don't have
	// too many arguments
	if len(args) == 0 {
		return fmt.Errorf("no command specified")
	}

	switch args[0] {
	case List:
		if len(args) > 2 {
			return fmt.Errorf("too many positional arguments")
		}
		if len(args) == 1 {
			mf.FileName = ""
		} else {
			mf.FileName = args[1]
		}
	case Search, Put, Get, Delete:
		if len(args) > 2 {
			return fmt.Errorf("too many positional arguments")
		}
		// also make sure the supporting argument isn't literally an
		// empty string for example, mongofiles get ""
		if len(args) == 1 || args[1] == "" {
			return fmt.Errorf("'%v' argument missing", args[0])
		}
		mf.FileName = args[1]
	case GetID, DeleteID:
		if len(args) > 2 {
			return fmt.Errorf("too many positional arguments")
		}
		if len(args) == 1 || args[1] == "" {
			return fmt.Errorf("'%v' argument missing", args[0])
		}
		mf.Id = args[1]
	case PutID:
		if len(args) > 3 {
			return fmt.Errorf("too many positional arguments")
		}
		if len(args) < 3 || args[1] == "" || args[2] == "" {
			return fmt.Errorf("'%v' argument(s) missing", args[0])
		}
		mf.FileName = args[1]
		mf.Id = args[2]
	default:
		return fmt.Errorf("'%v' is not a valid command", args[0])
	}

	if mf.StorageOptions.GridFSPrefix == "" {
		return fmt.Errorf("--prefix can not be blank")
	}

	mf.Command = args[0]
	return nil
}

// Run the mongofiles utility. If displayHost is true, the connected host/port is
// displayed.
func (mf *MongoFiles) Run(displayHost bool) (output string, finalErr error) {
	var err error

	connUrl := mf.ToolOptions.Host
	if connUrl == "" {
		connUrl = util.DefaultHost
	}
	if mf.ToolOptions.Port != "" {
		connUrl = fmt.Sprintf("%s:%s", connUrl, mf.ToolOptions.Port)
	}

	// check type of node we're connected to, and fall back to w=1 if standalone (for <= 2.4)
	nodeType, err := mf.SessionProvider.GetNodeType()
	if err != nil {
		return "", fmt.Errorf("error determining type of node connected: %v", err)
	}

	log.Logvf(log.DebugLow, "connected to node type: %v", nodeType)

	client, err := mf.SessionProvider.GetSession()
	if err != nil {
		return "", fmt.Errorf("error getting client: %v", err)
	}

	err = client.Ping(context.Background(), nil)
	if err != nil {
		return "", fmt.Errorf("error connecting to host: %v", err)
	}

	database := client.Database(mf.StorageOptions.DB)
	mf.bucket, err = gridfs.NewBucket(database, &driverOptions.BucketOptions{Name: &mf.StorageOptions.GridFSPrefix})
	if err != nil {
		return "", fmt.Errorf("error getting GridFS bucket: %v", err)
	}

	if displayHost {
		log.Logvf(log.Always, "connected to: %v", connUrl)
	}

	// first validate the namespaces we'll be using: <db>.<prefix>.files and <db>.<prefix>.chunks
	// it's ok to validate only <db>.<prefix>.chunks (the longer one)
	err = util.ValidateFullNamespace(fmt.Sprintf("%s.%s.chunks", mf.StorageOptions.DB,
		mf.StorageOptions.GridFSPrefix))
	if err != nil {
		return "", err
	}

	safeClose := func(file *gfsFile) {
		if closeErr := file.Close(); closeErr != nil {
			err = closeErr
			output = ""
		}
	}

	log.Logvf(log.Info, "handling mongofiles '%v' command...", mf.Command)

	switch mf.Command {

	case List:
		query := bson.M{}
		if mf.FileName != "" {
			regex := bson.M{"$regex": "^" + regexp.QuoteMeta(mf.FileName)}
			query = bson.M{"filename": regex}
		}

		output, err = mf.findAndDisplay(query)
		if err != nil {
			return "", err
		}

	case Search:
		regex := bson.M{"$regex": mf.FileName}
		query := bson.M{"filename": regex}

		output, err = mf.findAndDisplay(query)
		if err != nil {
			return "", err
		}

	case Get, GetID:
		file, err := mf.getTargetGFSFile()
		if err != nil {
			return "", err
		}
		defer safeClose(file)

		if err = mf.writeGFSFileToFile(file); err != nil {
			return "", err
		}

	case Put, PutID:
		id, err := mf.parseID()
		if err != nil {
			return "", err
		}

		file := gfsFile{Name: mf.FileName, Id: id, mf: mf}
		defer safeClose(&file)

		if err = mf.put(&file); err != nil {
			return "", err
		}

	case DeleteID:
		file, err := mf.getTargetGFSFile()
		if err != nil {
			return "", err
		}

		if err := file.Delete(); err != nil {
			return "", err
		}

		log.Logvf(log.Always, fmt.Sprintf("successfully deleted file with _id %v from GridFS\n", mf.Id))

	case Delete:
		if err := mf.deleteAll(mf.FileName); err != nil {
			return "", err
		}
	}

	return output, nil
}
