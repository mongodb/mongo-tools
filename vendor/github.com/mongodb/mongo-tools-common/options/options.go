// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Package options implements command-line options that are used by all of
// the mongo tools.
package options

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	flags "github.com/jessevdk/go-flags"
	"github.com/mongodb/mongo-tools-common/failpoint"
	"github.com/mongodb/mongo-tools-common/log"
	"github.com/mongodb/mongo-tools-common/util"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
	"go.mongodb.org/mongo-driver/x/mongo/driver/connstring"
)

var (
	KnownURIOptionsAuth           = []string{"authsource", "authmechanism"}
	KnownURIOptionsConnection     = []string{"connecttimeoutms"}
	KnownURIOptionsSSL            = []string{"ssl"}
	KnownURIOptionsReadPreference = []string{"readpreference"}
	KnownURIOptionsKerberos       = []string{"gssapiservicename", "gssapihostname"}
	KnownURIOptionsWriteConcern   = []string{"wtimeout", "w", "j", "fsync"}
	KnownURIOptionsReplicaSet     = []string{"replicaset"}
)

// XXX Force these true as the Go driver supports them always.  Once the
// conditionals that depend on them are removed, these can be removed.
var (
	BuiltWithSSL    = true
	BuiltWithGSSAPI = true
)

const IncompatibleArgsErrorFormat = "illegal argument combination: cannot specify %s and --uri"

func ConflictingArgsErrorFormat(optionName, uriValue, cliValue, cliOptionName string) error {
	return fmt.Errorf("Invalid Options: Cannot specify different %s in connection URI and command-line option (\"%s\" was specified in the URI and \"%s\" was specified in the %s option)", optionName, uriValue, cliValue, cliOptionName)
}

// Struct encompassing all of the options that are reused across tools: "help",
// "version", verbosity settings, ssl settings, etc.
type ToolOptions struct {

	// The name of the tool
	AppName string

	// The version of the tool
	VersionStr string

	// The git commit reference of the tool
	GitCommit string

	// Sub-option types
	*URI
	*General
	*Verbosity
	*Connection
	*SSL
	*Auth
	*Kerberos
	*Namespace

	// Force direct connection to the server and disable the
	// drivers automatic repl set discovery logic.
	Direct bool

	// ReplicaSetName, if specified, will prevent the obtained session from
	// communicating with any server which is not part of a replica set
	// with the given name. The default is to communicate with any server
	// specified or discovered via the servers contacted.
	ReplicaSetName string

	// ReadPreference, if specified, sets the client default
	ReadPreference *readpref.ReadPref

	// WriteConcern, if specified, sets the client default
	WriteConcern *writeconcern.WriteConcern

	// RetryWrites, if specified, sets the client default.
	RetryWrites *bool

	// for caching the parser
	parser *flags.Parser

	// for checking which options were enabled on this tool
	enabledOptions EnabledOptions

	// Will attempt to parse positional arguments as connection strings if true
	parsePositionalArgsAsURI bool
}

type Namespace struct {
	// Specified database and collection
	DB         string `short:"d" long:"db" value-name:"<database-name>" description:"database to use"`
	Collection string `short:"c" long:"collection" value-name:"<collection-name>" description:"collection to use"`
}

func (ns Namespace) String() string {
	return ns.DB + "." + ns.Collection
}

// Struct holding generic options
type General struct {
	Help    bool `long:"help" description:"print usage"`
	Version bool `long:"version" description:"print the tool version and exit"`

	MaxProcs   int    `long:"numThreads" hidden:"true"`
	Failpoints string `long:"failpoints" hidden:"true"`
	Trace      bool   `long:"trace" hidden:"true"`
}

// Struct holding verbosity-related options
type Verbosity struct {
	SetVerbosity func(string) `short:"v" long:"verbose" value-name:"<level>" description:"more detailed log output (include multiple times for more verbosity, e.g. -vvvvv, or specify a numeric value, e.g. --verbose=N)" optional:"true" optional-value:""`
	Quiet        bool         `long:"quiet" description:"hide all log output"`
	VLevel       int          `no-flag:"true"`
}

func (v Verbosity) Level() int {
	return v.VLevel
}

func (v Verbosity) IsQuiet() bool {
	return v.Quiet
}

type URI struct {
	ConnectionString string `long:"uri" value-name:"mongodb-uri" description:"mongodb uri connection string"`

	knownURIParameters   []string
	extraOptionsRegistry []ExtraOptions
	connString           connstring.ConnString
}

