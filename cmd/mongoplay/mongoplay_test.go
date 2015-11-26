package main

import (
	"bytes"
	"io/ioutil"
	"net"
	"testing"
	"time"

	"github.com/10gen/llmgo"
	"github.com/10gen/llmgo/bson"
	"github.com/10gen/mongoplay/mongoproto"
	"reflect"
)

type SessionStub struct {
	mgo.MongoSession
	connection ConnStub
}

type ConnStub struct {
	closed      bool
	readBuffer  *bytes.Buffer
	writeBuffer *bytes.Buffer
}

func (conn *ConnStub) Read(b []byte) (n int, err error) {
	return conn.readBuffer.Read(b)
}

func (conn *ConnStub) Write(b []byte) (n int, err error) {
	return conn.writeBuffer.Write(b)
}

func (conn *ConnStub) Close() error {
	return ioutil.NopCloser(conn).Close()
}

func (conn *ConnStub) LocalAddr() net.Addr {
	return nil
}

func (conn *ConnStub) RemoteAddr() net.Addr {
	return nil
}

func (conn *ConnStub) SetDeadline(t time.Time) error {
	return nil
}

func (conn *ConnStub) SetReadDeadline(t time.Time) error {
	return nil
}

func (conn *ConnStub) SetWriteDeadline(t time.Time) error {
	return nil
}

func newTwoSidedConn() (conn1 ConnStub, conn2 ConnStub) {
	buffer1 := &bytes.Buffer{}
	buffer2 := &bytes.Buffer{}
	conn1 = ConnStub{false, buffer1, buffer2}
	conn2 = ConnStub{false, buffer2, buffer1}
	return conn1, conn2
}

func (session *SessionStub) AcquireSocketPrivate(slaveOk bool) (*mgo.MongoSocket, error) {
	server := mgo.MongoServer{}
	var t time.Duration
	return mgo.NewSocket(&server, &session.connection, t), nil
}

func TestOpGetMore(t *testing.T) {
	session := SessionStub{}
	var serverConnection ConnStub
	serverConnection, session.connection = newTwoSidedConn()

	op := mongoproto.GetMoreOp{}
	op.Collection = "mongoplay_test.test"
	op.CursorId = 12345
	op.Limit = -1

	mgo.ExecOpWithReply(&session, &op.GetMoreOp)
	_, err := mongoproto.OpFromReader(&serverConnection)
	if err != nil {
		panic(err)
	}
	opReceived, err := mongoproto.OpFromReader(&serverConnection)
	if err != nil {
		panic(err)
	}
	getMoreOp := opReceived.(*mongoproto.GetMoreOp)

	switch {
	case getMoreOp.Collection != "mongoplay_test.test":
		t.Fail()
	case getMoreOp.CursorId != 12345:
		t.Fail()
	case getMoreOp.Limit != -1:
		t.Fail()
	}
}

func TestOpDelete(t *testing.T) {
	session := SessionStub{}
	var serverConnection ConnStub
	serverConnection, session.connection = newTwoSidedConn()

	op := mongoproto.DeleteOp{}
	op.Collection = "mongoplay_test.test"
	op.Flags = 7
	selector := bson.D{{"test", 1}}
	op.Selector = selector

	mgo.ExecOpWithoutReply(&session, &op.DeleteOp)
	_, err := mongoproto.OpFromReader(&serverConnection)
	if err != nil {
		panic(err)
	}

	opReceived, err := mongoproto.OpFromReader(&serverConnection)
	if err != nil {
		panic(err)
	}

	deleteOp := opReceived.(*mongoproto.DeleteOp)

	switch {
	case deleteOp.Collection != "mongoplay_test.test":
		t.Fail()
	case deleteOp.Flags != 7:
		t.Fail()
	case !reflect.DeepEqual(selector, op.Selector):
		t.Fail()
	}
}

type testDoc struct {
	Name           string `bson:"name"`
	DocumentNumber int    `bson:"docNum"`
	Success        bool   `bson:"success"`
}

func TestInsertOp(t *testing.T) {
	session := SessionStub{}
	var serverConnection ConnStub
	serverConnection, session.connection = newTwoSidedConn()

	op := mongoproto.InsertOp{}
	op.Collection = "mongoplay_test.test"
	op.Flags = 7

	documents := []interface{}(nil)
	for i := 0; i < 10; i++ {
		insertDoc := &testDoc{
			DocumentNumber: i,
			Success:        true,
		}
		documents = append(documents, insertDoc)
	}
	op.Documents = documents
	err := mgo.ExecOpWithoutReply(&session, &op.InsertOp)
	if err != nil {
		panic(err)
	}
	_, err = mongoproto.OpFromReader(&serverConnection)
	if err != nil {
		panic(err)
	}

	opReceived, err := mongoproto.OpFromReader(&serverConnection)
	if err != nil {
		panic(err)
	}
	insertOp := opReceived.(*mongoproto.InsertOp)

	switch {
	case insertOp.Collection != "mongoplay_test.test":
		t.Fail()
	case insertOp.Flags != 7:
		t.Fail()
	}
	for i, doc := range insertOp.Documents {
		marshaled, _ := bson.Marshal(documents[i])
		unmarshaled := &bson.D{}
		bson.Unmarshal(marshaled, unmarshaled)
		if !reflect.DeepEqual(unmarshaled, doc) {
			t.Fatalf("Documents not equal %v -- %v\n", unmarshaled, doc)
		}
	}
}

