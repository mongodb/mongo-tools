package mongotape

import (
	mgo "github.com/10gen/llmgo"
	"github.com/10gen/llmgo/bson"
	"io"
	"os"
	"testing"
)

type verifyFunc func(*testing.T, *mgo.Session)

func TestOpCommandFromPcapFile(t *testing.T) {
	if err := teardownDB(); err != nil {
		t.Error(err)
	}

	pcapFname := "op_command_2inserts.pcap"

	var verifier verifyFunc = func(t *testing.T, session *mgo.Session) {
		coll := session.DB(testDB).C(testCollection)
		iter := coll.Find(bson.D{}).Sort("op_command_test").Iter()
		result := struct {
			TestNum int `bson:"op_command_test"`
		}{}

		t.Log("Querying database to ensure insert occured successfully")
		ind := 1
		for iter.Next(&result) {
			if result.TestNum != ind {
				t.Errorf("document number not matched. Expected: %v -- found %v --", ind, result.TestNum)
			}
			ind++
		}
	}
	pcapTestHelper(t, pcapFname, verifier)
	if err := teardownDB(); err != nil {
		t.Error(err)
	}

}

func pcapTestHelper(t *testing.T, pcapFname string, verifier verifyFunc) {
	playbackFname := "pcap_test_run.tape"
	streamSettings := OpStreamSettings{
		PcapFile:      "testPcap/" + pcapFname,
		PacketBufSize: 1000,
	}
	t.Log("Opening op stream")
	ctx, err := getOpstream(streamSettings)
	if err != nil {
		t.Errorf("error opening opstream: %v\n", err)
	}

	playbackWriter, err := NewPlaybackWriter(playbackFname, false)
	defer os.Remove(playbackFname)
	if err != nil {
		t.Errorf("error opening playback file to write: %v\n", err)
	}

	t.Log("Recording playbackfile from pcap file")
	err = Record(ctx, playbackWriter)
	if err != nil {
		t.Errorf("error makign tape file: %v\n", err)
	}

	playbackReader, err := NewPlaybackFileReader(playbackFname, false)
	if err != nil {
		t.Errorf("error opening playback file to write: %v\n", err)
	}

	opChan, errChan := NewOpChanFromFile(playbackReader, 1)

	statCollector, _ := newStatCollector(testCollectorOpts, true, true)
	//statRec := statCollector.StatRecorder.(*BufferedStatRecorder)
	context := NewExecutionContext(statCollector)

	t.Log("Reading ops from playback file")
	err = Play(context, opChan, testSpeed, currentTestUrl, 1, 10)
	if err != nil {
		t.Errorf("error playing back recorded file: %v\n", err)
	}
	err = <-errChan
	if err != io.EOF {
		t.Errorf("error reading ops from file: %v\n", err)
	}
	//prepare a query for the database
	session, err := mgo.Dial(currentTestUrl)
	if err != nil {
		t.Errorf("Error connecting to test server: %v", err)
	}
	verifier(t, session)
}
