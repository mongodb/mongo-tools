package db

import (
	"context"

	"github.com/mongodb/mongo-tools/common/bsonutil"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	mopt "go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/xoptions"
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
func (q *DeferredQuery) Iter(version Version) (*mongo.Cursor, error) {
	opts := mopt.Find()
	if q.Hint != nil {
		opts.SetHint(q.Hint)
	}
	if q.LogReplay {
		opts.SetOplogReplay(true)
	}

	// TODO (SERVER-121847): The backup role is missing the rawData privilege on the admin db; this
	// server ticket will fix that. When that happens, remove this special case.
	if version.GTE(Version{8, 3, 0}) && q.Coll.Database().Name() != "admin" {
		err := xoptions.SetInternalFindOptions(opts, "rawData", true)
		if err != nil {
			return nil, err
		}
	}

	filter := q.Filter
	if filter == nil {
		filter = bson.D{}
	}
	return q.Coll.Find(context.TODO(), filter, opts)
}
