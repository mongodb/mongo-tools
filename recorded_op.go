package mongotape

type RecordedOp struct {
	RawOp
	Seen          *PreciseTime
	PlayAt        *PreciseTime `bson:",omitempty"`
	EOF           bool         `bson:",omitempty"`
	SrcEndpoint   string
	DstEndpoint   string
	ConnectionNum int64
	PlayedAt      *PreciseTime `bson:",omitempty"`
	Generation    int
	Order         int64
}

func (op *RecordedOp) ConnectionString() string {
	return op.SrcEndpoint + "->" + op.DstEndpoint
}
func (op *RecordedOp) ReversedConnectionString() string {
	return op.DstEndpoint + "->" + op.SrcEndpoint
}

type orderedOps []RecordedOp

func (o orderedOps) Len() int {
	return len(o)
}

func (o orderedOps) Less(i, j int) bool {
	return o[i].Seen.Before(o[j].Seen.Time)
}

func (o orderedOps) Swap(i, j int) {
	o[i], o[j] = o[j], o[i]
}

func (o *orderedOps) Pop() interface{} {
	i := len(*o) - 1
	op := (*o)[i]
	*o = (*o)[:i]
	return op
}

func (o *orderedOps) Push(op interface{}) {
	*o = append(*o, op.(RecordedOp))
}
