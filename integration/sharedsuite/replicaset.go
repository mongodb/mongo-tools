package sharedsuite

import (
	"strings"

	"github.com/mongodb/mongo-tools/common/testopts"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ReplicaSetToolArgs returns the command-line arguments that point a tool at the
// test deployment as a replica set, i.e. with the "<setName>/<seedlist>" form of
// --host rather than a single host.
func (s *IntegrationSuite) ReplicaSetToolArgs() []string {
	setName, hosts := s.replicaSetNameAndHosts()

	return s.replicaSetArgsFor(setName + "/" + strings.Join(hosts, ","))
}

// ReplicaSetToolArgsForHosts is ReplicaSetToolArgs over a seedlist of just the
// given hosts. A seedlist naming only a secondary still has to reach the
// primary, because the set name makes the driver discover the rest of the set.
func (s *IntegrationSuite) ReplicaSetToolArgsForHosts(hosts ...string) []string {
	setName, _ := s.replicaSetNameAndHosts()

	return s.replicaSetArgsFor(setName + "/" + strings.Join(hosts, ","))
}

func (s *IntegrationSuite) replicaSetArgsFor(hostArg string) []string {
	args := append([]string{}, testopts.GetSSLArgs()...)
	if len(args) > 0 {
		// A "<setName>/..." host makes the driver discover the set's members and
		// connect to them under the names the set's own config uses, which are not
		// the names the test certificate was issued for.
		args = append(args, "--sslAllowInvalidHostnames")
	}
	args = append(args, testopts.GetAuthArgs()...)

	return append(args, "--host", hostArg)
}

// replicaSetNameAndHosts returns the set's name and the hosts of every member,
// as the set itself names them.
func (s *IntegrationSuite) replicaSetNameAndHosts() (string, []string) {
	status := s.replicaSetStatus()
	s.Require().NotEmpty(status.SetName, "the server is a replica set member")
	s.Require().NotEmpty(status.Hosts, "the replica set reports its members")

	return status.SetName, status.Hosts
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
