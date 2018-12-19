// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package db

import (
	"context"
	"fmt"

	mopt "github.com/mongodb/mongo-go-driver/mongo/options"
	"github.com/mongodb/mongo-go-driver/mongo/readpref"
	"gopkg.in/mgo.v2/bson"
)

// Query flags
const (
	Snapshot = 1 << iota
	LogReplay
	Prefetch
)

type NodeType string

const (
	Mongos     NodeType = "mongos"
	Standalone          = "standalone"
	ReplSet             = "replset"
	Unknown             = "unknown"
)

// CommandRunner exposes functions that can be run against a server
// XXX Does anything rely on this?
type CommandRunner interface {
	Run(command interface{}, out interface{}, database string) error
	RunString(commandName string, out interface{}, database string) error
	FindOne(db, collection string, skip int, query interface{}, sort []string, into interface{}, opts int) error
	Remove(db, collection string, query interface{}) error
	DatabaseNames() ([]string, error)
	CollectionNames(db string) ([]string, error)
}

// // Remove removes all documents matched by query q in the db database and c collection.
// func (sp *SessionProvider) Remove(db, c string, q interface{}) error {
// 	session, err := sp.GetSession()
// 	if err != nil {
// 		return err
// 	}
// 	_, err = session.Database(db).Collection(c).RemoveAll(q)
// 	return err
// }
//
// Run issues the provided command on the db database and unmarshals its result
// into out.

func (sp *SessionProvider) Run(command interface{}, out interface{}, name string) error {
	db := sp.DB(name)
	result := db.RunCommand(context.Background(), command)
	if result.Err() != nil {
		return result.Err()
	}
	err := result.Decode(out)
	if err != nil {
		return err
	}
	return nil
}

func (sp *SessionProvider) RunString(commandName string, out interface{}, name string) error {
	command := &bson.M{commandName: 1}
	return sp.Run(command, out, name)
}

func (sp *SessionProvider) DropDatabase(dbName string) error {
	return sp.DB(dbName).Drop(context.Background())
}

func (sp *SessionProvider) CreateCollection(dbName, collName string) error {
	command := &bson.M{"create": collName}
	out := &bson.RawD{}
	err := sp.Run(command, out, dbName)
	return err
}

func (sp *SessionProvider) ServerVersion() (string, error) {
	out := struct{ Version string }{}
	err := sp.RunString("buildInfo", &out, "admin")
	if err != nil {
		return "", err
	}
	return out.Version, nil
}

// DatabaseNames returns a slice containing the names of all the databases on the
// connected server.
func (sp *SessionProvider) DatabaseNames() ([]string, error) {
	return sp.client.ListDatabaseNames(nil, nil)
}

// CollectionNames returns the names of all the collections in the dbName database.
// func (sp *SessionProvider) CollectionNames(dbName string) ([]string, error) {
// 	session, err := sp.GetSession()
// 	if err != nil {
// 		return nil, err
// 	}
// 	return session.DB(dbName).CollectionNames()
// }

// GetNodeType checks if the connected SessionProvider is a mongos, standalone, or replset,
// by looking at the result of calling isMaster.
func (sp *SessionProvider) GetNodeType() (NodeType, error) {
	session, err := sp.GetSession()
	if err != nil {
		return Unknown, err
	}
	masterDoc := struct {
		SetName interface{} `bson:"setName"`
		Hosts   interface{} `bson:"hosts"`
		Msg     string      `bson:"msg"`
	}{}
	result := session.Database("admin").RunCommand(
		context.Background(),
		&bson.M{"ismaster": 1},
		mopt.RunCmd().SetReadPreference(readpref.Nearest()),
	)
	if result.Err() != nil {
		return Unknown, err
	}
	err = result.Decode(&masterDoc)
	if err != nil {
		return Unknown, err
	}
	if masterDoc.SetName != nil || masterDoc.Hosts != nil {
		return ReplSet, nil
	} else if masterDoc.Msg == "isdbgrid" {
		// isdbgrid is always the msg value when calling isMaster on a mongos
		// see http://docs.mongodb.org/manual/core/sharded-cluster-query-router/
		return Mongos, nil
	}
	return Standalone, nil
}

// IsReplicaSet returns a boolean which is true if the connected server is part
// of a replica set.
func (sp *SessionProvider) IsReplicaSet() (bool, error) {
	nodeType, err := sp.GetNodeType()
	if err != nil {
		return false, err
	}
	return nodeType == ReplSet, nil
}

