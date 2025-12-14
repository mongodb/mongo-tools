// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package archive

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/x/bsonx/bsoncore"
)

// NamespaceHeader is a data structure that, as BSON, is found in archives where it indicates
// that either the subsequent stream of BSON belongs to this new namespace, or that the
// indicated namespace will have no more documents (EOF).
type NamespaceHeader struct {
	Database   string `bson:"db"`
	Collection string `bson:"collection"`
	EOF        bool   `bson:"EOF"`
	CRC        uint64 `bson:"CRC"`
}

// Because CRC is a uint64 but BSON only stores int64 we implement BSON
// marshal & unmarshal directly. We could do it via Go’s type-casting, but
// this is cleaner & doesn’t trip up security auditing tools like gosec.
var _ bson.Marshaler = NamespaceHeader{}
var _ bson.Unmarshaler = &NamespaceHeader{}

func (nh NamespaceHeader) MarshalBSON() ([]byte, error) {
	doc := bsoncore.NewDocumentBuilder().
		AppendString("db", nh.Database).
		AppendString("collection", nh.Collection).
		AppendBoolean("EOF", nh.EOF).
		AppendValue(
			"CRC",
			bsoncore.Value{
				Type: bson.TypeInt64,
				Data: binary.LittleEndian.AppendUint64(nil, nh.CRC),
			},
		).Build()

	return doc, nil
}

func (nh *NamespaceHeader) UnmarshalBSON(in []byte) error {
	els, err := bson.Raw(in).Elements()
	if err != nil {
		return errors.Wrapf(err, "parsing %T BSON elements", *nh)
	}

	for _, el := range els {
		var ok bool

		val := el.Value()

		switch el.Key() {
		case "db":
			nh.Database, ok = val.StringValueOK()
			if !ok {
				return fmt.Errorf(
					"%#q is BSON %T but expected %T",
					el.Key(),
					val.Type,
					bson.TypeString,
				)
			}
		case "collection":
			nh.Collection, ok = val.StringValueOK()
			if !ok {
				return fmt.Errorf(
					"%#q is BSON %T but expected %T",
					el.Key(),
					val.Type,
					bson.TypeString,
				)
			}
		case "EOF":
			nh.EOF, ok = val.BooleanOK()
			if !ok {
				return fmt.Errorf(
					"%#q is BSON %T but expected %T",
					el.Key(),
					val.Type,
					bson.TypeBoolean,
				)
			}
		case "CRC":
			if val.Type != bson.TypeInt64 {
				return fmt.Errorf(
					"%#q is BSON %T but expected %T",
					el.Key(),
					val.Type,
					bson.TypeBoolean,
				)
			}

			nh.CRC = binary.LittleEndian.Uint64(val.Value)
		default:
			return fmt.Errorf("unknown BSON %T: %#q", *nh, el.Key())
		}
	}

	return nil
}

// CollectionMetadata is a data structure that, as BSON, is found in the prelude of the archive.
// There is one CollectionMetadata per collection that will be in the archive.
// For a CollectionMetadata for collection X with Type == "timeseries",
// there will be no data for collection X. Instead there will be data for collection system.buckets.X.
// Mongorestore will restore both the timeseries view and the underlying system.buckets collection.
type CollectionMetadata struct {
	Database   string `bson:"db"`
	Collection string `bson:"collection"`
	Metadata   string `bson:"metadata"`
	Size       int    `bson:"size"`
	Type       string `bson:"type"`
}

// Header is a data structure that, as BSON, is found immediately after the magic
// number in the archive, before any CollectionMetadatas. It is the home of any archive level information.
type Header struct {
	ConcurrentCollections int32  `bson:"concurrent_collections"`
	FormatVersion         string `bson:"version"`
	ServerVersion         string `bson:"server_version"`
	ToolVersion           string `bson:"tool_version"`
}

const minBSONSize = 4 + 1 // an empty BSON document should be exactly five bytes long

const terminator = 0xff_ff_ff_ff

var terminatorBytes = binary.LittleEndian.AppendUint32(nil, terminator)

// MagicNumber is four bytes that are found at the beginning of the archive that indicate that
// the byte stream is an archive, as opposed to anything else, including a stream of BSON documents.
const MagicNumber uint32 = 0x8199e26d
const archiveFormatVersion = "0.1"

// Writer is the top level object to contain information about archives in mongodump.
type Writer struct {
	Out     io.WriteCloser
	Prelude *Prelude
	Mux     *Multiplexer
}

// Reader is the top level object to contain information about archives in mongorestore.
type Reader struct {
	In      io.ReadCloser
	Demux   *Demultiplexer
	Prelude *Prelude
}
