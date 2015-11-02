package mongoproto
import(
	"io"
	"fmt"

	mgo "github.com/10gen/llmgo"
)
// OpKillCursors is used to close an active cursor in the database. This is necessary
// to ensure that database resources are reclaimed at the end of the query.
// http://docs.mongodb.org/meta-driver/latest/legacy/mongodb-wire-protocol/#op-kill-cursors
type KillCursorsOp struct {
	Header    MsgHeader
	mgo.KillCursorsOp
}
func (op *KillCursorsOp) String() string {
	return  fmt.Sprintf("KillCursorsOp %v", op.CursorIds)

}
func (op *KillCursorsOp) OpCode() OpCode {
	return OpCodeKillCursors
}

func (op *KillCursorsOp) FromReader(r io.Reader) error {
	var b [8]byte
	_, err := io.ReadFull(r, b[:]) //skip ZERO and grab numberOfCursors
	if err != nil {
		return err
	}

	numCursors := uint32(getInt32(b[4:], 0))
	var i uint32
	for i = 0; i < numCursors; i++ {
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return nil
		}
		op.CursorIds = append(op.CursorIds, getInt64(b[:], 0))
	}
	return nil
}

func (op *KillCursorsOp) Execute(session *mgo.Session) error {
	if err := session.KillCursorsOp(&op.KillCursorsOp); err != nil {
		return err
	}

	fmt.Println("Kill cursors")
	return nil
}