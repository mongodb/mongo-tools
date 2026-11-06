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
// Which node served a read is observed through that node's own serverStatus opcounters. They are
// per-node and not replicated. Only the query counter is read: mongodump's document reads each
// begin with a query (a find per collection), the metadata commands a dump also runs count under
// `command` instead, and getMore is left out because it cannot be attributed to the dump - a
// replica set member fetching oplog from its sync source makes ordinary getMores on that sync
// source, which a counter cannot tell apart from a dump's. The change in queries across the dump is
// therefore exactly the dump's document reads.
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

	before := s.nodeReadCounts()

	tagged := []string{}
	for host, count := range before {
		if count.tagged {
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

	s.dumpWithArgs(
		append(s.ReplicaSetToolArgs(), "--readPreference", readPreference),
		"--out", s.T().TempDir(),
		"--db", testDB.Name(),
	)

	after := s.nodeReadCounts()

	for host, count := range before {
		reads := after[host].queries - count.queries
		if count.tagged {
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

// nodeReadCount is what one node of the replica set tells about itself: whether it carries the tag
// the read preference names, and how many queries it has served since it started.
type nodeReadCount struct {
	tagged  bool
	queries int64
}

// nodeReadCounts asks every node of the replica set, over a direct connection to that one node,
// because neither thing it reports is replicated: serverStatus describes one node only, and the
// tags a read preference selects by are the ones the node itself reports in its handshake.
func (s *DumpRestoreSuite) nodeReadCounts() map[string]nodeReadCount {
	counts := map[string]nodeReadCount{}
	for _, host := range append([]string{s.PrimaryHost()}, s.SecondaryHosts()...) {
		client := s.DirectClient(host)
		counts[host] = nodeReadCount{
			tagged:  s.hasDumpTargetTag(client),
			queries: s.queryCount(client),
		}
		s.disconnect(client)
	}

	return counts
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

// queryCount reads only the query counter, not getMore: a replica set member fetching oplog from
// its sync source makes ordinary getMores on that sync source, so the getMore counter counts
// replication's traffic too, while the query counter counts only clients' finds.
func (s *DumpRestoreSuite) queryCount(client *mongo.Client) int64 {
	var status struct {
		Opcounters struct {
			Query int64 `bson:"query"`
		} `bson:"opcounters"`
	}
	err := client.Database("admin").
		RunCommand(s.Context(), bson.D{{"serverStatus", 1}}).
		Decode(&status)
	s.Require().NoError(err, "can read the server's own status")

	return status.Opcounters.Query
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