// Struct holding connection-related options
type Connection struct {
	Host string `short:"h" long:"host" value-name:"<hostname>" description:"mongodb host to connect to (setname/host1,host2 for replica sets)"`
	Port string `long:"port" value-name:"<port>" description:"server port (can also use --host hostname:port)"`

	Timeout                int    `long:"dialTimeout" default:"3" hidden:"true" description:"dial timeout in seconds"`
	SocketTimeout          int    `long:"socketTimeout" default:"0" hidden:"true" description:"socket timeout in seconds (0 for no timeout)"`
	TCPKeepAliveSeconds    int    `long:"TCPKeepAliveSeconds" default:"30" hidden:"true" description:"seconds between TCP keep alives"`
	ServerSelectionTimeout int    `long:"serverSelectionTimeout" hidden:"true" description:"seconds to wait for server selection; 0 means driver default"`
	Compressors            string `long:"compressors" default:"none" hidden:"true" value-name:"<snappy,...>" description:"comma-separated list of compressors to enable. Use 'none' to disable."`
}

// Struct holding ssl-related options
type SSL struct {
	UseSSL              bool   `long:"ssl" description:"connect to a mongod or mongos that has ssl enabled"`
	SSLCAFile           string `long:"sslCAFile" value-name:"<filename>" description:"the .pem file containing the root certificate chain from the certificate authority"`
	SSLPEMKeyFile       string `long:"sslPEMKeyFile" value-name:"<filename>" description:"the .pem file containing the certificate and key"`
	SSLPEMKeyPassword   string `long:"sslPEMKeyPassword" value-name:"<password>" description:"the password to decrypt the sslPEMKeyFile, if necessary"`
	SSLCRLFile          string `long:"sslCRLFile" value-name:"<filename>" description:"the .pem file containing the certificate revocation list"`
	SSLAllowInvalidCert bool   `long:"sslAllowInvalidCertificates" description:"bypass the validation for server certificates"`
	SSLAllowInvalidHost bool   `long:"sslAllowInvalidHostnames" description:"bypass the validation for server name"`
	SSLFipsMode         bool   `long:"sslFIPSMode" description:"use FIPS mode of the installed openssl library"`
}

// Struct holding auth-related options
type Auth struct {
	Username  string `short:"u" value-name:"<username>" long:"username" description:"username for authentication"`
	Password  string `short:"p" value-name:"<password>" long:"password" description:"password for authentication"`
	Source    string `long:"authenticationDatabase" value-name:"<database-name>" description:"database that holds the user's credentials"`
	Mechanism string `long:"authenticationMechanism" value-name:"<mechanism>" description:"authentication mechanism to use"`
}

// Struct for Kerberos/GSSAPI-specific options
type Kerberos struct {
	Service     string `long:"gssapiServiceName" value-name:"<service-name>" description:"service name to use when authenticating using GSSAPI/Kerberos (default: mongodb)"`
	ServiceHost string `long:"gssapiHostName" value-name:"<host-name>" description:"hostname to use when authenticating using GSSAPI/Kerberos (default: <remote server's address>)"`
}
type WriteConcern struct {
	// Specifies the write concern for each write operation that mongofiles writes to the target database.
	// By default, mongofiles waits for a majority of members from the replica set to respond before returning.
	WriteConcern string `long:"writeConcern" value-name:"<write-concern>" default:"majority" default-mask:"-" description:"write concern options e.g. --writeConcern majority, --writeConcern '{w: 3, wtimeout: 500, fsync: true, j: true}'"`

	w        int
	wtimeout int
	fsync    bool
	journal  bool
}

type OptionRegistrationFunction func(*ToolOptions) error

var ConnectionOptFunctions []OptionRegistrationFunction

type EnabledOptions struct {
	Auth       bool
	Connection bool
	Namespace  bool
	URI        bool
}

func parseVal(val string) int {
	idx := strings.Index(val, "=")
	ret, err := strconv.Atoi(val[idx+1:])
	if err != nil {
		panic(fmt.Errorf("value was not a valid integer: %v", err))
	}
	return ret
}

