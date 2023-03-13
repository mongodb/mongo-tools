package db

import (
	"fmt"
	"github.com/mongodb/mongo-tools/common/util"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// OpTime represents the values to uniquely identify an oplog entry.
// An OpTime must always have a timestamp, but may or may not have a term.
// The hash is set uniquely up until (and including) version 4.0, but is set
// to zero in version 4.2+ with plans to remove it soon (see SERVER-36334).
type OpTime struct {
	Timestamp primitive.Timestamp `json:"timestamp"`
	Term      *int64              `json:"term"`
	Hash      *int64              `json:"hash"`
}

// GetOpTimeFromOplogEntry returns an OpTime struct from the relevant fields in an Oplog struct.
func GetOpTimeFromOplogEntry(oplogEntry *Oplog) OpTime {
	return OpTime{
		Timestamp: oplogEntry.Timestamp,
		Term:      oplogEntry.Term,
		Hash:      oplogEntry.Hash,
	}
}

// OpTimeIsEmpty returns true if opTime is uninitialized, false otherwise.
func OpTimeIsEmpty(opTime OpTime) bool {
	return opTime == OpTime{}
}

// OpTimeEquals returns true if lhs equals rhs, false otherwise.
// We first check for nil / not nil mismatches between the terms and
// between the hashes. Then we check for equality between the terms and
// between the hashes (if they exist) before checking the timestamps.
func OpTimeEquals(lhs OpTime, rhs OpTime) bool {
	if (lhs.Term == nil && rhs.Term != nil) || (lhs.Term != nil && rhs.Term == nil) ||
		(lhs.Hash == nil && rhs.Hash != nil) || (lhs.Hash != nil && rhs.Hash == nil) {
		return false
	}

	termsBothNilOrEqual := true
	if lhs.Term != nil && rhs.Term != nil {
		termsBothNilOrEqual = *lhs.Term == *rhs.Term
	}

	hashesBothNilOrEqual := true
	if lhs.Hash != nil && rhs.Hash != nil {
		hashesBothNilOrEqual = *lhs.Hash == *rhs.Hash
	}

	return lhs.Timestamp.Equal(rhs.Timestamp) && termsBothNilOrEqual && hashesBothNilOrEqual
}

// OpTimeLessThan returns true if lhs comes before rhs, false otherwise.
// We first check if both the terms exist. If they don't or they're equal,
// we compare just the timestamps.
func OpTimeLessThan(lhs OpTime, rhs OpTime) bool {
	if lhs.Term != nil && rhs.Term != nil {
		if *lhs.Term == *rhs.Term {
			return util.TimestampLessThan(lhs.Timestamp, rhs.Timestamp)
		}
		return *lhs.Term < *rhs.Term
	}

	return util.TimestampLessThan(lhs.Timestamp, rhs.Timestamp)
}

// OpTimeGreaterThan returns true if lhs comes after rhs, false otherwise.
// We first check if both the terms exist. If they don't or they're equal,
// we compare just the timestamps.
func OpTimeGreaterThan(lhs OpTime, rhs OpTime) bool {
	if lhs.Term != nil && rhs.Term != nil {
		if *lhs.Term == *rhs.Term {
			return util.TimestampGreaterThan(lhs.Timestamp, rhs.Timestamp)
		}
		return *lhs.Term > *rhs.Term
	}

	return util.TimestampGreaterThan(lhs.Timestamp, rhs.Timestamp)
}

func (ot OpTime) String() string {
	if ot.Term != nil && ot.Hash != nil {
		return fmt.Sprintf("{Timestamp: %v, Term: %v, Hash: %v}", ot.Timestamp, *ot.Term, *ot.Hash)
	} else if ot.Term == nil && ot.Hash != nil {
		return fmt.Sprintf("{Timestamp: %v, Term: %v, Hash: %v}", ot.Timestamp, nil, *ot.Hash)
	} else if ot.Term != nil && ot.Hash == nil {
		return fmt.Sprintf("{Timestamp: %v, Term: %v, Hash: %v}", ot.Timestamp, *ot.Term, nil)
	}

	return fmt.Sprintf("{Timestamp: %v, Term: %v, Hash: %v}", ot.Timestamp, nil, nil)
}
