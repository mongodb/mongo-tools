package oplog

import (
	"gopkg.in/mgo.v2/bson"
)

type OplogEntry struct {
	Timestamp bson.MongoTimestamp `bson:"ts" json:"ts"`
	HistoryID int64               `bson:"h" json:"h"`
	Version   int                 `bson:"v" json:"v"`
	Operation string              `bson:"op" json:"op"`
	Namespace string              `bson:"ns" json:"ns"`
	Object    bson.M              `bson:"o" json:"o"`
	Query     bson.M              `bson:"o2" json:"o2"`
}
