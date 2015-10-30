package mongoproto
import mgo "github.com/10gen/llmgo"
// OpKillCursors is used to close an active cursor in the database. This is necessary
// to ensure that database resources are reclaimed at the end of the query.
// http://docs.mongodb.org/meta-driver/latest/legacy/mongodb-wire-protocol/#op-kill-cursors
type KillCursorsOp struct {
	Header    MsgHeader
	mgo.KillCursorsOp
}
