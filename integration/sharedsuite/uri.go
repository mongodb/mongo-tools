package sharedsuite

import (
	"maps"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/mongodb/mongo-tools/common/testopts"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/connstring"
)

// toolArgsForHosts returns the command-line arguments that point a tool at the given hosts. When
// the deployment is configured through a URI, the arguments carry that URI's settings; otherwise
// hostArg is passed as --host.
func (s *IntegrationSuite) toolArgsForHosts(
	hosts []string,
	setParams url.Values,
	hostArg string,
) []string {
	uri, haveURI := s.configuredURIWithHosts(hosts, setParams)

	args := append([]string{}, testopts.GetSSLArgs()...)
	if len(args) > 0 || (haveURI && s.uriUsesTLS(uri)) {
		// The replica set names its members by hostnames the test certificate was not issued for.
		// This has to be a flag even when TLS comes from the URI, because the driver has no URI
		// option for it.
		args = append(args, "--sslAllowInvalidHostnames")
	}
	args = append(args, testopts.GetAuthArgs()...)

	if haveURI {
		return append(args, "--uri", uri)
	}

	return append(args, "--host", hostArg)
}

// configuredURIWithHosts returns the URI the test deployment was configured with, addressed at the
// given hosts instead of its own. Credentials, TLS settings, and every other option in the URI are
// kept, so a tool pointed at the new hosts connects the same way the suite's own client does.
//
// The returned URI never names a replica set, forces a direct connection, or names a database;
// callers add whichever of the first two they need through setParams. A database in the path would
// conflict with the tools' --db, so it is dropped, but kept as the authSource it implies when the
// URI has credentials. It returns false when no URI is configured, in which case the deployment's
// configuration all comes from the test env vars instead.
func (s *IntegrationSuite) configuredURIWithHosts(
	hosts []string,
	setParams url.Values,
) (string, bool) {
	uri := os.Getenv(testopts.URIEnvVar)
	if uri == "" {
		return "", false
	}

	// net/url cannot parse this because a seed list's comma-separated hosts are not a valid URL
	// authority, so the URI is split by hand.
	scheme, rest, found := strings.Cut(uri, "://")
	s.Require().True(found, "%#q has a scheme", testopts.URIEnvVar)

	authority, pathAndQuery := rest, ""
	if i := strings.IndexAny(rest, "/?"); i >= 0 {
		authority, pathAndQuery = rest[:i], rest[i:]
	}

	userInfo := ""
	if i := strings.LastIndex(authority, "@"); i >= 0 {
		userInfo = authority[:i+1]
	}

	path, rawQuery, _ := strings.Cut(pathAndQuery, "?")

	query, err := url.ParseQuery(rawQuery)
	s.Require().NoError(err, "%#q has a valid query string", testopts.URIEnvVar)

	for _, key := range []string{"replicaSet", "directConnection"} {
		deleteParamIgnoringCase(query, key)
	}
	db := strings.TrimPrefix(path, "/")
	if db != "" && userInfo != "" && !hasParamIgnoringCase(query, "authSource") {
		query.Set("authSource", db)
	}
	maps.Copy(query, setParams)

	newURI := scheme + "://" + userInfo + strings.Join(hosts, ",") + "/"
	if encoded := query.Encode(); encoded != "" {
		newURI += "?" + encoded
	}

	return newURI, true
}

// configuredHosts returns the hosts the test deployment was configured with:
// the seed list of the configured URI, or localhost and the default port when
// there is no URI and the configuration comes from the test env vars instead.
func (s *IntegrationSuite) configuredHosts() []string {
	uri := os.Getenv(testopts.URIEnvVar)
	if uri == "" {
		return []string{net.JoinHostPort("localhost", testopts.DefaultTestPort)}
	}

	// net/url cannot parse this because a seed list's comma-separated hosts are
	// not a valid URL authority, so the URI is split by hand.
	_, rest, found := strings.Cut(uri, "://")
	s.Require().True(found, "%#q has a scheme", testopts.URIEnvVar)

	authority := rest
	if i := strings.IndexAny(rest, "/?"); i >= 0 {
		authority = rest[:i]
	}
	if i := strings.LastIndex(authority, "@"); i >= 0 {
		authority = authority[i+1:]
	}

	return strings.Split(authority, ",")
}

// deleteParamIgnoringCase removes a URI option however it is capitalized, since URI option names
// are case-insensitive.
func deleteParamIgnoringCase(query url.Values, key string) {
	for existing := range query {
		if strings.EqualFold(existing, key) {
			delete(query, existing)
		}
	}
}

func hasParamIgnoringCase(query url.Values, key string) bool {
	for existing := range query {
		if strings.EqualFold(existing, key) {
			return true
		}
	}

	return false
}

// uriUsesTLS reports whether a URI turns TLS on, including implicitly through an option like
// tlsCAFile.
func (s *IntegrationSuite) uriUsesTLS(uri string) bool {
	cs, err := connstring.Parse(uri)
	s.Require().NoError(err, "the rewritten %#q is a valid connection string", testopts.URIEnvVar)

	return cs.SSL
}
