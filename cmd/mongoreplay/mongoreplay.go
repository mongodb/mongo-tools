package main

import (
	"bytes"
	"fmt"

	"github.com/10gen/mongoplay"
	"github.com/10gen/mongoplay/mongoproto"
	"github.com/jessevdk/go-flags"
	"os"
)

type Options struct {
	PlaybackFile1 struct {
		Fname string
	} `required:"yes" positional-args:"yes" description:"First playback file to read"`
	PlaybackFile2 struct {
		Fname string
	} `required:"yes" positional-args:"yes" description:"Second playback file to read, generated form reply of first file"`
}

func main() {
	opts := Options{}
	var parser = flags.NewParser(&opts, flags.Default)
	_, err := parser.Parse()
	if err != nil {
		os.Exit(1)
	}
	eqv, err := equivalentPlayback(opts.PlaybackFile1.Fname, opts.PlaybackFile2.Fname)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	if !eqv {
		fmt.Println("FILES ARE NOT EQUIVALENT")
		os.Exit(1)
	} else {
		fmt.Println("FILES ARE EQUIVALENT")
	}
}

func equivalentPlayback(playFile, replayFile string) (bool, error) {
	playChan, err := mongoplay.NewPlayOpChan(playFile)
	if err != nil {
		return false, err
	}
	replayChan, err := mongoplay.NewPlayOpChan(replayFile)
	if err != nil {
		return false, err
	}

	for op1 := range playChan {
		moreReplay := false
		for op2 := range replayChan {
			moreReplay = true

			val, err := EqualOps(op1, op2)
			if err != nil {
				return false, err
			}
			if !val {
				fmt.Println("Operations Not Equivalent")
				continue
			} else {
				fmt.Println("Operations Equivalent")
				break
			}
		}
		if !moreReplay {
			fmt.Println("Reached End of Replay File")
			return false, nil
		}
	}
	return true, nil
}

func EqualOps(op1 *mongoplay.OpWithTime, op2 *mongoplay.OpWithTime) (bool, error) {
	if int32(op1.Header.OpCode) != int32(op2.Header.OpCode) {
		fmt.Printf("Opcodes do not match. Found %d and %d\n", op1.Header.OpCode, op2.Header.OpCode)
		return false, nil
	}

	if op1.Header.OpCode == mongoproto.OpCodeReserved ||
		op1.Header.OpCode == mongoproto.OpCodeReply ||
		op1.Header.OpCode == mongoproto.OpCodeMessage {
		fmt.Printf("Saw %s: IGNORING\n", op1.Header.OpCode)
		return true, nil
	}
	reader1 := bytes.NewReader(op1.Body)
	reader2 := bytes.NewReader(op2.Body)

	fmt.Printf("Comparing Ops: %s\n", op1.Header.OpCode)

	var protoOp1, protoOp2 mongoproto.Op

	switch op1.Header.OpCode {
	case mongoproto.OpCodeQuery:
		protoOp1 = &mongoproto.QueryOp{Header: op1.Header}
		protoOp2 = &mongoproto.QueryOp{Header: op2.Header}
	case mongoproto.OpCodeDelete:
		protoOp1 = &mongoproto.DeleteOp{Header: op1.Header}
		protoOp2 = &mongoproto.DeleteOp{Header: op2.Header}
	case mongoproto.OpCodeGetMore:
		protoOp1 = &mongoproto.GetMoreOp{Header: op1.Header}
		protoOp2 = &mongoproto.GetMoreOp{Header: op2.Header}
	case mongoproto.OpCodeInsert:
		protoOp1 = &mongoproto.InsertOp{Header: op1.Header}
		protoOp2 = &mongoproto.InsertOp{Header: op2.Header}
	case mongoproto.OpCodeUpdate:
		protoOp1 = &mongoproto.UpdateOp{Header: op1.Header}
		protoOp2 = &mongoproto.UpdateOp{Header: op2.Header}
	case mongoproto.OpCodeKillCursors:
		protoOp1 = &mongoproto.KillCursorsOp{Header: op1.Header}
		protoOp2 = &mongoproto.KillCursorsOp{Header: op2.Header}
	default:
		return false, fmt.Errorf("Unknown Opcode: %d", op1.Header.OpCode)
	}

	if err := protoOp1.FromReader(reader1); err != nil {
		return false, err
	}
	if err := protoOp2.FromReader(reader2); err != nil {
		return false, err
	}
	equal := protoOp1.Equals(protoOp2)
	return equal, nil
}