// Ask for a new instance of tool options
func New(appName, versionStr, gitCommit, usageStr string, parsePositionalArgsAsURI bool, enabled EnabledOptions) *ToolOptions {
	opts := &ToolOptions{
		AppName:    appName,
		VersionStr: versionStr,
		GitCommit:  gitCommit,

		General:    &General{},
		Verbosity:  &Verbosity{},
		Connection: &Connection{},
		URI:        &URI{},
		SSL:        &SSL{},
		Auth:       &Auth{},
		Namespace:  &Namespace{},
		Kerberos:   &Kerberos{},
		parser: flags.NewNamedParser(
			fmt.Sprintf("%v %v", appName, usageStr), flags.None),
		enabledOptions:           enabled,
		parsePositionalArgsAsURI: parsePositionalArgsAsURI,
	}

	// Called when -v or --verbose is parsed
	opts.SetVerbosity = func(val string) {
		if i, err := strconv.Atoi(val); err == nil {
			opts.VLevel = opts.VLevel + i // -v=N or --verbose=N
		} else if matched, _ := regexp.MatchString(`^v+$`, val); matched {
			opts.VLevel = opts.VLevel + len(val) + 1 // Handles the -vvv cases
		} else if matched, _ := regexp.MatchString(`^v+=[0-9]$`, val); matched {
			opts.VLevel = parseVal(val) // I.e. -vv=3
		} else if val == "" {
			opts.VLevel = opts.VLevel + 1 // Increment for every occurrence of flag
		} else {
			log.Logvf(log.Always, "Invalid verbosity value given")
			os.Exit(-1)
		}
	}

	opts.parser.UnknownOptionHandler = opts.handleUnknownOption

	opts.URI.AddKnownURIParameters(KnownURIOptionsReplicaSet)

	if _, err := opts.parser.AddGroup("general options", "", opts.General); err != nil {
		panic(fmt.Errorf("couldn't register general options: %v", err))
	}
	if _, err := opts.parser.AddGroup("verbosity options", "", opts.Verbosity); err != nil {
		panic(fmt.Errorf("couldn't register verbosity options: %v", err))
	}

	// this call disables failpoints if compiled without failpoint support
	EnableFailpoints(opts)

	if enabled.Connection {
		opts.URI.AddKnownURIParameters(KnownURIOptionsConnection)
		if _, err := opts.parser.AddGroup("connection options", "", opts.Connection); err != nil {
			panic(fmt.Errorf("couldn't register connection options: %v", err))
		}
		opts.URI.AddKnownURIParameters(KnownURIOptionsSSL)
		if _, err := opts.parser.AddGroup("ssl options", "", opts.SSL); err != nil {
			panic(fmt.Errorf("couldn't register SSL options: %v", err))
		}
	}

	if enabled.Auth {
		opts.URI.AddKnownURIParameters(KnownURIOptionsAuth)
		if _, err := opts.parser.AddGroup("authentication options", "", opts.Auth); err != nil {
			panic(fmt.Errorf("couldn't register auth options"))
		}
		opts.URI.AddKnownURIParameters(KnownURIOptionsKerberos)
		if _, err := opts.parser.AddGroup("kerberos options", "", opts.Kerberos); err != nil {
			panic(fmt.Errorf("couldn't register Kerberos options"))
		}
	}
	if enabled.Namespace {
		if _, err := opts.parser.AddGroup("namespace options", "", opts.Namespace); err != nil {
			panic(fmt.Errorf("couldn't register namespace options"))
		}
	}
	if enabled.URI {
		if _, err := opts.parser.AddGroup("uri options", "", opts.URI); err != nil {
			panic(fmt.Errorf("couldn't register URI options"))
		}
	}
	if opts.MaxProcs <= 0 {
		opts.MaxProcs = runtime.NumCPU()
	}
	log.Logvf(log.Info, "Setting num cpus to %v", opts.MaxProcs)
	runtime.GOMAXPROCS(opts.MaxProcs)
	return opts
}

// UseReadOnlyHostDescription changes the help description of the --host arg to
// not mention the shard/host:port format used in the data-mutating tools
func (opts *ToolOptions) UseReadOnlyHostDescription() {
	hostOpt := opts.parser.FindOptionByLongName("host")
	hostOpt.Description = "mongodb host(s) to connect to (use commas to delimit hosts)"
}

// FindOptionByLongName finds an option in any of the added option groups by
// matching its long name; useful for modifying the attributes (e.g. description
// or name) of an option
func (opts *ToolOptions) FindOptionByLongName(name string) *flags.Option {
	return opts.parser.FindOptionByLongName(name)
}

// Print the usage message for the tool to stdout.  Returns whether or not the
// help flag is specified.
func (opts *ToolOptions) PrintHelp(force bool) bool {
	if opts.Help || force {
		opts.parser.WriteHelp(os.Stdout)
	}
	return opts.Help
}

