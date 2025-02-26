package db

import (
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
)

const maxIdStrLen = 200

type FoundExistingDocumentError struct {
	Doc bson.Raw
}

var _ error = &FoundExistingDocumentError{}

func (fed FoundExistingDocumentError) Error() string {
	idStr := fed.Doc.Lookup("_id").String()

	if len(idStr) > maxIdStrLen {
		idStr = idStr[:maxIdStrLen] + " (truncated)"
	}

	return fmt.Sprintf("Found an existing, conflicting document (_id=%s)", idStr)
}
