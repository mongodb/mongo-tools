package archive

import (
	"bytes"
	"encoding/binary"
	"hash/crc64"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// SimpleArchive represents an entire archive. This is useful for synthesizing
// archives in tests.
type SimpleArchive struct {
	Header             Header
	CollectionMetadata []CollectionMetadata
	Namespaces         []SimpleNamespace
}

// SimpleNamespace represents a namespace in a SimpleArchive.
type SimpleNamespace struct {
	Database   string
	Collection string
	Documents  []bson.D
}

func (sa SimpleArchive) Marshal() ([]byte, error) {
	archiveBytes := []byte{0, 0, 0, 0}
	binary.LittleEndian.PutUint32(archiveBytes, MagicNumber)

	archive := bytes.NewBuffer(archiveBytes)

	dupeHeader := sa.Header
	dupeHeader.FormatVersion = archiveFormatVersion

	headerBytes, err := bson.Marshal(dupeHeader)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal archive header (%+v)", dupeHeader)
	}
	archive.Write(headerBytes)

	for _, metadata := range sa.CollectionMetadata {
		mdBytes, err := bson.Marshal(metadata)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to marshal collection metadata (%+v)", metadata)
		}

		archive.Write(mdBytes)
	}

	archive.Write(terminatorBytes)

	for _, ns := range sa.Namespaces {
		header := NamespaceHeader{
			Database:   ns.Database,
			Collection: ns.Collection,
		}
		nsBytes, err := bson.Marshal(header)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to marshal namespace header (%+v)", header)
		}

		archive.Write(nsBytes)

		crc := crc64.New(crc64.MakeTable(crc64.ECMA))

		for _, doc := range ns.Documents {
			docBytes, err := bson.Marshal(doc)
			if err != nil {
				return nil, errors.Wrapf(
					err,
					"failed to marshal namespace %#q’s document (%+v)",
					ns.Database+"."+ns.Collection,
					doc,
				)
			}

			crc.Write(docBytes)

			archive.Write(docBytes)
		}

		archive.Write(terminatorBytes)

		eol := header
		eol.EOF = true
		eol.CRC = int64(crc.Sum64())
		eolBytes, err := bson.Marshal(eol)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to marshal namespace EOL (%+v)", eol)
		}

		archive.Write(eolBytes)
		archive.Write(terminatorBytes)
	}

	return archive.Bytes(), nil
}
