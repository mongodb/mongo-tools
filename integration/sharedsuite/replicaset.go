package sharedsuite

import (
	"strings"

	"github.com/mongodb/mongo-tools/common/testutil"
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
	args := append([]string{}, testutil.GetSSLArgs()...)
	if len(args) > 0 {
		// A "<setName>/..." host makes the driver discover the set's members and
		// connect to them under the names the set's own config uses, which are not
		// the names the test certificate was issued for.
		args = append(args, "--sslAllowInvalidHostnames")
	}
	args = append(args, testutil.GetAuthArgs()...)

	return append(args, "--host", hostArg)
}

// replicaSetNameAndHosts returns the set's name and the hosts of every member,
// as the set itself names them.
func (s *IntegrationSuite) replicaSetNameAndHosts() (string, []string) {
	// isMaster rather than hello because the servers this runs against include
	// versions older than 5.0, where hello does not exist.
	var isMaster struct {
		SetName string   `bson:"setName"`
		Hosts   []string `bson:"hosts"`
	}
	err := s.Client().Database("admin").
		RunCommand(s.Context(), bson.D{{"isMaster", 1}}).
		Decode(&isMaster)
	s.Require().NoError(err, "can ask the server which replica set it belongs to")
	s.Require().NotEmpty(isMaster.SetName, "the server is a replica set member")
	s.Require().NotEmpty(isMaster.Hosts, "the replica set reports its members")

	return isMaster.SetName, isMaster.Hosts
}
