package mongoimport

import (
	"errors"
	"fmt"
	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/log"
	commonOpts "github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/common/util"
	"github.com/mongodb/mongo-tools/mongoimport/options"
	"gopkg.in/mgo.v2/bson"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

const (
	CSV  = "csv"
	TSV  = "tsv"
	JSON = "json"
)

// compile-time interface sanity check
var (
	_ ImportInput = (*CSVImportInput)(nil)
	_ ImportInput = (*TSVImportInput)(nil)
	_ ImportInput = (*JSONImportInput)(nil)
)

var (
	errNsNotFound = errors.New("ns not found")
)

// Wrapper for MongoImport functionality
type MongoImport struct {
	// generic mongo tool options
	ToolOptions *commonOpts.ToolOptions

	// InputOptions defines options used to read data to be ingested
	InputOptions *options.InputOptions

	// IngestOptions defines options used to ingest data into MongoDB
	IngestOptions *options.IngestOptions

	// SessionProvider is used for connecting to the database
	SessionProvider *db.SessionProvider
}

// ImportInput is an interface that specifies how an input source should be
// converted to BSON
type ImportInput interface {
	// ImportDocument reads the given record from the given io.Reader according
	// to the format supported by the underlying ImportInput implementation.
	ImportDocument() (bson.M, error)

	// SetHeader sets the header for the CSV/TSV import when --headerline is
	// specified. It a --fields or --fieldFile argument is passed, it overwrites
	// the values of those with what is read from the input source
	SetHeader(bool) error

	// ReadHeadersFromSource attempts to reads the header fields for the specific implementation
	ReadHeadersFromSource() ([]string, error)

	// GetHeaders returns the current set of header fields for the specific implementation
	GetHeaders() []string
}

func (mongoImport *MongoImport) getImportWriter() ImportWriter {
	var upsertFields []string
	if mongoImport.IngestOptions.Upsert &&
		len(mongoImport.IngestOptions.UpsertFields) != 0 {
		upsertFields = strings.Split(mongoImport.IngestOptions.UpsertFields, ",")
	}
	if mongoImport.ToolOptions.DBPath == "" {
		return &DriverImportWriter{
			upsertMode:      mongoImport.IngestOptions.Upsert,
			upsertFields:    upsertFields,
			sessionProvider: mongoImport.SessionProvider,
			session:         nil,
		}
	}
	return &ShimImportWriter{
		upsertMode:   mongoImport.IngestOptions.Upsert,
		upsertFields: upsertFields,
		dbPath:       util.ToUniversalPath(mongoImport.ToolOptions.DBPath),
		dirPerDB:     mongoImport.ToolOptions.DirectoryPerDB,
		db:           mongoImport.ToolOptions.Namespace.DB,
		collection:   mongoImport.ToolOptions.Namespace.Collection,
	}
}

// ValidateSettings ensures that the tool specific options supplied for
// MongoImport are valid
func (mongoImport *MongoImport) ValidateSettings(args []string) error {
	if err := mongoImport.ToolOptions.Validate(); err != nil {
		return err
	}
	// Namespace must have a valid database if none is specified,
	// use 'test'
	if mongoImport.ToolOptions.Namespace.DB == "" {
		mongoImport.ToolOptions.Namespace.DB = "test"
	} else {
		if err := util.ValidateDBName(mongoImport.ToolOptions.Namespace.DB); err != nil {
			return err
		}
	}

	// use JSON as default input type
	if mongoImport.InputOptions.Type == "" {
		mongoImport.InputOptions.Type = JSON
	} else {
		if !(mongoImport.InputOptions.Type == TSV ||
			mongoImport.InputOptions.Type == JSON ||
			mongoImport.InputOptions.Type == CSV) {
			return fmt.Errorf("don't know what type [\"%v\"] is",
				mongoImport.InputOptions.Type)
		}
	}

	// ensure headers are supplied for CSV/TSV
	if mongoImport.InputOptions.Type == CSV ||
		mongoImport.InputOptions.Type == TSV {
		if !mongoImport.InputOptions.HeaderLine {
			if mongoImport.InputOptions.Fields == "" &&
				mongoImport.InputOptions.FieldFile == "" {
				return fmt.Errorf("You need to specify fields or have a " +
					"header line to import this file type")
			}
			if mongoImport.InputOptions.Fields != "" &&
				mongoImport.InputOptions.FieldFile != "" {
				return fmt.Errorf("incompatible options: --fields and --fieldFile")
			}
		} else {
			if mongoImport.InputOptions.Fields != "" {
				return fmt.Errorf("incompatible options: --fields and --headerline")
			}
			if mongoImport.InputOptions.FieldFile != "" {
				return fmt.Errorf("incompatible options: --fieldFile and --headerline")
			}
		}
	}
	if len(args) > 1 {
		return fmt.Errorf("too many positional arguments")
	}
	if mongoImport.InputOptions.File != "" && len(args) != 0 {
		return fmt.Errorf(`multiple occurrences of option "--file"`)
	}
	var fileBaseName string
	if mongoImport.InputOptions.File != "" {
		fileBaseName = mongoImport.InputOptions.File
	} else {
		if len(args) != 0 {
			fileBaseName = args[0]
			mongoImport.InputOptions.File = args[0]
		}
	}

	// ensure we have a valid string to use for the collection
	if mongoImport.ToolOptions.Namespace.Collection == "" {
		if fileBaseName == "" {
			return fmt.Errorf("no collection specified")
		}
		fileBaseName = filepath.Base(fileBaseName)
		if lastDotIndex := strings.LastIndex(fileBaseName, "."); lastDotIndex != -1 {
			fileBaseName = fileBaseName[0:lastDotIndex]
		}
		if err := util.ValidateCollectionName(fileBaseName); err != nil {
			return err
		}
		mongoImport.ToolOptions.Namespace.Collection = fileBaseName
		log.Logf(0, "no collection specified")
		log.Logf(0, "using filename '%v' as collection",
			mongoImport.ToolOptions.Namespace.Collection)
	}
	return nil
}

// getInputReader returns an io.Reader corresponding to the input location
func (mongoImport *MongoImport) getInputReader() (io.ReadCloser, error) {
	if mongoImport.InputOptions.File != "" {
		file, err := os.Open(util.ToUniversalPath(mongoImport.InputOptions.File))
		if err != nil {
			return nil, err
		}
		fileStat, err := file.Stat()
		if err != nil {
			return nil, err
		}
		log.Logf(1, "filesize: %v", fileStat.Size())
		return file, err
	}
	log.Logf(1, "filesize: 0")
	return os.Stdin, nil
}

// ImportDocuments is used to write input data to the database. It returns the
// number of documents successfully imported to the appropriate namespace and
// any error encountered in doing this
func (mongoImport *MongoImport) ImportDocuments() (int64, error) {
	in, err := mongoImport.getInputReader()
	if err != nil {
		return 0, err
	}
	defer in.Close()

	importInput, err := mongoImport.getImportInput(in)
	if err != nil {
		return 0, err
	}

	err = importInput.SetHeader(mongoImport.InputOptions.HeaderLine)
	if err != nil {
		return 0, err
	}
	return mongoImport.importDocuments(importInput)
}

// importDocuments is a helper to ImportDocuments and does all the ingestion
// work by taking data from the 'importInput' source and writing it to the
// appropriate namespace
func (mongoImport *MongoImport) importDocuments(importInput ImportInput) (docsCount int64, err error) {
	importWriter := mongoImport.getImportWriter()
	connURL := mongoImport.ToolOptions.Host
	if connURL == "" {
		connURL = util.DefaultHost
	}
	if mongoImport.ToolOptions.Port != "" {
		connURL = connURL + ":" + mongoImport.ToolOptions.Port
	}
	log.Logf(0, "connected to: %v", connURL)

	err = importWriter.Open(
		mongoImport.ToolOptions.Namespace.DB,
		mongoImport.ToolOptions.Namespace.Collection,
	)
	if err != nil {
		return
	}
	log.Logf(1, "ns: %v.%v",
		mongoImport.ToolOptions.Namespace.DB,
		mongoImport.ToolOptions.Namespace.Collection)

	defer func() {
		closeErr := importWriter.Close()
		if err == nil {
			err = closeErr
		}
	}()

	// drop the database if necessary
	if mongoImport.IngestOptions.Drop {
		log.Logf(0, "dropping: %v.%v",
			mongoImport.ToolOptions.DB,
			mongoImport.ToolOptions.Collection)

		if err := importWriter.Drop(); err != nil &&
			err.Error() != errNsNotFound.Error() {
			return 0, err
		}
	}

	ignoreBlanks := mongoImport.IngestOptions.IgnoreBlanks &&
		mongoImport.InputOptions.Type != JSON

	for {
		document, err := importInput.ImportDocument()
		if err != nil {
			if err == io.EOF {
				return docsCount, nil
			}
			if mongoImport.IngestOptions.StopOnError {
				return docsCount, err
			}
			if document == nil {
				return docsCount, err
			}
			log.Logf(0, "error importing document: %v", err)
			continue
		}

		// ignore blank fields if specified
		if ignoreBlanks {
			document = removeBlankFields(document)
		}
		if err = importWriter.Import(document); err != nil {
			if err.Error() == errNoReachableServer.Error() {
				return docsCount, err
			}
			if mongoImport.IngestOptions.StopOnError {
				return docsCount, err
			}
			log.Logf(0, "error inserting document: %v", err)
			continue
		}
		docsCount++
	}
}

// removeBlankFields removes empty/blank fields in csv and tsv
func removeBlankFields(document bson.M) bson.M {
	for key, value := range document {
		if reflect.TypeOf(value).Kind() == reflect.String &&
			value.(string) == "" {
			delete(document, key)
		}
	}
	return document
}

// getImportInput returns an implementation of ImportInput which can handle
// transforming TSV, CSV, or JSON into appropriate BSON documents
func (mongoImport *MongoImport) getImportInput(in io.Reader) (ImportInput,
	error) {
	var fields []string
	var err error
	if len(mongoImport.InputOptions.Fields) != 0 {
		fields = strings.Split(strings.Trim(mongoImport.InputOptions.Fields,
			" "), ",")
	} else if mongoImport.InputOptions.FieldFile != "" {
		fields, err = util.GetFieldsFromFile(mongoImport.InputOptions.FieldFile)
		if err != nil {
			return nil, err
		}
	}
	if mongoImport.InputOptions.Type == CSV {
		return NewCSVImportInput(fields, in), nil
	} else if mongoImport.InputOptions.Type == TSV {
		return NewTSVImportInput(fields, in), nil
	}
	return NewJSONImportInput(mongoImport.InputOptions.JSONArray, in), nil
}
