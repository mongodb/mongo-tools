package generate

// TODO: shyam - edit comments
// TODO: shyam - remove mention of mongodump-task-gen
// Package "generate" is used for both mongosync task-generator and
// mongodump-suite-gen. The default behavior is appropriate for mongosync,
// but we can change some variables to get the mongodump-task-gen behavior.

var isMongodumpTaskGen = false

// Initialize variables in the suite package for mongodump-suite-gen.
func InitForMongodumpTaskGen() {
	isMongodumpTaskGen = true
	// Add calls here to initialize other package vars as needed.
	initVersionCombinationsForMongodump()
}

// Is this initialized for mongodump-suite-gen ?
func IsMongodumpTaskGen() bool {
	return isMongodumpTaskGen
}

// Is this initialized for (mongosync) suite-generator ?
func IsMongosyncTaskGen() bool {
	return !isMongodumpTaskGen
}

func getResmokeVariant() string {
	if isMongodumpTaskGen {
		return "mongodump_passthru_v"
	}
	return "amazon2-arm64"
}
