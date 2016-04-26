package mongotape

import (
	"encoding/json"
	"fmt"
	"io"

	mgo "github.com/10gen/llmgo"
	"github.com/10gen/llmgo/bson"
)

// CommandOp is a struct for parsing OP_COMMAND as defined here: https://github.com/mongodb/mongo/blob/master/src/mongo/rpc/command_request.h.
// Although this file parses the wire protocol message into a more useable struct, it does not currently provide functionality to execute
// the operation, as it is not implemented fully in llmgo.

type CommandOp struct {
	Header MsgHeader
	mgo.CommandOp
}

func (op *CommandOp) String() string {
	commandArgsString, metadataString, inputDocsString, err := op.getOpBodyString()
	if err != nil {
		return fmt.Sprintf("%v", err)
	}
	return fmt.Sprintf("OpCommand %v %v %v %v %v", op.Database, op.CommandName, commandArgsString, metadataString, inputDocsString)
}

// Meta returns metadata about the operation, useful for analysis of traffic.
// Currently only returns 'unknown' as it is not fully parsed and analyzed.

func (op *CommandOp) Meta() OpMetadata {
	return OpMetadata{"", "", "unknown", nil}
}

func (op *CommandOp) Abbreviated(chars int) string {
	commandArgsString, metadataString, inputDocsString, err := op.getOpBodyString()
	if err != nil {
		return fmt.Sprintf("%v", err)
	}
	return fmt.Sprintf("OpCommand %v %v", op.Database, Abbreviate(commandArgsString, chars),
		Abbreviate(metadataString, chars), Abbreviate(inputDocsString, chars))
}

func (op *CommandOp) OpCode() OpCode {
	return OpCodeCommand
}

func (op *CommandOp) getOpBodyString() (string, string, string, error) {
	commandArgsDoc, err := ConvertBSONValueToJSON(op.CommandArgs)
	if err != nil {
		return "", "", "", fmt.Errorf("ConvertBSONValueToJSON err: %#v - %v", op, err)
	}

	commandArgsAsJson, err := json.Marshal(commandArgsDoc)
	if err != nil {
		return "", "", "", fmt.Errorf("json marshal err: %#v - %v", op, err)
	}

	metadataDocs, err := ConvertBSONValueToJSON(op.Metadata)
	if err != nil {
		return "", "", "", fmt.Errorf("ConvertBSONValueToJSON err: %#v - %v", op, err)
	}

	metadataAsJson, err := json.Marshal(metadataDocs)
	if err != nil {
		return "", "", "", fmt.Errorf("json marshal err: %#v - %v", op, err)
	}

	var inputDocsString string

	if len(op.InputDocs) != 0 {
		inputDocs, err := ConvertBSONValueToJSON(op.InputDocs)
		if err != nil {
			return "", "", "", fmt.Errorf("ConvertBSONValueToJSON err: %#v - %v", op, err)
		}

		inputDocsAsJson, err := json.Marshal(inputDocs)
		if err != nil {
			return "", "", "", fmt.Errorf("json marshal err: %#v - %v", op, err)
		}
		inputDocsString = string(inputDocsAsJson)
	}
	return string(commandArgsAsJson), string(metadataAsJson), inputDocsString, nil
}

func (op *CommandOp) FromReader(r io.Reader) error {
	database, err := readCStringFromReader(r)
	if err != nil {
		return err
	}
	op.Database = string(database)

	commandName, err := readCStringFromReader(r)
	if err != nil {
		return err
	}
	op.CommandName = string(commandName)

	commandArgsAsSlice, err := ReadDocument(r)
	if err != nil {
		return err
	}
	op.CommandArgs = &bson.D{}
	err = bson.Unmarshal(commandArgsAsSlice, op.CommandArgs)
	if err != nil {
		return err
	}

	metadataAsSlice, err := ReadDocument(r)
	if err != nil {
		return err
	}
	op.Metadata = &bson.D{}
	err = bson.Unmarshal(metadataAsSlice, op.Metadata)
	if err != nil {
		return err
	}

	lengthRead := len(database) + 1 + len(commandName) + 1 + len(commandArgsAsSlice) + len(metadataAsSlice)

	op.InputDocs = make([]interface{}, 0)
	docLen := 0
	for lengthRead+docLen < int(op.Header.MessageLength)-MsgHeaderLen {
		docAsSlice, err := ReadDocument(r)
		doc := &bson.D{}
		err = bson.Unmarshal(docAsSlice, doc)
		if err != nil {
			return err
		}
		docLen += len(docAsSlice)
		op.InputDocs = append(op.InputDocs, doc)
	}
	return nil
}

// Execute logs a warning and returns nil because OP_COMMAND cannot yet be handled fully by mongotape.

func (op *CommandOp) Execute(session *mgo.Session) (*ReplyOp, error) {
	userInfoLogger.Log(Always, "Skipping unimplmented op: OP_COMMAND")
	return nil, nil
}
func (commandOp1 *CommandOp) Equals(otherOp Op) bool {
	return false
}
