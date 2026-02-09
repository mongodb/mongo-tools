package db

import (
	"context"

	"github.com/mongodb/mongo-tools/common/bsonutil"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	mopt "go.mongodb.org/mongo-driver/v2/mongo/options"
)

// DeferredQuery represents a deferred query.
type DeferredQuery struct {
	Coll      *mongo.Collection
	Filter    any
	Hint      any
	LogReplay bool
}

// Count issues a EstimatedDocumentCount command when there is no Filter in the query and a CountDocuments command otherwise.
func (q *DeferredQuery) Count(isView bool) (int, error) {
	emptyFilter := false

	filter := q.Filter
	if q.Filter == nil {
		emptyFilter = true
		filter = bson.D{}
	} else if val, ok := q.Filter.(bson.D); ok && (val == nil || len(bsonutil.ToMap(val)) == 0) {
		emptyFilter = true
	} else if val, ok := q.Filter.(bson.M); ok && (val == nil || len(val) == 0) {
		emptyFilter = true
	}

	if emptyFilter && !isView {
		opt := mopt.EstimatedDocumentCount()
		c, err := q.Coll.EstimatedDocumentCount(context.TODO(), opt)
		return int(c), err
	}

	opt := mopt.Count()
	c, err := q.Coll.CountDocuments(context.TODO(), filter, opt)
	return int(c), err
}

// Iter executes a find query and returns a cursor.
func (q *DeferredQuery) Iter() (*mongo.Cursor, error) {
	opts := mopt.Find()
	if q.Hint != nil {
		opts.SetHint(q.Hint)
	}
	if q.LogReplay {
		opts.SetOplogReplay(true)
	}
	filter := q.Filter
	if filter == nil {
		filter = bson.D{}
	}
	return q.Coll.Find(context.TODO(), filter, opts)
}
