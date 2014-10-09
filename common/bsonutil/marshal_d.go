package bsonutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gopkg.in/mgo.v2/bson"
)

// MarshalD is a wrapper for bson.D that allows unmarshalling
// of bson.D with preserved order. Necessary for printing
// certain database commands
type MarshalD bson.D

// MarshalJSON makes the MarshalD type usable by
// the encoding/json package
func (md MarshalD) MarshalJSON() ([]byte, error) {
	var buff bytes.Buffer
	buff.WriteString("{")
	for i, item := range md {
		key := fmt.Sprintf(`"%s":`, item.Name)
		val, err := json.Marshal(item.Value)
		if err != nil {
			return nil, fmt.Errorf("cannot marshal %v: %v", item.Value, err)
		}
		buff.WriteString(key)
		buff.Write(val)
		if i != len(md)-1 {
			buff.WriteString(",")
		}
	}
	buff.WriteString("}")
	return buff.Bytes(), nil
}

// MakeSortString takes a bson.D object and converts it to a slice of strings
// that can be used as the input args to mgo's .Sort(...) function.
// For example:
// {a:1, b:-1} -> ["+a", "-b"]
func MakeSortString(sortObj bson.D) ([]string, error) {
	sortStrs := make([]string, 0, len(sortObj))
	for _, docElem := range sortObj {
		valueAsNumber := float64(0)
		switch v := docElem.Value.(type) {
		case float64:
			valueAsNumber = v
		default:
			return nil, fmt.Errorf("sort direction must be numeric type")
		}
		prefix := "+"
		if valueAsNumber < 0 {
			prefix = "-"
		}
		sortStrs = append(sortStrs, fmt.Sprintf("%v%v", prefix, docElem.Name))
	}
	return sortStrs, nil
}
