package db

import (
	"github.com/mongodb/mongo-go-driver/mongo"
	mopt "github.com/mongodb/mongo-go-driver/mongo/options"
)

// DeferredQuery represents a deferred query
type DeferredQuery struct {
	Coll      *mongo.Collection
	Filter    interface{}
	Hint      interface{}
	LogReplay bool
}

func (q *DeferredQuery) Count() (int, error) {
	opt := mopt.Count()
	if q.Hint != nil {
		opt.SetHint(q.Hint)
	}
	c, err := q.Coll.CountDocuments(nil, q.Filter, opt)
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
	return q.Coll.Find(nil, q.Filter, opts)
}

// XXX temporary fix; fake a Repair via regular cursor
func (q *DeferredQuery) Repair() (mongo.Cursor, error) {
	return q.Iter()
}
