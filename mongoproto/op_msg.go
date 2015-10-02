package mongoproto

// OpMsg sends a diagnostic message to the database. The database sends back a fixed response.
// OpMsg is Deprecated
// http://docs.mongodb.org/meta-driver/latest/legacy/mongodb-wire-protocol/#op-msg
type OpMsg struct {
	Header  MsgHeader
	Message string
}