type versionInfo struct {
	key, value string
}

var versionInfos []versionInfo

// Print the tool version to stdout.  Returns whether or not the version flag
// is specified.
func (opts *ToolOptions) PrintVersion() bool {
	if opts.Version {
		fmt.Printf("%v version: %v\n", opts.AppName, opts.VersionStr)
		fmt.Printf("git version: %v\n", opts.GitCommit)
		fmt.Printf("Go version: %v\n", runtime.Version())
		fmt.Printf("   os: %v\n", runtime.GOOS)
		fmt.Printf("   arch: %v\n", runtime.GOARCH)
		fmt.Printf("   compiler: %v\n", runtime.Compiler)
		for _, info := range versionInfos {
			fmt.Printf("%s: %s\n", info.key, info.value)
		}
	}
	return opts.Version
}

// Interface for extra options that need to be used by specific tools
type ExtraOptions interface {
	// Name specifying what type of options these are
	Name() string
}

type URISetter interface {
	// SetOptionsFromURI provides a way for tools to fetch any options that were
	// set in the URI and set them on the ExtraOptions that they pass to the options
	// package.
	SetOptionsFromURI(connstring.ConnString) error
}

func (auth *Auth) RequiresExternalDB() bool {
	return auth.Mechanism == "GSSAPI" || auth.Mechanism == "PLAIN" || auth.Mechanism == "MONGODB-X509"
}

func (auth *Auth) IsSet() bool {
	return *auth != Auth{}
}

// ShouldAskForPassword returns true if the user specifies a username flag
// but no password, and the authentication mechanism requires a password.
func (auth *Auth) ShouldAskForPassword() bool {
	return auth.Username != "" && auth.Password == "" &&
		!(auth.Mechanism == "MONGODB-X509" || auth.Mechanism == "GSSAPI")
}

func NewURI(unparsed string) (*URI, error) {
	cs, err := connstring.ParseAndValidate(unparsed)
	if err != nil {
		return nil, fmt.Errorf("error parsing URI from %v: %v", unparsed, err)
	}
	return &URI{ConnectionString: cs.String(), connString: cs}, nil
}

func (uri *URI) GetConnectionAddrs() []string {
	return uri.connString.Hosts
}
func (uri *URI) ParsedConnString() *connstring.ConnString {
	if uri.ConnectionString == "" {
		return nil
	}
	return &uri.connString
}
func (uri *URI) AddKnownURIParameters(uriFieldNames []string) {
	uri.knownURIParameters = append(uri.knownURIParameters, uriFieldNames...)
}

func (opts *ToolOptions) EnabledToolOptions() EnabledOptions {
	return opts.enabledOptions
}

func (uri *URI) LogUnsupportedOptions() {
	allOptionsFromURI := map[string]struct{}{}

	for optName := range uri.connString.Options {
		allOptionsFromURI[optName] = struct{}{}
	}

	for optName := range uri.connString.UnknownOptions {
		allOptionsFromURI[optName] = struct{}{}
	}

	for _, optName := range uri.knownURIParameters {
		if _, ok := allOptionsFromURI[optName]; ok {
			delete(allOptionsFromURI, optName)
		}
	}

	unsupportedOptions := make([]string, len(allOptionsFromURI))
	optionIndex := 0
	for optionName := range allOptionsFromURI {
		unsupportedOptions[optionIndex] = optionName
		optionIndex++
	}

	for _, optName := range unsupportedOptions {
		log.Logvf(log.Always, "WARNING: ignoring unsupported URI parameter '%v'", optName)
	}
}

// Get the authentication database to use. Should be the value of
// --authenticationDatabase if it's provided, otherwise, the database that's
// specified in the tool's --db arg.
func (opts *ToolOptions) GetAuthenticationDatabase() string {
	if opts.Auth.Source != "" {
		return opts.Auth.Source
	} else if opts.Auth.RequiresExternalDB() {
		return "$external"
	} else if opts.Namespace != nil && opts.Namespace.DB != "" {
		return opts.Namespace.DB
	}
	return ""
}

// AddOptions registers an additional options group to this instance
func (opts *ToolOptions) AddOptions(extraOpts ExtraOptions) {
	_, err := opts.parser.AddGroup(extraOpts.Name()+" options", "", extraOpts)
	if err != nil {
		panic(fmt.Sprintf("error setting command line options for  %v: %v",
			extraOpts.Name(), err))
	}

	if opts.enabledOptions.URI {
		opts.URI.extraOptionsRegistry = append(opts.URI.extraOptionsRegistry, extraOpts)
	}
}

