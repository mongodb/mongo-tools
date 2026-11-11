//go:build integration

package dumprestore

import (
	"github.com/mongodb/mongo-tools/common/bsonutil"
	"github.com/mongodb/mongo-tools/common/testutil"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// The tag that scripts/start-test-cluster.sh puts on exactly one member of a replica set. The read
// preference below names it, so the member carrying it is the only one that preference can
// select. The value has to be kept in step with the one in the script, because the read preference
// reaches the tool as a string.
const dumpTargetTag = "dumpTarget"

// TestDumpHonorsTaggedReadPreference checks that a `--readPreference` naming a tag set sends the
// dump's reads to the node carrying that tag, and to no other node. The test cluster starts with
// exactly one secondary tagged, and that secondary is unelectable, so it is the only member the
// read preference can select and it can never be the primary: a dump that ignored the read
// preference and read the primary would send its reads to a node the preference cannot select.
//
// Which node served a read is observed through that node's profiler. Profiling is turned on for
// the test's database on every node, and a node's `system.profile` holds the client operations
// it served. The profiler records only client operations, so mongod's own internal reads (the
// logical-session maintenance the primary runs every few minutes, for instance) cannot appear
// here the way they can in `serverStatus` opcounters (TOOLS-4495), and nothing but this dump
// reads the test's database, so the counts are exactly the dump's reads.
//
// Only finds are counted, by matching the profiler's op type "query": mongodump's document reads
// are finds, while the metadata commands a dump also runs (listCollections, listIndexes, count)
// are recorded with op type "command", as is the count that reads them.
//
// Parsing of the `--readPreference` argument itself, tag sets included, is covered by unit tests in
// common/db/read_preferences_test.go.
func (s *DumpRestoreSuite) TestDumpHonorsTaggedReadPreference() {
	testutil.SkipUnlessReplicaSet(s.T())
	testutil.SkipForDisaggregatedStorage(s.T(), "disagg cluster nodes are not tagged")

	readPreference := `{mode: "nearest", tagSets: [{use: "` + dumpTargetTag + `"}]}`

	collNames := []string{"bar", "baz", "bam"}

	testDB := s.database("read_preference_tags")
	for _, collName := range collNames {
		s.insertNamespacedDocs(s.fullSetWriteCollection(testDB, collName))
	}

	tags := s.nodeTags()

	tagged := []string{}
	for host, hasTag := range tags {
		if hasTag {
			tagged = append(tagged, host)
		}
	}
	s.Require().Len(
		tagged,
		1,
		"exactly one node carries the tag %#q, which the test cluster's own configuration gives it",
		dumpTargetTag,
	)
	s.Require().NotEqual(
		s.PrimaryHost(),
		tagged[0],
		"the tagged node is a secondary, not the primary, which the test cluster's own "+
			"configuration guarantees by making it unelectable",
	)

	s.enableProfiling(testDB.Name())

	s.dumpWithArgs(
		append(s.ReplicaSetToolArgs(), "--readPreference", readPreference),
		"--out", s.T().TempDir(),
		"--db", testDB.Name(),
	)

	for host, hasTag := range tags {
		reads := s.profiledQueries(host, testDB.Name())
		if hasTag {
			s.Assert().NotZero(
				reads,
				"the tagged node %s served the dump's reads",
				host,
			)
		} else {
			s.Assert().Zero(
				reads,
				"%s served none of the dump's reads, because it does not carry the tag",
				host,
			)
		}
	}
}

// nodeTags reports whether each node of the replica set carries the read preference's tag. It
// asks every node over a direct connection, because the tags a read preference selects by are
// the ones the node itself reports in its handshake, and those are not replicated.
func (s *DumpRestoreSuite) nodeTags() map[string]bool {
	tags := map[string]bool{}
	for _, host := range append([]string{s.PrimaryHost()}, s.SecondaryHosts()...) {
		client := s.DirectClient(host)
		tags[host] = s.hasDumpTargetTag(client)
		s.disconnect(client)
	}

	return tags
}

// enableProfiling turns the profiler on for dbName on every node of the replica set, over direct
// connections: the level is per database and per node, and the reads the test counts may be
// served by any node.
func (s *DumpRestoreSuite) enableProfiling(dbName string) {
	for _, host := range append([]string{s.PrimaryHost()}, s.SecondaryHosts()...) {
		client := s.DirectClient(host)
		res := client.Database(dbName).RunCommand(s.Context(), bson.D{{"profile", 2}})
		s.Require().NoError(res.Err(), "can enable profiling on %#q on %s", dbName, host)
		s.disconnect(client)
	}
}

// profiledQueries counts the finds the profiler recorded for dbName on one node, over a direct
// connection so the count is that node's own.
func (s *DumpRestoreSuite) profiledQueries(host, dbName string) int64 {
	client := s.DirectClient(host)
	count, err := client.Database(dbName).Collection("system.profile").
		CountDocuments(s.Context(), bson.D{{"op", "query"}})
	s.Require().NoError(err, "can read the profiler's records for %#q on %s", dbName, host)
	s.disconnect(client)

	return count
}

func (s *DumpRestoreSuite) hasDumpTargetTag(client *mongo.Client) bool {
	var isMaster struct {
		Tags bson.D `bson:"tags"`
	}
	err := client.Database("admin").
		RunCommand(s.Context(), bson.D{{"isMaster", 1}}).
		Decode(&isMaster)

	return err == nil && bsonLookUp(isMaster.Tags, "use") == dumpTargetTag
}

func (s *DumpRestoreSuite) disconnect(client *mongo.Client) {
	s.Assert().NoError(client.Disconnect(s.Context()), "can disconnect from the server")
}

// bsonLookUp returns the value of key in doc, or nil when it is not there. The tests here read a
// node's handshake tags, where a missing key and a key whose value is not what was wanted are the
// same failure.
func bsonLookUp(doc bson.D, key string) any {
	value, err := bsonutil.FindValueByKey(key, &doc)
	if err != nil {
		return nil
	}

	return value
}
