package db

// MongoCanAcceptLiteralZeroTimestamp indicates whether the given server
// version can accept a literal zero timestamp in a query. See SERVER-88750
// and TOOLS-3540.
func MongoCanAcceptLiteralZeroTimestamp(version Version) bool {
	// bypassEmptyTsReplacement was released with 8.0.
	if version[0] >= 8 {
		return true
	}

	// bypassEmptyTsReplacement was backported to 7.0, 6.0, and 5.0.
	// No other minor releases received it.
	if version[1] != 0 {
		return false
	}

	switch version[0] {
	case 7:
		return version[2] >= 13
	case 6:
		return version[2] >= 17
	case 5:
		return version[2] >= 29
	default:
		return false
	}
}
