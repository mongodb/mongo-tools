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
	args := append([]string{}, testopts.GetSSLArgs()...)
	if len(args) > 0 {
		// A "<setName>/..." host makes the driver discover the set's members and
		// connect to them under the names the set's own config uses, which are not
		// the names the test certificate was issued for.
		args = append(args, "--sslAllowInvalidHostnames")
	}
	args = append(args, testopts.GetAuthArgs()...)

	return append(args, "--host", s.replicaSetHostArg())
}

// replicaSetHostArg builds the "<setName>/<host>,<host>" form of --host out of
// what the server reports about the set it belongs to.
func (s *IntegrationSuite) replicaSetHostArg() string {
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

	return isMaster.SetName + "/" + strings.Join(isMaster.Hosts, ",")
}
