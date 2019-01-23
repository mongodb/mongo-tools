package db

import (
	"github.com/mongodb/mongo-go-driver/mongo"
	mopt "github.com/mongodb/mongo-go-driver/mongo/options"
	"gopkg.in/mgo.v2/bson"
)

// DeferredQuery represents a deferred query
type DeferredQuery struct {
	Coll      *mongo.Collection
	Filter    interface{}
	Hint      interface{}
	LogReplay bool
}

// Count issues a count command. We don't use the Hint because
// that's not supported with older servers.
func (q *DeferredQuery) Count() (int, error) {
	opt := mopt.Count()
	filter := q.Filter
	if filter == nil {
		filter = bson.D{}
	}
	c, err := q.Coll.CountDocuments(nil, filter, opt)
	return int(c), err
}

func (q *DeferredQuery) Iter() (mongo.Cursor, error) {
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
	return q.Coll.Find(nil, filter, opts)
}

// XXX temporary fix; fake a Repair via regular cursor
func (q *DeferredQuery) Repair() (mongo.Cursor, error) {
	return q.Iter()
}
