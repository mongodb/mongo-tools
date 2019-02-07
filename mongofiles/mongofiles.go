// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Package mongofiles provides an interface to GridFS collections in a MongoDB instance.
package mongofiles

import (
	"context"
	"errors"
	"fmt"
	"github.com/mongodb/mongo-go-driver/bson/primitive"
	"github.com/mongodb/mongo-go-driver/mongo/gridfs"
	driverOptions "github.com/mongodb/mongo-go-driver/mongo/options"
	"github.com/mongodb/mongo-go-driver/mongo/readpref"
	"io"
	"os"
	"regexp"

	//"regexp"
	"time"

	"github.com/mongodb/mongo-go-driver/bson"
	"github.com/mongodb/mongo-tools-common/db"
	"github.com/mongodb/mongo-tools-common/log"
	"github.com/mongodb/mongo-tools-common/options"
	"github.com/mongodb/mongo-tools-common/util"
	"gopkg.in/mgo.v2"
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

type GFSFile struct {
	Metadata GFSFileMetadata
	Data     []byte
	i        int
}

func (file *GFSFile) Read(buf []byte) (int, error) {
	return copy(buf, file.Data), nil
}

// GFSFileMetadata represents a GridFS file.
type GFSFileMetadata struct {
	Id          bson.RawValue      `bson:"_id"`
	ChunkSize   int           	   `bson:"chunkSize"`
	Name        string             `bson:"filename"`
	Length      int64              `bson:"length"`
	Md5         string             `bson:"md5"`
	UploadDate  time.Time          `bson:"uploadDate"`
	ContentType string             `bson:"contentType,omitempty"`
}

func (mf *MongoFiles) getBucket() (*gridfs.Bucket, error) {
	if mf.bucket != nil {
		return mf.bucket, nil
	}

	session, err := mf.SessionProvider.GetSession()
	if err != nil {
		return nil, err
	}

	database := session.Database(mf.StorageOptions.DB)
	mf.bucket, err = gridfs.NewBucket(database, &driverOptions.BucketOptions{Name: &mf.StorageOptions.GridFSPrefix})
	if err != nil {
		return nil, err
	}

	return mf.bucket, nil
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

// Query GridFS for files and display the results.
func (mf *MongoFiles) findAndDisplay(query bson.M) (string, error) {
	bucket, err := mf.getBucket()
	if err != nil {
		return "", err
	}

	display := ""

	cursor, err := bucket.Find(query)
	if err != nil {
		return "", err
	}

	defer cursor.Close(context.Background())

	for cursor.Next(context.Background()) {
		var file GFSFileMetadata
		err = cursor.Decode(&file)
		if err != nil {
			return "", fmt.Errorf("error encountered while iterating cursor: %v", err)
		}

		display += fmt.Sprintf("%s\t%d\n", file.Name, file.Length)
		/*
		if file.Name != "" {
			display += fmt.Sprintf("filename: %s\t%d\n", file.Name, file.Length)
		} else {
			display += fmt.Sprintf("_id: %s\t%d\n", file.Id, file.Length)
		}*/
	}
	if err := cursor.Err(); err != nil {
		return "", fmt.Errorf("error retrieving list of GridFS files: %v", err)
	}

	return display, nil
}

// Return the local filename, as specified by the --local flag. Defaults to
// the GridFile's name if not present. If GridFile is nil, uses the filename
// given on the command line.
func (mf *MongoFiles) getLocalFileName(gridFile *GFSFile) string {
	localFileName := mf.StorageOptions.LocalFileName
	if localFileName == "" {
		if gridFile != nil {
			localFileName = gridFile.Metadata.Name
		} else {
			localFileName = mf.FileName
		}
	}
	return localFileName
}

func (mf *MongoFiles) getFileMetadata(query bson.M) (*GFSFileMetadata, error) {
	bucket, err := mf.getBucket()
	if err != nil {
		return nil, err
	}

	cursor, err := bucket.Find(query)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())

	if !cursor.Next(context.Background()) {
		return nil, errors.New("cursor has no results")
	}

	var out GFSFileMetadata
	if err = cursor.Decode(&out); err != nil {
		return &out, err
	}

	return &out, nil
}

func (mf *MongoFiles) getFileMetadataByName(filename string) (*GFSFileMetadata, error) {
	return mf.getFileMetadata(bson.M{"filename": filename})
}

func (mf *MongoFiles) getFileMetadataById(id interface{}) (*GFSFileMetadata, error) {
	return mf.getFileMetadata(bson.M{"_id": id})
}

func (mf *MongoFiles) getGFSFileByStream(stream *gridfs.DownloadStream, metadata *GFSFileMetadata) (*GFSFile, error) {
	var buf = make([]byte, metadata.Length)
	_, err := stream.Read(buf)
	if err != nil {
		return nil, err
	}

	return &GFSFile{Metadata: *metadata, Data: buf}, nil
}

func (mf *MongoFiles) getGFSFileById(id interface{}) (*GFSFile, error) {
	metadata, err := mf.getFileMetadataById(id)
	if err != nil {
		return nil, err
	}

	bucket, err := mf.getBucket()
	if err != nil {
		return nil, err
	}

	oid, ok := id.(primitive.ObjectID)
	if !ok {
		return nil, fmt.Errorf("id is not an objectid %v", id)
	}

	stream, err := bucket.OpenDownloadStream(oid)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	return mf.getGFSFileByStream(stream, metadata)
}

func (mf *MongoFiles) getGFSFileByName(filename string) (*GFSFile, error) {
	metadata, err := mf.getFileMetadataByName(filename)
	if err != nil {
		return nil, err
	}

	bucket, err := mf.getBucket()
	if err != nil {
		return nil, err
	}

	stream, err := bucket.OpenDownloadStreamByName(filename)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	return mf.getGFSFileByStream(stream, metadata)
}

// handle logic for 'get' command
func (mf *MongoFiles) handleGet() error {
	file, err := mf.getGFSFileByName(mf.FileName)

	if err = mf.writeGFSFile(file); err != nil {
		return err
	}

	log.Logvf(log.Always, fmt.Sprintf("finished writing to %s\n", mf.FileName))

	return nil
}

// handle logic for 'get_id' command
func (mf *MongoFiles) handleGetID() error {
	id, err := mf.parseID()
	if err != nil {
		return err
	}

	file, err := mf.getGFSFileById(id)
	if err != nil {
		return err
	}

	if err = mf.writeGFSFile(file); err != nil {
		return err
	}

	log.Logvf(log.Always, fmt.Sprintf("finished writing to: %s\n", file.Metadata.Name))
	return nil
}

// logic for deleting a file
func (mf *MongoFiles) handleDelete(gfs *mgo.GridFS) error {
	err := gfs.Remove(mf.FileName)
	if err != nil {
		return fmt.Errorf("error while removing '%v' from GridFS: %v\n", mf.FileName, err)
	}
	log.Logvf(log.Always, "successfully deleted all instances of '%v' from GridFS\n", mf.FileName)
	return nil
}

// logic for deleting a file with 'delete_id'
func (mf *MongoFiles) handleDeleteID(gfs *mgo.GridFS) error {
	id, err := mf.parseID()
	if err != nil {
		return err
	}
	if err = gfs.RemoveId(id); err != nil {
		return fmt.Errorf("error while removing file with _id %v from GridFS: %v\n", mf.Id, err)
	}
	log.Logvf(log.Always, fmt.Sprintf("successfully deleted file with _id %v from GridFS\n", mf.Id))
	return nil
}

// parse and convert extended JSON
func (mf *MongoFiles) parseID() (interface{}, error) {
	var id interface{}
	if err := bson.UnmarshalExtJSON([]byte(mf.Id), false, &id); err != nil {
		return nil, fmt.Errorf("cant unmarshal ext json: %v", err)
	}

	// TODO: fix this
	if _, ok := id.(primitive.ObjectID); !ok {
		return bson.Raw{}, fmt.Errorf("only use ObjectIds as _id")
		// return nil, fmt.Errorf("error parsing _id as json: %v; make sure you are properly escaping input")
	}

	return id, nil
}

// writeGFSFile writes a file from gridFS to stdout or the filesystem.
func (mf *MongoFiles) writeGFSFile(gridFile *GFSFile) (err error) {
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

	if _, err = localFile.Write(gridFile.Data); err != nil {
		return fmt.Errorf("error while writing Data into local file '%v': %v\n", localFileName, err)
	}
	return nil
}

func (mf *MongoFiles) handlePut(gfs *mgo.GridFS, hasID bool) (err error) {
	localFileName := mf.getLocalFileName(nil)

	// check if --replace flag turned on
	if mf.StorageOptions.Replace {
		err = gfs.Remove(mf.FileName)
		if err != nil {
			return err
		}
		// always log that Data has been removed
		log.Logvf(log.Always, "removed all instances of '%v' from GridFS\n", mf.FileName)
	}

	var localFile io.ReadCloser

	if localFileName == "-" {
		localFile = os.Stdin
	} else {
		localFile, err = os.Open(localFileName)
		if err != nil {
			return fmt.Errorf("error while opening local file '%v' : %v\n", localFileName, err)
		}
		defer localFile.Close()
		log.Logvf(log.DebugLow, "creating GridFS file '%v' from local file '%v'", mf.FileName, localFileName)
	}

	gridFile, err := gfs.Create(mf.FileName)
	if err != nil {
		return fmt.Errorf("error while creating '%v' in GridFS: %v\n", mf.FileName, err)
	}
	defer func() {
		// GridFS files flush a buffer on Close(), so it's important we
		// capture any errors that occur as this function exits and
		// overwrite the error if earlier writes executed successfully
		if closeErr := gridFile.Close(); err == nil && closeErr != nil {
			log.Logvf(log.DebugHigh, "error occurred while closing GridFS file handler")
			err = fmt.Errorf("error while storing '%v' into GridFS: %v\n", localFileName, closeErr)
		}
	}()

	if hasID {
		id, err := mf.parseID()
		if err != nil {
			return err
		}
		gridFile.SetId(id)
	}

	// set optional mime type
	if mf.StorageOptions.ContentType != "" {
		gridFile.SetContentType(mf.StorageOptions.ContentType)
	}

	n, err := io.Copy(gridFile, localFile)
	if err != nil {
		return fmt.Errorf("error while storing '%v' into GridFS: %v\n", localFileName, err)
	}
	log.Logvf(log.DebugLow, "copied %v bytes to server", n)

	log.Logvf(log.Always, fmt.Sprintf("added file: %v\n", gridFile.Name()))
	return nil
}

// Run the mongofiles utility. If displayHost is true, the connected host/port is
// displayed.
func (mf *MongoFiles) Run(displayHost bool) (string, error) {
	connUrl := mf.ToolOptions.Host

	// TODO: validate options
	var err error

	if connUrl == "" {
		connUrl = util.DefaultHost
	}
	if mf.ToolOptions.Port != "" {
		connUrl = fmt.Sprintf("%s:%s", connUrl, mf.ToolOptions.Port)
	}

	var readPref *readpref.ReadPref
	if mf.InputOptions.ReadPreference != "" {
		var err error
		readPref, err = db.ParseReadPreference(mf.InputOptions.ReadPreference)
		if err != nil {
			return "", fmt.Errorf("error parsing --readPreference : %v", err)
		}
	} else {
		readPref = readpref.Nearest()
	}
	mf.ToolOptions.ReadPreference = readPref
	// TODO: disable socket timeout

	mf.SessionProvider, err = db.NewSessionProvider(*mf.ToolOptions)
	if err != nil {
		return "", err
	}

	// check type of node we're connected to, and fall back to w=1 if standalone (for <= 2.4)
	nodeType, err := mf.SessionProvider.GetNodeType()
	if err != nil {
		return "", fmt.Errorf("error determining type of node connected: %v", err)
	}

	log.Logvf(log.DebugLow, "connected to node type: %v", nodeType)

	// TODO: figure out new write concern situation
	// safety, err := db.BuildWriteConcern(mf.StorageOptions.WriteConcern, nodeType,
	//	mf.ToolOptions.URI.ParsedConnString())

	// if err != nil {
	//	return "", fmt.Errorf("error parsing write concern: %v", err)
	// }

	// configure the session with the appropriate write concern and ensure the
	// socket does not timeout
	// session.SetSafe(safety)

	client, err := mf.SessionProvider.GetSession()
	if err != nil {
		return "", fmt.Errorf("error getting client: %v", err)
	}

	err = client.Ping(context.Background(), nil)
	if err != nil {
		return "", fmt.Errorf("error connecting to host: %v", err)
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

	var output string

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

	case Get:
		err = mf.handleGet()
		if err != nil {
			return "", err
		}
		output = ""
	case GetID:
		err = mf.handleGetID()
		if err != nil {
			return "", err
		}

	case Put:
		return "put", nil
		/*

		err = mf.handlePut(gfs, false)
		if err != nil {
			return "", err
		}*/

	case PutID:
		return "putID", nil
		/*
		err = mf.handlePut(gfs, true)
		if err != nil {
			return "", err
		}*/

	case Delete:
		return "delete", nil
		/*
		err = mf.handleDelete(gfs)
		if err != nil {
			return "", err
		}*/

	case DeleteID:
		return "deleteId", nil

		/*
		err = mf.handleDeleteID(gfs)
		if err != nil {
			return "", err
		}
*/
	}

	return output, nil
}