// Parse the command line args.  Returns any extra args not accounted for by
// parsing, as well as an error if the parsing returns an error.
func (opts *ToolOptions) ParseArgs(args []string) ([]string, error) {
	args, err := opts.parser.ParseArgs(args)
	if err != nil {
		return []string{}, err
	}

	if opts.parsePositionalArgsAsURI {
		args, err = opts.setURIFromPositionalArg(args)
		if err != nil {
			return []string{}, err
		}
	}

	failpoint.ParseFailpoints(opts.Failpoints)

	err = opts.NormalizeOptionsAndURI()
	if err != nil {
		return []string{}, err
	}

	return args, err
}

func (opts *ToolOptions) setURIFromPositionalArg(args []string) ([]string, error) {
	newArgs := []string{}
	var foundURI bool
	var parsedURI connstring.ConnString

	for _, arg := range args {
		cs, err := connstring.Parse(arg)
		if err == nil {
			if foundURI {
				return []string{}, fmt.Errorf("too many URIs found in positional arguments: only one URI can be set as a positional argument")
			}
			foundURI = true
			parsedURI = cs
		} else {
			newArgs = append(newArgs, arg)
		}
	}

	if foundURI { // Successfully parsed a URI
		if opts.ConnectionString != "" {
			return []string{}, fmt.Errorf(IncompatibleArgsErrorFormat, "a URI in a positional argument")
		}
		opts.ConnectionString = parsedURI.Original
	}

	return newArgs, nil
}

// NormalizeOptionsAndURI syncs the connection string an toolOptions objects.
// It returns an error if there is any conflict between options and the connection string.
// If a value is set on the options, but not the connection string, that value is added to the
// connection string. If a value is set on the connection string, but not the options,
// that value is added to the options.
func (opts *ToolOptions) NormalizeOptionsAndURI() error {
	if opts.URI != nil && opts.URI.ConnectionString != "" {
		cs, err := connstring.Parse(opts.URI.ConnectionString)
		if err != nil {
			return err
		}
		err = opts.setOptionsFromURI(cs)
		if err != nil {
			return err
		}
		err = opts.connString.Validate()
		if err != nil {
			return err
		}
	} else {
		// If URI not provided, get replica set name and generate connection string
		_, opts.ReplicaSetName = util.SplitHostArg(opts.Host)
		uri, err := NewURI(util.BuildURI(opts.Host, opts.Port))
		if err != nil {
			return err
		}
		opts.URI = uri
	}

	// connect directly, unless a replica set name is explicitly specified
	opts.Direct = (opts.ReplicaSetName == "")

	return nil
}

func (opts *ToolOptions) handleUnknownOption(option string, arg flags.SplitArgument, args []string) ([]string, error) {
	if option == "dbpath" || option == "directoryperdb" || option == "journal" {
		return args, fmt.Errorf("--dbpath and related flags are not supported in 3.0 tools.\n" +
			"See http://dochub.mongodb.org/core/tools-dbpath-deprecated for more information")
	}

	return args, fmt.Errorf(`unknown option "%v"`, option)
}

