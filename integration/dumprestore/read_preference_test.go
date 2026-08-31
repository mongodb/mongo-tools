package dumprestore

import (
	"context"
	"time"

	"github.com/mongodb/mongo-tools/common/bsonutil"
	"github.com/mongodb/mongo-tools/common/testtype"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// TestDumpHonorsTaggedReadPreference checks that a --readPreference naming a tag
// set sends the dump's reads to the node carrying that tag, and to no other node.
// One secondary is tagged, so it is the only member the read preference can select,
// and every other node is asserted to have served no read of the dumped collections
// at all.
//
// Parsing of the --readPreference argument itself, tag sets included, is covered by
// unit tests in common/db/read_preferences_test.go.
func (s *DumpRestoreSuite) TestDumpHonorsTaggedReadPreference() {
	testtype.SkipUnlessTestType(s.T(), testtype.MultiNodeReplSetTestType)

	// The read preference is passed to the tool as a string, so the tag it names has
	// to be kept in step with the tag the member is given by hand.
	const dumpTargetReadPreference = `{mode: "nearest", tagSets: [{use: "dumpTarget"}]}`
	dumpTargetTag := bson.D{{"use", "dumpTarget"}}

	collNames := []string{"bar", "baz", "bam"}

	testDB := s.database("read_preference_tags")
	for _, collName := range collNames {
		s.insertNamespacedDocs(s.fullSetWriteCollection(testDB, collName))
	}

	taggedHost := s.SecondaryHosts()[0]
	s.tagReplicaSetMember(taggedHost, dumpTargetTag)

	nodes := s.profileEveryNode(testDB.Name())

	s.dumpWithArgs(
		append(s.ReplicaSetToolArgs(), "--readPreference", dumpTargetReadPreference),
		"--out", s.T().TempDir(),
		"--db", testDB.Name(),
	)

	for host, profile := range nodes {
		reads := s.countProfiledReads(host, profile, testDB.Name(), collNames)
		if host == taggedHost {
			s.Assert().NotZero(
				reads,
				"the tagged secondary %s served the dump's reads",
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

// tagReplicaSetMember gives one member of the set a tag and puts its original tags
// back afterwards. Only one member is tagged, so that the tag identifies exactly one
// node the read preference can select, and only that member's tags are touched, so
// the reconfiguration cannot trigger an election.
func (s *DumpRestoreSuite) tagReplicaSetMember(host string, tags bson.D) {
	// The context the suite hands out belongs to the running test, so restoring the
	// set's configuration afterwards needs one that outlives the test.
	teardownCtx := context.WithoutCancel(s.Context())

	originalTags := s.memberTags(s.Context(), host)
	s.T().Cleanup(func() {
		// Assert rather than Require: a failure to put the configuration back is
		// worth reporting but must not abort the remaining cleanups, and it is not
		// a failure of what this test set out to check.
		s.Assert().NoError(
			s.setMemberTags(teardownCtx, host, originalTags),
			"%s gets its original tags back",
			host,
		)
	})

	s.Require().NoError(
		s.setMemberTags(s.Context(), host, tags),
		"can tag the replica set member %s",
		host,
	)
	s.waitForMemberTags(host, tags)
}

func (s *DumpRestoreSuite) memberTags(ctx context.Context, host string) bson.D {
	client := s.primaryClient(ctx)
	defer s.disconnect(ctx, client)

	member := s.memberConfig(s.replicaSetConfig(ctx, client), host)
	tags, ok := bsonLookUp(member, "tags").(bson.D)
	if !ok {
		return bson.D{}
	}

	return tags
}

// setMemberTags replaces one member's tags, leaving the rest of the configuration
// as the set currently has it. Only the reconfiguration's own error is returned, so
// that a caller restoring the original tags can report a failure without aborting
// the rest of its cleanup.
func (s *DumpRestoreSuite) setMemberTags(ctx context.Context, host string, tags bson.D) error {
	client := s.primaryClient(ctx)
	defer s.disconnect(ctx, client)

	// Read the configuration as it is now and change one field of it, rather than
	// replaying a copy captured earlier. A captured copy carries the version and
	// election term it was read at, which the server rejects once an election has
	// moved them on, and it would silently undo any other change made in between.
	config := s.replicaSetConfig(ctx, client)
	tagged := s.withMemberTags(config, host, tags)

	version, ok := bsonLookUp(config, "version").(int32)
	s.Require().True(ok, "the replica set configuration has an integer version")

	return client.Database("admin").
		RunCommand(ctx, bson.D{{"replSetReconfig", bsonReplaceKey(tagged, "version", version+1)}}).
		Err()
}

// primaryClient connects to whichever node is primary now. A reconfiguration is only
// accepted there, and the node the suite's own client reaches is not guaranteed to
// be it.
func (s *DumpRestoreSuite) primaryClient(ctx context.Context) *mongo.Client {
	seed := s.Client()
	defer s.disconnect(ctx, seed)

	// This repeats what sharedsuite.PrimaryHost does, because that takes its context
	// from the running test and this is also called once the test has finished.
	var isMaster struct {
		Primary string `bson:"primary"`
	}
	err := seed.Database("admin").
		RunCommand(ctx, bson.D{{"isMaster", 1}}).
		Decode(&isMaster)
	s.Require().NoError(err, "can ask the server which node is primary")
	s.Require().NotEmpty(isMaster.Primary, "the replica set has a primary")

	return s.DirectClient(isMaster.Primary)
}

func (s *DumpRestoreSuite) disconnect(ctx context.Context, client *mongo.Client) {
	s.Assert().NoError(client.Disconnect(ctx), "can disconnect from the server")
}

func (s *DumpRestoreSuite) replicaSetConfig(ctx context.Context, client *mongo.Client) bson.D {
	var reply struct {
		Config bson.D `bson:"config"`
	}
	err := client.Database("admin").
		RunCommand(ctx, bson.D{{"replSetGetConfig", 1}}).
		Decode(&reply)
	s.Require().NoError(err, "can read the replica set configuration")
	s.Require().NotEmpty(reply.Config, "the replica set configuration is not empty")

	return reply.Config
}

func (s *DumpRestoreSuite) withMemberTags(config bson.D, host string, tags bson.D) bson.D {
	members := s.members(config)

	tagged := make(bson.A, len(members))
	for i, member := range members {
		if bsonLookUp(member, "host") == host {
			member = bsonReplaceKey(member, "tags", tags)
		}
		tagged[i] = member
	}

	return bsonReplaceKey(config, "members", tagged)
}

func (s *DumpRestoreSuite) memberConfig(config bson.D, host string) bson.D {
	for _, member := range s.members(config) {
		if bsonLookUp(member, "host") == host {
			return member
		}
	}

	s.Require().Failf(
		"no member of the replica set has that host",
		"%s is one of the replica set's members",
		host,
	)

	return nil
}

func (s *DumpRestoreSuite) members(config bson.D) []bson.D {
	entries, ok := bsonLookUp(config, "members").(bson.A)
	s.Require().True(ok, "the replica set configuration lists its members")

	members := make([]bson.D, len(entries))
	for i, entry := range entries {
		member, ok := entry.(bson.D)
		s.Require().True(ok, "member %d of the replica set is a document", i)
		members[i] = member
	}

	return members
}

// bsonLookUp returns the value of key in doc, or nil when it is not there. The
// tests here read a replica set configuration, where a missing key and a key
// whose value is not what was wanted are the same failure.
func bsonLookUp(doc bson.D, key string) any {
	value, err := bsonutil.FindValueByKey(key, &doc)
	if err != nil {
		return nil
	}

	return value
}

// bsonReplaceKey returns doc with key set to value, in place if the key is already
// there. Appending instead would leave the document with the key twice, which the
// server reads as whichever copy it sees first.
func bsonReplaceKey(doc bson.D, key string, value any) bson.D {
	updated := make(bson.D, 0, len(doc)+1)
	replaced := false
	for _, elem := range doc {
		if elem.Key == key {
			elem.Value = value
			replaced = true
		}
		updated = append(updated, elem)
	}
	if !replaced {
		updated = append(updated, bson.E{key, value})
	}

	return updated
}

// waitForMemberTags waits until the tagged node reports its new tags itself. The
// driver learns a member's tags from that node's own handshake reply, so a dump
// started before the reconfiguration reaches the node would find no member carrying
// the tag and fail server selection instead.
func (s *DumpRestoreSuite) waitForMemberTags(host string, tags bson.D) {
	client := s.DirectClient(host)
	defer s.disconnect(s.Context(), client)

	s.Require().Eventually(
		func() bool {
			var isMaster struct {
				Tags bson.D `bson:"tags"`
			}
			err := client.Database("admin").
				RunCommand(s.Context(), bson.D{{"isMaster", 1}}).
				Decode(&isMaster)

			return err == nil && hasTags(isMaster.Tags, tags)
		},
		tagPropagationTimeout,
		tagPollInterval,
		"%s reports the tags it was just given",
		host,
	)
}

const (
	tagPropagationTimeout = 30 * time.Second
	tagPollInterval       = 100 * time.Millisecond
)

func hasTags(got, want bson.D) bool {
	for _, tag := range want {
		if bsonLookUp(got, tag.Key) != tag.Value {
			return false
		}
	}

	return true
}

// profileEveryNode turns on profiling for dbName on every node of the set and
// returns each node's own profile collection, keyed by host. Profiling and the
// collection it writes to are per-node and are not replicated, so each has to be
// reached over a connection to that one node.
func (s *DumpRestoreSuite) profileEveryNode(dbName string) map[string]*mongo.Collection {
	teardownCtx := context.WithoutCancel(s.Context())

	hosts := append([]string{s.PrimaryHost()}, s.SecondaryHosts()...)

	profiles := map[string]*mongo.Collection{}
	for _, host := range hosts {
		client := s.DirectClient(host)
		s.T().Cleanup(func() { s.disconnect(teardownCtx, client) })

		nodeDB := client.Database(dbName)
		s.setProfilingLevel(nodeDB, profileEverything)
		s.T().Cleanup(func() {
			s.Assert().NoError(
				setProfilingLevel(teardownCtx, nodeDB, profilingOff),
				"profiling is turned back off on %s",
				host,
			)
		})

		profiles[host] = nodeDB.Collection("system.profile")
	}

	return profiles
}

// countProfiledReads counts the reads of the given collections that one node
// profiled. Only document reads are counted: the metadata commands a dump also
// runs go to the primary whatever the read preference is, so counting every
// profiled operation would be counting those too.
func (s *DumpRestoreSuite) countProfiledReads(
	host string,
	profile *mongo.Collection,
	dbName string,
	collNames []string,
) int64 {
	namespaces := make(bson.A, len(collNames))
	for i, collName := range collNames {
		namespaces[i] = dbName + "." + collName
	}

	count, err := profile.CountDocuments(
		s.Context(),
		bson.D{
			{"ns", bson.D{{"$in", namespaces}}},
			{"op", bson.D{{"$in", bson.A{"query", "getmore"}}}},
		},
	)
	s.Require().NoError(err, "can count the profiled reads on %s", host)

	return count
}