// IsMongos returns true if the connected server is a mongos.
func (sp *SessionProvider) IsMongos() (bool, error) {
	nodeType, err := sp.GetNodeType()
	if err != nil {
		return false, err
	}
	return nodeType == Mongos, nil
}

//
// // SupportsCollectionUUID returns true if the connected server identifies
// // collections with UUIDs
// func (sp *SessionProvider) SupportsCollectionUUID() (bool, error) {
// 	session, err := sp.GetSession()
// 	if err != nil {
// 		return false, err
// 	}
//
// 	collInfo, err := GetCollectionInfo(session.Database("admin").Collection("system.version"))
// 	if err != nil {
// 		return false, err
// 	}
//
// 	// On FCV 3.6+, admin.system.version will have a UUID
// 	if collInfo != nil && collInfo.GetUUID() != "" {
// 		return true, nil
// 	}
//
// 	return false, nil
// }

// SupportsRepairCursor takes in an example db and collection name and
// returns true if the connected server supports the repairCursor command.
// It returns false and the error that occurred if it is not supported.
func (sp *SessionProvider) SupportsRepairCursor(db, collection string) (bool, error) {
	// XXX disable for now -- xdg, 2018-09-19
	return false, fmt.Errorf("--repair flag cannot be used until supported by the Go driver")
	// 	session, err := sp.GetSession()
	// 	if err != nil {
	// 		return false, err
	// 	}
	//
	// 	// This check is slightly hacky, but necessary to allow users to run repair without
	// 	// permissions to all collections. There are multiple reasons a repair command could fail,
	// 	// but we are only interested in the ones that imply that the repair command is not
	// 	// usable by the connected server. If we do not get one of these specific error messages,
	// 	// we will let the error happen again later.
	// 	//
	// 	// XXX Repair not available, maybe have to just run command and see
	// 	// what raw result we get -- xdg, 2018-09-19
	// 	repairIter := session.Database(db).Collection(collection).Repair()
	// 	repairIter.Next(bson.D{})
	// 	err = repairIter.Err()
	// 	if err == nil {
	// 		return true, nil
	// 	}
	// 	if strings.Index(err.Error(), "no such cmd: repairCursor") > -1 {
	// 		// return a helpful error message for early server versions
	// 		return false, fmt.Errorf("--repair flag cannot be used on mongodb versions before 2.7.8")
	// 	}
	// 	if strings.Index(err.Error(), "repair iterator not supported") > -1 {
	// 		// helpful error message if the storage engine does not support repair (WiredTiger)
	// 		return false, fmt.Errorf("--repair is not supported by the connected storage engine")
	// 	}
	//
	// 	return true, nil
}

//
// // SupportsWriteCommands returns true if the connected server supports write
// // commands, returns false otherwise.
// func (sp *SessionProvider) SupportsWriteCommands() (bool, error) {
// 	session, err := sp.GetSession()
// 	if err != nil {
// 		return false, err
// 	}
// 	masterDoc := struct {
// 		Ok      int `bson:"ok"`
// 		MaxWire int `bson:"maxWireVersion"`
// 	}{}
// 	err = session.Run("isMaster", &masterDoc)
// 	if err != nil {
// 		return false, err
// 	}
// 	// the connected server supports write commands if
// 	// the maxWriteVersion field is present
// 	return (masterDoc.Ok == 1 && masterDoc.MaxWire >= 2), nil
// }

// FindOne retuns the first document in the collection and database that matches
// the query after skip, sort and query flags are applied.
func (sp *SessionProvider) FindOne(db, collection string, skip int, query interface{}, sort interface{}, into interface{}, flags int) error {
	session, err := sp.GetSession()
	if err != nil {
		return err
	}

	opts := mopt.FindOne().SetSort(sort).SetSkip(int64(skip))
	ApplyFlags(opts, flags)

	res := session.Database(db).Collection(collection).FindOne(nil, query, opts)
	err = res.Decode(into)
	return err
}

// ApplyFlags applies flags to the given query session.
func ApplyFlags(opts *mopt.FindOneOptions, flags int) {
	if flags&Snapshot > 0 {
		opts.SetHint("_id_")
	}
	if flags&LogReplay > 0 {
		opts.SetOplogReplay(true)
	}
}
