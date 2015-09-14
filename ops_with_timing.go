package mongocaputils

import (
	"bytes"
	"fmt"
	"gopkg.in/mgo.v2"
	"time"

	"github.com/gabrielrussell/mongocaputils/mongoproto"
)

type OpWithTime struct {
	mongoproto.OpRaw
	Seen       time.Time
	PlayAt     time.Time `bson:",omitempty"`
	EOF        bool      `bson:",omitempty"`
	Connection string
}

type orderedOps []OpWithTime

func (o orderedOps) Len() int {
	return len(o)
}

func (o orderedOps) Less(i, j int) bool {
	return o[i].Seen.Before(o[j].Seen)
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
	*o = append(*o, op.(OpWithTime))
}

func (o *OpWithTime) Execute(session *mgo.Session) error {
	reader := bytes.NewReader(o.OpRaw.Body)
	fmt.Printf("%v %v\n", o.OpRaw.Header, len(o.OpRaw.Body))
	switch o.OpRaw.Header.OpCode {
	case mongoproto.OpCodeQuery:
		opQuery := &mongoproto.OpQuery{Header: o.OpRaw.Header}
		err := opQuery.FromReader(reader)
		if err != nil {
			return err
		}
		return opQuery.Execute(session)
	case mongoproto.OpCodeGetMore:
		opGetMore := &mongoproto.OpGetMore{Header: o.OpRaw.Header}
		err := opGetMore.FromReader(reader)
		if err != nil {
			return err
		}
		return opGetMore.Execute(session)
	case mongoproto.OpCodeInsert:
		opInsert := &mongoproto.OpInsert{Header: o.OpRaw.Header}
		err := opInsert.FromReader(reader)
		if err != nil {
			return err
		}
		return opInsert.Execute(session)
	default:
		fmt.Printf("OpWithTime Execute unknown\n")
		opUnknown := &mongoproto.OpUnknown{Header: o.OpRaw.Header}
		err := opUnknown.FromReader(reader)
		if err != nil {
			return err
		}
		return opUnknown.Execute(session)
	}
	return nil
}