func TestKillCursorsOp(t *testing.T) {
	session := SessionStub{}
	var serverConnection ConnStub
	serverConnection, session.connection = newTwoSidedConn()

	op := mongoproto.KillCursorsOp{}
	op.CursorIds = []int64{123, 456, 789, 55}

	err := mgo.ExecOpWithoutReply(&session, &op.KillCursorsOp)
	if err != nil {
		panic(err)
	}
	_, err = mongoproto.OpFromReader(&serverConnection)
	if err != nil {
		panic(err)
	}

	opReceived, err := mongoproto.OpFromReader(&serverConnection)
	if err != nil {
		panic(err)
	}

	killCursorsOp := opReceived.(*mongoproto.KillCursorsOp)

	if !reflect.DeepEqual(killCursorsOp.CursorIds, op.CursorIds) {
		t.Fatalf("CursorId Arrays not equal %v -- %v\n", killCursorsOp.CursorIds, op.CursorIds)
	}
}

func TestQueryOp(t *testing.T) {
	session := SessionStub{}
	var serverConnection ConnStub
	serverConnection, session.connection = newTwoSidedConn()

	op := mongoproto.QueryOp{}
	op.Collection = "mongoplay_test.test"
	op.Flags = 0
	op.HasOptions = true
	op.Limit = -1
	op.Skip = 0
	selector := bson.D{{"test", 1}}
	op.Selector = selector
	options := mgo.QueryWrapper{}
	options.Explain = false
	options.OrderBy = &bson.D{{"_id", 1}}
	op.Options = options

	err := mgo.ExecOpWithoutReply(&session, &op.QueryOp)
	if err != nil {
		panic(err)
	}
	_, err = mongoproto.OpFromReader(&serverConnection)
	if err != nil {
		panic(err)
	}

	opReceived, err := mongoproto.OpFromReader(&serverConnection)
	if err != nil {
		panic(err)
	}

	queryOp := opReceived.(*mongoproto.QueryOp)
	switch {
	case queryOp.Collection != op.Collection:
		t.Fatalf("Collections not equal: %v -- %v\n", queryOp.Collection, op.Collection)
	case !reflect.DeepEqual(&selector, queryOp.Selector):
		t.Fatalf("Selectors not equal: %v -- %v\n", queryOp.Selector, &selector)
	case queryOp.Flags != op.Flags:
		t.Fatalf("Flags not equal: %d -- %d\n", queryOp.Flags, op.Flags)
	case queryOp.Skip != op.Skip:
		t.Fatalf("Skips not equal: %d -- %d\n", queryOp.Skip, op.Skip)
	case queryOp.Limit != op.Limit:
		t.Fatalf("Limits not equal: %d -- %d\n", queryOp.Limit, op.Limit)
	}
	//currently we do not test the Options functionality of mgo
}

func TestOpUpdate(t *testing.T) {
	session := SessionStub{}
	var serverConnection ConnStub
	serverConnection, session.connection = newTwoSidedConn()

	op := mongoproto.UpdateOp{}
	selector := bson.D{{"test", 1}}
	op.Selector = selector
	update := bson.D{{"$set", bson.D{{"updated", true}}}}
	op.Update = update
	op.Collection = "mongoplay_test.test"
	op.Flags = 12345

	err := mgo.ExecOpWithoutReply(&session, &op.UpdateOp)
	if err != nil {
		panic(err)
	}

	_, err = mongoproto.OpFromReader(&serverConnection)
	if err != nil {
		panic(err)
	}

	opReceived, err := mongoproto.OpFromReader(&serverConnection)
	if err != nil {
		panic(err)
	}
	updateOp := opReceived.(*mongoproto.UpdateOp)
	switch {
	case updateOp.Collection != op.Collection:
		t.Fatalf("Collections not equal: %v -- %v\n", updateOp.Collection, op.Collection)
	case !reflect.DeepEqual(updateOp.Selector, &selector):
		t.Fatalf("Selectors not equal: %v -- %v\n", updateOp.Selector, &selector)
	case !reflect.DeepEqual(updateOp.Update, &update):
		t.Fatalf("Updates not equal: %v -- %v\n", updateOp.Update, &update)
	case updateOp.Flags != op.Flags:
		t.Fatalf("Flags not equal: %d -- %d\n", updateOp.Flags, op.Flags)
	}
}
