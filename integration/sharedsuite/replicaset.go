package sharedsuite

import (
	"net/url"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ReplicaSetToolArgs returns the command-line arguments that point a tool at the
// test deployment as a replica set, i.e. with the "<setName>/<seedlist>" form of
// --host rather than a single host.
func (s *IntegrationSuite) ReplicaSetToolArgs() []string {
	status := s.replicaSetStatus()
	s.Require().NotEmpty(status.SetName, "the server is a replica set member")
	s.Require().NotEmpty(status.Hosts, "the replica set reports its members")

	return s.toolArgsForHosts(
		status.Hosts,
		url.Values{"replicaSet": {status.SetName}},
		status.SetName+"/"+strings.Join(status.Hosts, ","),
	)
}

// replicaSetStatus is what the tests need to know about the replica set the
// server belongs to.
type replicaSetStatus struct {
	SetName string   `bson:"setName"`
	Primary string   `bson:"primary"`
	Hosts   []string `bson:"hosts"`
}

// replicaSetStatus asks the server about the replica set it belongs to. Callers
// assert on the fields they need, since a standalone answers this too, just
// without any of them.
func (s *IntegrationSuite) replicaSetStatus() replicaSetStatus {
	// isMaster rather than hello because the servers this runs against include
	// versions older than 5.0, where hello does not exist.
	var status replicaSetStatus
	err := s.Client().Database("admin").
		RunCommand(s.Context(), bson.D{{"isMaster", 1}}).
		Decode(&status)
	s.Require().NoError(err, "can ask the server about the replica set it belongs to")

	return status
}
