// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

package mongoreplay

import (
	"io"

	"github.com/10gen/llmgo"
)

// OpMsg sends a diagnostic message to the database. The database sends back a fixed response.
// OpMsg is Deprecated
// http://docs.mongodb.org/meta-driver/latest/legacy/mongodb-wire-protocol/#op-msg
type OpMsg struct {
	Header  MsgHeader
	Message string
}

// OpCode returns the OpCode for the OpMsg.
func (op *OpMsg) OpCode() OpCode {
	return OpCodeMessage
}

// FromReader does nothing for an OpMsg
func (op *OpMsg) FromReader(r io.Reader) error {
	return nil
}

// Execute does nothing for an OpMsg
func (op *OpMsg) Execute(socket *mgo.MongoSocket) (*ReplyOp, error) {
	return nil, nil
}

// Abbreviated does nothing for an OpMsg
func (op *OpMsg) Abbreviated(chars int) string {
	return ""
}