// Sets options from the URI. If any options are already set, they are added to the connection string.
// which is eventually added to the connString field.
// Most CLI and URI options are normalized in three steps:
//
// 1. If both CLI option and URI option are set, throw an erroor if they conflict.
// 2. If the CLI option is set, but the URI option isn't, set the URI option
// 3. If the URI option is set, but the CLI option isn't, set the CLI option
//
// Some options (e.g. host and port) are more complicated. To check if a CLI option is set,
// we check that it is not equal to its default value. To check that a URI option is set,
// some options have an "OptionSet" field.
func (opts *ToolOptions) setOptionsFromURI(cs connstring.ConnString) error {
	opts.URI.connString = cs

	if opts.enabledOptions.Connection {
		// Port can be set in --port, --host, or URI
		// Each host/port pair in the options must match the URI host/port pairs
		if opts.Port != "" {
			// if --port is set, check that each host:port pair in the URI the port defined in --port
			for i, host := range cs.Hosts {
				if strings.Index(host, ":") != -1 {
					hostPort := strings.Split(host, ":")[1]
					if hostPort != opts.Port {
						return ConflictingArgsErrorFormat("port", strings.Join(cs.Hosts, ","), opts.Port, "--port")
					}
				} else {
					// if the URI hosts have no ports, append them
					cs.Hosts[i] = cs.Hosts[i] + ":" + opts.Port
				}
			}
		}

		if opts.Host != "" {
			// build hosts from --host and --port
			seedlist, replicaSetName := util.SplitHostArg(opts.Host)
			opts.ReplicaSetName = replicaSetName

			if opts.Port != "" {
				for i := range seedlist {
					if strings.Index(seedlist[i], ":") == -1 { // no port
						seedlist[i] = seedlist[i] + ":" + opts.Port
					}
				}
			}

			// create a set of hosts since the order of a seedlist doesn't matter
			csHostSet := make(map[string]bool)
			for _, host := range cs.Hosts {
				csHostSet[host] = true
			}

			optionHostSet := make(map[string]bool)
			for _, host := range seedlist {
				optionHostSet[host] = true
			}

			// check the sets are equal
			if len(csHostSet) != len(optionHostSet) {
				return ConflictingArgsErrorFormat("host", strings.Join(cs.Hosts, ","), opts.Host, "--host")
			}

			for host := range csHostSet {
				if _, ok := optionHostSet[host]; !ok {
					return ConflictingArgsErrorFormat("host", strings.Join(cs.Hosts, ","), opts.Host, "--host")
				}
			}
		}

		if opts.Connection.ServerSelectionTimeout != 0 && cs.ServerSelectionTimeoutSet {
			if (time.Duration(opts.Connection.ServerSelectionTimeout) * time.Millisecond) != cs.ServerSelectionTimeout {
				return ConflictingArgsErrorFormat("serverSelectionTimeout", strconv.Itoa(int(cs.ServerSelectionTimeout/time.Millisecond)), strconv.Itoa(opts.Connection.ServerSelectionTimeout), "--serverSelectionTimeout")
			}
		}
		if opts.Connection.ServerSelectionTimeout != 0 && !cs.ServerSelectionTimeoutSet {
			cs.ServerSelectionTimeout = time.Duration(opts.Connection.ServerSelectionTimeout) * time.Millisecond
		}
		if opts.Connection.ServerSelectionTimeout == 0 && cs.ServerSelectionTimeoutSet {
			opts.Connection.ServerSelectionTimeout = int(cs.ServerSelectionTimeout / time.Millisecond)
		}

		if opts.Connection.Timeout != 3 && cs.ConnectTimeoutSet {
			if (time.Duration(opts.Connection.Timeout) * time.Millisecond) != cs.ConnectTimeout {
				return ConflictingArgsErrorFormat("connectTimeout", strconv.Itoa(int(cs.ConnectTimeout/time.Millisecond)), strconv.Itoa(opts.Connection.Timeout), "--dialTimeout")
			}
		}
		if opts.Connection.Timeout != 3 && !cs.ConnectTimeoutSet {
			cs.ConnectTimeout = time.Duration(opts.Connection.Timeout) * time.Millisecond
		}
		if opts.Connection.Timeout == 3 && cs.ConnectTimeoutSet {
			opts.Connection.Timeout = int(cs.ConnectTimeout / time.Millisecond)
		}

		if opts.Connection.SocketTimeout != 0 && cs.SocketTimeoutSet {
			if (time.Duration(opts.Connection.SocketTimeout) * time.Millisecond) != cs.SocketTimeout {
				return ConflictingArgsErrorFormat("SocketTimeout", strconv.Itoa(int(cs.SocketTimeout/time.Millisecond)), strconv.Itoa(opts.Connection.SocketTimeout), "--socketTimeout")
			}
		}
		if opts.Connection.SocketTimeout != 0 && !cs.SocketTimeoutSet {
			cs.SocketTimeout = time.Duration(opts.Connection.SocketTimeout) * time.Millisecond
		}
		if opts.Connection.SocketTimeout == 0 && cs.SocketTimeoutSet {
			opts.Connection.SocketTimeout = int(cs.SocketTimeout / time.Millisecond)
		}

		if len(cs.Compressors) != 0 {
			if opts.Connection.Compressors != "none" && opts.Connection.Compressors != strings.Join(cs.Compressors, ",") {
				return ConflictingArgsErrorFormat("compressors", strings.Join(cs.Compressors, ","), opts.Connection.Compressors, "--compressors")
			}
		} else {
			cs.Compressors = strings.Split(opts.Connection.Compressors, ",")
		}
	}

	if opts.enabledOptions.Auth {

		if opts.Username != "" && cs.Username != "" {
			if opts.Username != cs.Username {
				return ConflictingArgsErrorFormat("username", opts.Username, cs.Username, "--username")
			}
		}
		if opts.Username != "" && cs.Username == "" {
			cs.Username = opts.Username
		}
		if opts.Username == "" && cs.Username != "" {
			opts.Username = cs.Username
		}
		if opts.Username == "" && cs.Username == "" && cs.Scheme == connstring.SchemeMongoDBSRV {
			return fmt.Errorf("must set a username when using an SRV scheme")
		}

		if opts.Password != "" && cs.Password != "" {
			if opts.Password != cs.Password {
				return fmt.Errorf("Invalid Options: Cannot specify different password in connection URI and command-line option")
			}
		}
		if opts.Password != "" && cs.Password == "" {
			cs.Password = opts.Password
		}
		if opts.Password == "" && cs.Password != "" {
			opts.Password = cs.Password
		}

		if opts.Source != "" && cs.AuthSourceSet {
			if opts.Source != cs.AuthSource {
				return ConflictingArgsErrorFormat("authSource", opts.Source, cs.AuthSource, "--authenticationDatabase")
			}
		}
		if opts.Source != "" && !cs.AuthSourceSet {
			cs.AuthSource = opts.Source
		}
		if opts.Source == "" && cs.AuthSourceSet {
			opts.Source = cs.AuthSource
		}

		if opts.Mechanism != "" && cs.AuthMechanism != "" {
			if opts.Mechanism != cs.AuthMechanism {
				return ConflictingArgsErrorFormat("authMechanism", opts.Mechanism, cs.AuthMechanism, "--authenticationMechanism")
			}
		}
		if opts.Mechanism != "" && cs.AuthMechanism == "" {
			cs.AuthMechanism = opts.Mechanism
		}
		if opts.Mechanism == "" && cs.AuthMechanism != "" {
			opts.Mechanism = cs.AuthMechanism
		}

	}

	if opts.enabledOptions.Namespace {

		if opts.DB != "" && cs.Database != "" {
			if opts.DB != cs.Database {
				return ConflictingArgsErrorFormat("database", cs.Database, opts.DB, "--db")
			}
		}
		if opts.DB != "" && cs.Database == "" {
			cs.Database = opts.DB
		}
		if opts.DB == "" && cs.Database != "" {
			opts.DB = cs.Database
		}
	}

	opts.Direct = (cs.Connect == connstring.SingleConnect)

	// check replica set name equality
	if opts.ReplicaSetName != "" && cs.ReplicaSet != "" {
		if opts.ReplicaSetName != cs.ReplicaSet {
			return ConflictingArgsErrorFormat("replica set name", cs.ReplicaSet, opts.Host, "--host")
		}

	}
	if opts.ReplicaSetName != "" && cs.ReplicaSet == "" {
		cs.ReplicaSet = opts.ReplicaSetName
	}
	if opts.ReplicaSetName == "" && cs.ReplicaSet != "" {
		opts.ReplicaSetName = cs.ReplicaSet
	}

	if (cs.SSL || opts.UseSSL) && !BuiltWithSSL {
		if strings.HasPrefix(cs.Original, "mongodb+srv") {
			return fmt.Errorf("SSL enabled by default when using SRV but tool not built with SSL: " +
				"SSL must be explicitly disabled with ssl=false in the connection string")
		}
		return fmt.Errorf("cannot use ssl: tool not built with SSL support")
	}

	if cs.SSLSet {
		if opts.UseSSL && !cs.SSL {
			return ConflictingArgsErrorFormat("ssl", strconv.FormatBool(cs.SSL), strconv.FormatBool(opts.UseSSL), "--ssl")
		} else if !opts.UseSSL && cs.SSL {
			opts.UseSSL = cs.SSL
		}
	}

	if opts.SSLCAFile != "" && cs.SSLCaFileSet {
		if opts.SSLCAFile != cs.SSLCaFile {
			return ConflictingArgsErrorFormat("sslCAFile", cs.SSLCaFile, opts.SSLCAFile, "--sslCAFile")
		}
	}
	if opts.SSLCAFile != "" && !cs.SSLCaFileSet {
		cs.SSLCaFile = opts.SSLCAFile
	}
	if opts.SSLCAFile == "" && cs.SSLCaFileSet {
		opts.SSLCAFile = cs.SSLCaFile
	}

	if opts.SSLPEMKeyFile != "" && cs.SSLClientCertificateKeyFileSet {
		if opts.SSLPEMKeyFile != cs.SSLClientCertificateKeyFile {
			return ConflictingArgsErrorFormat("sslPEMKeyFile", cs.SSLClientCertificateKeyFile, opts.SSLPEMKeyFile, "--sslPEMKeyFile")
		}
	}
	if opts.SSLPEMKeyFile != "" && !cs.SSLClientCertificateKeyFileSet {
		cs.SSLClientCertificateKeyFile = opts.SSLPEMKeyFile
	}
	if opts.SSLPEMKeyFile == "" && cs.SSLClientCertificateKeyFileSet {
		opts.SSLPEMKeyFile = cs.SSLClientCertificateKeyFile
	}

	if opts.SSLPEMKeyPassword != "" && cs.SSLClientCertificateKeyPasswordSet {
		if opts.SSLPEMKeyPassword != cs.SSLClientCertificateKeyPassword() {
			return ConflictingArgsErrorFormat("sslPEMKeyFile", cs.SSLClientCertificateKeyPassword(), opts.SSLPEMKeyPassword, "--sslPEMKeyFile")
		}
	}

	if opts.SSLPEMKeyPassword != "" && !cs.SSLClientCertificateKeyPasswordSet {
		cs.SSLClientCertificateKeyPassword = func() string { return opts.SSLPEMKeyPassword }
	}
	if opts.SSLPEMKeyPassword == "" && cs.SSLClientCertificateKeyPasswordSet {
		opts.SSLPEMKeyPassword = cs.SSLClientCertificateKeyPassword()
	}

	// Note: SSLCRLFile is not parsed by the go driver

	if cs.SSLInsecureSet {
		if (opts.SSLAllowInvalidCert || opts.SSLAllowInvalidHost) && !cs.SSLInsecure {
			return ConflictingArgsErrorFormat("sslPEMKeyFile", "false", "true", "--sslAllowInvalidCert or --sslAllowInvalidHost")
		}
		opts.SSLAllowInvalidCert = cs.SSLInsecure
		opts.SSLAllowInvalidHost = cs.SSLInsecure
	}

	if strings.ToLower(cs.AuthMechanism) == "gssapi" {
		if !BuiltWithGSSAPI {
			return fmt.Errorf("cannot specify gssapiservicename: tool not built with kerberos support")
		}

		gssapiServiceName, _ := cs.AuthMechanismProperties["SERVICE_NAME"]

		if opts.Kerberos.Service != "" && cs.AuthMechanismPropertiesSet {
			if opts.Kerberos.Service != gssapiServiceName {
				return ConflictingArgsErrorFormat("Kerberos service name", gssapiServiceName, opts.Kerberos.Service, "--gssapiServiceName")
			}
		}
		if opts.Kerberos.Service != "" && !cs.AuthMechanismPropertiesSet {
			cs.AuthMechanismProperties["SERVICE_NAME"] = opts.Kerberos.Service
		}
		if opts.Kerberos.Service == "" && cs.AuthMechanismPropertiesSet {
			opts.Kerberos.Service = gssapiServiceName
		}
	}

	for _, extraOpts := range opts.URI.extraOptionsRegistry {
		if uriSetter, ok := extraOpts.(URISetter); ok {
			err := uriSetter.SetOptionsFromURI(cs)
			if err != nil {
				return err
			}
		}
	}

	// set the connString on opts so it can be validated later
	opts.connString = cs

	return nil
}

// getIntArg returns 3 args: the parsed int value, a bool set to true if a value
// was consumed from the incoming args array during parsing, and an error
// value if parsing failed
func getIntArg(arg flags.SplitArgument, args []string) (int, bool, error) {
	var rawVal string
	consumeValue := false
	rawVal, hasVal := arg.Value()
	if !hasVal {
		if len(args) == 0 {
			return 0, false, fmt.Errorf("no value specified")
		}
		rawVal = args[0]
		consumeValue = true
	}
	val, err := strconv.Atoi(rawVal)
	if err != nil {
		return val, consumeValue, fmt.Errorf("expected an integer value but got '%v'", rawVal)
	}
	return val, consumeValue, nil
}

// getStringArg returns 3 args: the parsed string value, a bool set to true if a value
// was consumed from the incoming args array during parsing, and an error
// value if parsing failed
func getStringArg(arg flags.SplitArgument, args []string) (string, bool, error) {
	value, hasVal := arg.Value()
	if hasVal {
		return value, false, nil
	}
	if len(args) == 0 {
		return "", false, fmt.Errorf("no value specified")
	}
	return args[0], true, nil
}
