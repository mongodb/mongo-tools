// Copyright (C) MongoDB, Inc. 2014-present.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at http://www.apache.org/licenses/LICENSE-2.0

// Package options implements command-line options that are used by all of
// the mongo tools.
package options

import (
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	flags "github.com/jessevdk/go-flags"
	"github.com/mongodb/mongo-tools/common/failpoint"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/password"
	"github.com/mongodb/mongo-tools/common/util"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
	"go.mongodb.org/mongo-driver/x/mongo/driver/connstring"
	"gopkg.in/yaml.v2"
)

// XXX Force these true as the Go driver supports them always.  Once the
// conditionals that depend on them are removed, these can be removed.
var (
	BuiltWithSSL    = true
	BuiltWithGSSAPI = true
)

const IncompatibleArgsErrorFormat = "illegal argument combination: cannot specify %s and --uri"

const unknownOptionsWarningFormat = "WARNING: ignoring unsupported URI parameter '%v'"

func ConflictingArgsErrorFormat(optionName, uriValue, cliValue, cliOptionName string) error {
	return fmt.Errorf("Invalid Options: Cannot specify different %s in connection URI and command-line option (\"%s\" was specified in the URI and \"%s\" was specified in the %s option)", optionName, uriValue, cliValue, cliOptionName)
}

const deprecationWarningSSLAllow = "WARNING: --sslAllowInvalidCertificates and --sslAllowInvalidHostnames are deprecated, please use --tlsInsecure instead"

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
	Help       bool   `long:"help" description:"print usage"`
	Version    bool   `long:"version" description:"print the tool version and exit"`
	ConfigPath string `long:"config" description:"path to a configuration file"`

	MaxProcs   int    `long:"numThreads" hidden:"true"`
	Failpoints string `long:"failpoints" hidden:"true"`
	Trace      bool   `long:"trace" hidden:"true"`
}

// Struct holding verbosity-related options
type Verbosity struct {
	SetVerbosity    func(string) `short:"v" long:"verbose" value-name:"<level>" description:"more detailed log output (include multiple times for more verbosity, e.g. -vvvvv, or specify a numeric value, e.g. --verbose=N)" optional:"true" optional-value:""`
	Quiet           bool         `long:"quiet" description:"hide all log output"`
	VLevel          int          `no-flag:"true"`
	VerbosityParsed bool         `no-flag:"true"`
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
	ConnString           connstring.ConnString
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
	SSLAllowInvalidCert bool   `long:"sslAllowInvalidCertificates" hidden:"true" description:"bypass the validation for server certificates"`
	SSLAllowInvalidHost bool   `long:"sslAllowInvalidHostnames" hidden:"true" description:"bypass the validation for server name"`
	SSLFipsMode         bool   `long:"sslFIPSMode" description:"use FIPS mode of the installed openssl library"`
	TLSInsecure         bool   `long:"tlsInsecure" description:"bypass the validation for server's certificate chain and host name"`
}

// Struct holding auth-related options
type Auth struct {
	Username        string `short:"u" value-name:"<username>" long:"username" description:"username for authentication"`
	Password        string `short:"p" value-name:"<password>" long:"password" description:"password for authentication"`
	Source          string `long:"authenticationDatabase" value-name:"<database-name>" description:"database that holds the user's credentials"`
	Mechanism       string `long:"authenticationMechanism" value-name:"<mechanism>" description:"authentication mechanism to use"`
	AWSSessionToken string `long:"awsSessionToken" value-name:"<aws-session-token>" description:"session token to authenticate via AWS IAM"`
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
		// Reset verbosity level when we call ParseArgs again and see the verbosity flag
		if opts.VLevel != 0 && opts.VerbosityParsed {
			opts.VerbosityParsed = false
			opts.VLevel = 0
		}

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

	if _, err := opts.parser.AddGroup("general options", "", opts.General); err != nil {
		panic(fmt.Errorf("couldn't register general options: %v", err))
	}
	if _, err := opts.parser.AddGroup("verbosity options", "", opts.Verbosity); err != nil {
		panic(fmt.Errorf("couldn't register verbosity options: %v", err))
	}

	// this call disables failpoints if compiled without failpoint support
	EnableFailpoints(opts)

	if enabled.Connection {
		if _, err := opts.parser.AddGroup("connection options", "", opts.Connection); err != nil {
			panic(fmt.Errorf("couldn't register connection options: %v", err))
		}
		if _, err := opts.parser.AddGroup("ssl options", "", opts.SSL); err != nil {
			panic(fmt.Errorf("couldn't register SSL options: %v", err))
		}
	}

	if enabled.Auth {
		if _, err := opts.parser.AddGroup("authentication options", "", opts.Auth); err != nil {
			panic(fmt.Errorf("couldn't register auth options"))
		}
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

// Interface for extra options used in mongomirror.
type DestinationAuthOptions interface {
	// Set the password for authentication on the destination.
	SetDestinationPassword(string)
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

// ShouldAskForPassword returns true if the user specifies a ssl pem key file
// flag but no password for that file, and the key file has any encrypted
// blocks.
func (ssl *SSL) ShouldAskForPassword() (bool, error) {
	if ssl.SSLPEMKeyFile == "" || ssl.SSLPEMKeyPassword != "" {
		return false, nil
	}
	return ssl.pemKeyFileHasEncryptedKey()
}

func (ssl *SSL) pemKeyFileHasEncryptedKey() (bool, error) {
	b, err := ioutil.ReadFile(ssl.SSLPEMKeyFile)
	if err != nil {
		return false, err
	}

	for {
		var v *pem.Block
		v, b = pem.Decode(b)
		if v == nil {
			break
		}
		if v.Type == "ENCRYPTED PRIVATE KEY" {
			return true, nil
		}
	}

	return false, nil
}

func NewURI(unparsed string) (*URI, error) {
	cs, err := connstring.Parse(unparsed)
	if err != nil {
		return nil, fmt.Errorf("error parsing URI from %v: %v", unparsed, err)
	}
	return &URI{ConnectionString: cs.String(), ConnString: cs}, nil
}

func (uri *URI) GetConnectionAddrs() []string {
	return uri.ConnString.Hosts
}
func (uri *URI) ParsedConnString() *connstring.ConnString {
	if uri.ConnectionString == "" {
		return nil
	}
	return &uri.ConnString
}

func (opts *ToolOptions) EnabledToolOptions() EnabledOptions {
	return opts.enabledOptions
}

// LogUnsupportedOptions logs warnings regarding unknown/unsupported URI parameters.
// The unknown options are determined by the driver.
func (uri *URI) LogUnsupportedOptions() {
	for key := range uri.ConnString.UnknownOptions {
		log.Logvf(log.Always, unknownOptionsWarningFormat, key)
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
		opts.AddToExtraOptionsRegistry(extraOpts)
	}
}

// AddToExtraOptionsRegistry appends an additional options group to the extra options
// registry found in opts.URI.
func (opts *ToolOptions) AddToExtraOptionsRegistry(extraOpts ExtraOptions) {
	opts.URI.extraOptionsRegistry = append(opts.URI.extraOptionsRegistry, extraOpts)
}

func (opts *ToolOptions) CallArgParser(args []string) ([]string, error) {
	args, err := opts.parser.ParseArgs(args)
	if err != nil {
		return []string{}, err
	}

	// Set VerbosityParsed flag to make sure we reset verbosity level when we call ParseArgs again
	if opts.VLevel != 0 && !opts.VerbosityParsed {
		opts.VerbosityParsed = true
	}

	return args, nil
}

// ParseArgs parses a potential config file followed by the command line args, overriding
// any values in the config file. Returns any extra args not accounted for by parsing,
// as well as an error if the parsing returns an error.
func (opts *ToolOptions) ParseArgs(args []string) ([]string, error) {
	LogSensitiveOptionWarnings(args)

	if err := opts.ParseConfigFile(args); err != nil {
		return []string{}, err
	}

	args, err := opts.CallArgParser(args)
	if err != nil {
		return []string{}, err
	}

	if opts.SSLAllowInvalidCert || opts.SSLAllowInvalidHost {
		log.Logvf(log.Always, deprecationWarningSSLAllow)
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

// LogSensitiveOptionWarnings logs a warning for any sensitive information (i.e. passwords)
// that appear on the command line for the --password, --uri and --sslPEMKeyPassword options.
// This also applies to a connection string that appears as a positional argument.
func LogSensitiveOptionWarnings(args []string) {
	passwordMsg := "WARNING: On some systems, a password provided directly using " +
		"--password may be visible to system status programs such as `ps` that may be " +
		"invoked by other users. Consider omitting the password to provide it via stdin, " +
		"or using the --config option to specify a configuration file with the password."

	uriMsg := "WARNING: On some systems, a password provided directly in a connection string " +
		"or using --uri may be visible to system status programs such as `ps` that may be " +
		"invoked by other users. Consider omitting the password to provide it via stdin, " +
		"or using the --config option to specify a configuration file with the password."

	sslMsg := "WARNING: On some systems, a password provided directly using --sslPEMKeyPassword " +
		"may be visible to system status programs such as `ps` that may be invoked by other users. " +
		"Consider using the --config option to specify a configuration file with the password."

	// Create temporary options for parsing command line args.
	tempOpts := New("", "", "", "", true, EnabledOptions{Auth: true, Connection: true, URI: true})
	extraArgs, err := tempOpts.CallArgParser(args)
	if err != nil {
		return
	}

	// Parse the extraArgs for a positional connection string.
	_, err = tempOpts.setURIFromPositionalArg(extraArgs)
	if err != nil {
		return
	}

	// Log a message for --password, if specified.
	if tempOpts.Auth.Password != "" {
		log.Logvf(log.Always, passwordMsg)
	}

	// Log a message for --uri or a positional connection string, if either is specified.
	uri := tempOpts.URI.ConnectionString
	if uri != "" {
		if cs, err := connstring.Parse(uri); err == nil && cs.Password != "" {
			log.Logvf(log.Always, uriMsg)
		}
	}

	// Log a message for --sslPEMKeyPassword, if specified.
	if tempOpts.SSL.SSLPEMKeyPassword != "" {
		log.Logvf(log.Always, sslMsg)
	}
}

// ParseConfigFile iterates over args to find a --config option. If not found, we return.
// If found, we read the contents of the specified config file in YAML format. We parse
// any values corresponding to --password, --uri and --sslPEMKeyPassword, and store them
// in the opts.
// This also applies to --destinationPassword for mongomirror only.
func (opts *ToolOptions) ParseConfigFile(args []string) error {
	// Get config file path from the arguments, if specified.
	_, err := opts.CallArgParser(args)
	if err != nil {
		return err
	}

	// No --config option was specified.
	if opts.General.ConfigPath == "" {
		return nil
	}

	// --config option specifies a file path.
	configBytes, err := ioutil.ReadFile(opts.General.ConfigPath)
	if err != nil {
		return errors.Wrapf(err, "error opening file with --config")
	}

	// Unmarshal the config file as a top-level YAML file.
	var config struct {
		Password            string `yaml:"password"`
		ConnectionString    string `yaml:"uri"`
		SSLPEMKeyPassword   string `yaml:"sslPEMKeyPassword"`
		DestinationPassword string `yaml:"destinationPassword"`
	}
	err = yaml.UnmarshalStrict(configBytes, &config)
	if err != nil {
		return errors.Wrapf(err, "error parsing config file %s", opts.General.ConfigPath)
	}

	// Assign each parsed value to its respective ToolOptions field.
	opts.Auth.Password = config.Password
	opts.URI.ConnectionString = config.ConnectionString
	opts.SSL.SSLPEMKeyPassword = config.SSLPEMKeyPassword

	// Mongomirror has an extra option to set.
	for _, extraOpt := range opts.URI.extraOptionsRegistry {
		if destinationAuth, ok := extraOpt.(DestinationAuthOptions); ok {
			destinationAuth.SetDestinationPassword(config.DestinationPassword)
			break
		}
	}

	return nil
}

func (opts *ToolOptions) setURIFromPositionalArg(args []string) ([]string, error) {
	newArgs := []string{}
	var foundURI bool
	var parsedURI connstring.ConnString

	for _, arg := range args {
		if arg == "" {
			continue
		}
		cs, err := connstring.Parse(arg)
		if err == nil {
			if foundURI {
				return []string{}, fmt.Errorf("too many URIs found in positional arguments: only one URI can be set as a positional argument")
			}
			foundURI = true
			parsedURI = cs
		} else if err.Error() == "error parsing uri: scheme must be \"mongodb\" or \"mongodb+srv\"" {
			newArgs = append(newArgs, arg)
		} else {
			return []string{}, err
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

// NormalizeOptionsAndURI syncs the connection string and toolOptions objects.
// It returns an error if there is any conflict between options and the connection string.
// If a value is set on the options, but not the connection string, that value is added to the
// connection string. If a value is set on the connection string, but not the options,
// that value is added to the options.
func (opts *ToolOptions) NormalizeOptionsAndURI() error {
	if opts.URI == nil || opts.URI.ConnectionString == "" {
		// If URI not provided, get replica set name and generate connection string
		_, opts.ReplicaSetName = util.SplitHostArg(opts.Host)
		uri, err := NewURI(util.BuildURI(opts.Host, opts.Port))
		if err != nil {
			return err
		}
		opts.URI = uri
	}

	cs, err := connstring.Parse(opts.URI.ConnectionString)
	if err != nil {
		return err
	}
	err = opts.setOptionsFromURI(cs)
	if err != nil {
		return err
	}

	// finalize auth options, filling in missing passwords
	if opts.Auth.ShouldAskForPassword() {
		pass, err := password.Prompt("mongo user")
		if err != nil {
			return fmt.Errorf("error reading password: %v", err)
		}
		opts.Auth.Password = pass
		opts.ConnString.Password = pass
	}

	shouldAskForSSLPassword, err := opts.SSL.ShouldAskForPassword()
	if err != nil {
		return fmt.Errorf("error determining whether client cert needs password: %v", err)
	}
	if shouldAskForSSLPassword {
		pass, err := password.Prompt("client certificate")
		if err != nil {
			return fmt.Errorf("error reading password: %v", err)
		}
		opts.SSL.SSLPEMKeyPassword = pass
	}

	err = opts.ConnString.Validate()
	if err != nil {
		return errors.Wrap(err, "connection string failed validation")
	}

	// Connect directly to a host if there's no replica set specified, or
	// if the connection string already specified a direct connection.
	// Do not connect directly if loadbalanced.
	if !opts.ConnString.LoadBalanced {
		opts.Direct = (opts.ReplicaSetName == "") || opts.Direct
	}

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
// 1. If both CLI option and URI option are set, throw an error if they conflict.
// 2. If the CLI option is set, but the URI option isn't, set the URI option
// 3. If the URI option is set, but the CLI option isn't, set the CLI option
//
// Some options (e.g. host and port) are more complicated. To check if a CLI option is set,
// we check that it is not equal to its default value. To check that a URI option is set,
// some options have an "OptionSet" field.
func (opts *ToolOptions) setOptionsFromURI(cs connstring.ConnString) error {
	opts.URI.ConnString = cs

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
		} else if len(cs.Hosts) > 0 {
			if cs.ReplicaSet != "" {
				opts.Host = cs.ReplicaSet + "/"
			}

			// check if there is a <host:port> pair with a port that matches --port <port>
			conflictingPorts := true
			for _, host := range cs.Hosts {
				hostPort := strings.Split(host, ":")
				opts.Host += hostPort[0] + ","

				// a port might not be specified, e.g. `mongostat --discover`
				if len(hostPort) == 2 {
					if opts.Port != "" {
						if hostPort[1] == opts.Port {
							conflictingPorts = false
						}
					} else {
						opts.Port = hostPort[1]
						conflictingPorts = false
					}
				} else {
					conflictingPorts = false
				}
			}
			if conflictingPorts {
				return ConflictingArgsErrorFormat("port", strings.Join(cs.Hosts, ","), opts.Port, "--port")
			}
			// remove trailing comma
			opts.Host = opts.Host[:len(opts.Host)-1]
		}

		if len(cs.Hosts) > 1 && cs.LoadBalanced {
			return fmt.Errorf("loadBalanced cannot be set to true if multiple hosts are specified")
		}

		if opts.Connection.ServerSelectionTimeout != 0 && cs.ServerSelectionTimeoutSet {
			if (time.Duration(opts.Connection.ServerSelectionTimeout) * time.Millisecond) != cs.ServerSelectionTimeout {
				return ConflictingArgsErrorFormat("serverSelectionTimeout", strconv.Itoa(int(cs.ServerSelectionTimeout/time.Millisecond)), strconv.Itoa(opts.Connection.ServerSelectionTimeout), "--serverSelectionTimeout")
			}
		}
		if opts.Connection.ServerSelectionTimeout != 0 && !cs.ServerSelectionTimeoutSet {
			cs.ServerSelectionTimeout = time.Duration(opts.Connection.ServerSelectionTimeout) * time.Millisecond
			cs.ServerSelectionTimeoutSet = true
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
			cs.ConnectTimeoutSet = true
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
			cs.SocketTimeoutSet = true
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
				return ConflictingArgsErrorFormat("username", cs.Username, opts.Username, "--username")
			}
		}
		if opts.Username != "" && cs.Username == "" {
			cs.Username = opts.Username
		}
		if opts.Username == "" && cs.Username != "" {
			opts.Username = cs.Username
		}

		if opts.Password != "" && cs.PasswordSet {
			if opts.Password != cs.Password {
				return fmt.Errorf("Invalid Options: Cannot specify different password in connection URI and command-line option")
			}
		}
		if opts.Password != "" && !cs.PasswordSet {
			cs.Password = opts.Password
			cs.PasswordSet = true
		}
		if opts.Password == "" && cs.PasswordSet {
			opts.Password = cs.Password
		}

		if opts.Source != "" && cs.AuthSourceSet {
			if opts.Source != cs.AuthSource {
				return ConflictingArgsErrorFormat("authSource", cs.AuthSource, opts.Source, "--authenticationDatabase")
			}
		}
		if opts.Source != "" && !cs.AuthSourceSet {
			cs.AuthSource = opts.Source
			cs.AuthSourceSet = true
		}
		if opts.Source == "" && cs.AuthSourceSet {
			opts.Source = cs.AuthSource
		}

		if opts.Mechanism != "" && cs.AuthMechanism != "" {
			if opts.Mechanism != cs.AuthMechanism {
				return ConflictingArgsErrorFormat("authMechanism", cs.AuthMechanism, opts.Mechanism, "--authenticationMechanism")
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

	// check replica set name equality
	if opts.ReplicaSetName != "" && cs.ReplicaSet != "" {
		if opts.ReplicaSetName != cs.ReplicaSet {
			return ConflictingArgsErrorFormat("replica set name", cs.ReplicaSet, opts.Host, "--host")
		}
		if opts.ConnString.LoadBalanced {
			return fmt.Errorf("loadBalanced cannot be set to true if the replica set name is specified")
		}
	}
	if opts.ReplicaSetName != "" && cs.ReplicaSet == "" {
		cs.ReplicaSet = opts.ReplicaSetName
	}
	if opts.ReplicaSetName == "" && cs.ReplicaSet != "" {
		opts.ReplicaSetName = cs.ReplicaSet
	}

	// Connect directly to a host if indicated by the connection string.
	opts.Direct = cs.DirectConnection || (cs.Connect == connstring.SingleConnect)
	if opts.Direct && opts.ConnString.LoadBalanced {
		return fmt.Errorf("loadBalanced cannot be set to true if the direct connection option is specified")
	}

	if (cs.SSL || opts.UseSSL) && !BuiltWithSSL {
		if strings.HasPrefix(cs.Original, "mongodb+srv") {
			return fmt.Errorf("SSL enabled by default when using SRV but tool not built with SSL: " +
				"SSL must be explicitly disabled with ssl=false in the connection string")
		}
		return fmt.Errorf("cannot use ssl: tool not built with SSL support")
	}

	if cs.RetryWritesSet {
		opts.RetryWrites = &cs.RetryWrites
	}

	if cs.SSLSet {
		if opts.UseSSL && !cs.SSL {
			return ConflictingArgsErrorFormat("ssl", strconv.FormatBool(cs.SSL), strconv.FormatBool(opts.UseSSL), "--ssl")
		} else if !opts.UseSSL && cs.SSL {
			opts.UseSSL = cs.SSL
		}
	}

	// ignore opts.UseSSL being false due to zero-value problem (TOOLS-2459 PR for details)
	// Ignore: opts.UseSSL = false, cs.SSL = true (have cs take precedence)
	// Treat as conflict: opts.UseSSL = true, cs.SSL = false
	if opts.UseSSL && cs.SSLSet {
		if !cs.SSL {
			return ConflictingArgsErrorFormat("ssl or tls", strconv.FormatBool(cs.SSL), strconv.FormatBool(opts.UseSSL), "--ssl")
		}
	}
	if opts.UseSSL && !cs.SSLSet {
		cs.SSL = opts.UseSSL
		cs.SSLSet = true
	}
	// If SSL set in cs but not in opts,
	if !opts.UseSSL && cs.SSLSet {
		opts.UseSSL = cs.SSL
	}

	if opts.SSLCAFile != "" && cs.SSLCaFileSet {
		if opts.SSLCAFile != cs.SSLCaFile {
			return ConflictingArgsErrorFormat("sslCAFile", cs.SSLCaFile, opts.SSLCAFile, "--sslCAFile")
		}
	}
	if opts.SSLCAFile != "" && !cs.SSLCaFileSet {
		cs.SSLCaFile = opts.SSLCAFile
		cs.SSLCaFileSet = true
	}
	if opts.SSLCAFile == "" && cs.SSLCaFileSet {
		opts.SSLCAFile = cs.SSLCaFile
	}

	if opts.SSLPEMKeyFile != "" && cs.SSLClientCertificateKeyFileSet {
		if opts.SSLPEMKeyFile != cs.SSLClientCertificateKeyFile {
			return ConflictingArgsErrorFormat("sslClientCertificateKeyFile", cs.SSLClientCertificateKeyFile, opts.SSLPEMKeyFile, "--sslPEMKeyFile")
		}
	}
	if opts.SSLPEMKeyFile != "" && !cs.SSLClientCertificateKeyFileSet {
		cs.SSLClientCertificateKeyFile = opts.SSLPEMKeyFile
		cs.SSLClientCertificateKeyFileSet = true
	}
	if opts.SSLPEMKeyFile == "" && cs.SSLClientCertificateKeyFileSet {
		opts.SSLPEMKeyFile = cs.SSLClientCertificateKeyFile
	}

	if opts.SSLPEMKeyPassword != "" && cs.SSLClientCertificateKeyPasswordSet {
		if opts.SSLPEMKeyPassword != cs.SSLClientCertificateKeyPassword() {
			return ConflictingArgsErrorFormat("sslPEMKeyFilePassword", cs.SSLClientCertificateKeyPassword(), opts.SSLPEMKeyPassword, "--sslPEMKeyFilePassword")
		}
	}
	if opts.SSLPEMKeyPassword != "" && !cs.SSLClientCertificateKeyPasswordSet {
		cs.SSLClientCertificateKeyPassword = func() string { return opts.SSLPEMKeyPassword }
		cs.SSLClientCertificateKeyPasswordSet = true
	}
	if opts.SSLPEMKeyPassword == "" && cs.SSLClientCertificateKeyPasswordSet {
		opts.SSLPEMKeyPassword = cs.SSLClientCertificateKeyPassword()
	}

	// Note: SSLCRLFile is not parsed by the go driver

	// ignore (opts.SSLAllowInvalidCert || opts.SSLAllowInvalidHost) being false due to zero-value problem (TOOLS-2459 PR for details)
	// Have cs take precedence in cases where it is unclear
	if (opts.SSLAllowInvalidCert || opts.SSLAllowInvalidHost || opts.TLSInsecure) && cs.SSLInsecureSet {
		if !cs.SSLInsecure {
			return ConflictingArgsErrorFormat("sslInsecure or tlsInsecure", "false", "true", "--sslAllowInvalidCert or --sslAllowInvalidHost")
		}
	}
	if (opts.SSLAllowInvalidCert || opts.SSLAllowInvalidHost || opts.TLSInsecure) && !cs.SSLInsecureSet {
		cs.SSLInsecure = true
		cs.SSLInsecureSet = true
	}
	if (!opts.SSLAllowInvalidCert && !opts.SSLAllowInvalidHost || !opts.TLSInsecure) && cs.SSLInsecureSet {
		opts.SSLAllowInvalidCert = cs.SSLInsecure
		opts.SSLAllowInvalidHost = cs.SSLInsecure
		opts.TLSInsecure = cs.SSLInsecure
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
			if cs.AuthMechanismProperties == nil {
				cs.AuthMechanismProperties = make(map[string]string)
			}
			cs.AuthMechanismProperties["SERVICE_NAME"] = opts.Kerberos.Service
			cs.AuthMechanismPropertiesSet = true
		}
		if opts.Kerberos.Service == "" && cs.AuthMechanismPropertiesSet {
			opts.Kerberos.Service = gssapiServiceName
		}
	}

	if strings.ToLower(cs.AuthMechanism) == "mongodb-aws" {
		awsSessionToken, _ := cs.AuthMechanismProperties["AWS_SESSION_TOKEN"]

		if opts.AWSSessionToken != "" && cs.AuthMechanismPropertiesSet {
			if opts.AWSSessionToken != awsSessionToken {
				return ConflictingArgsErrorFormat("AWS Session Token", awsSessionToken, opts.AWSSessionToken, "--awsSessionToken")
			}
		}
		if opts.AWSSessionToken != "" && !cs.AuthMechanismPropertiesSet {
			if cs.AuthMechanismProperties == nil {
				cs.AuthMechanismProperties = make(map[string]string)
			}
			cs.AuthMechanismProperties["AWS_SESSION_TOKEN"] = opts.AWSSessionToken
			cs.AuthMechanismPropertiesSet = true
		}
		if opts.AWSSessionToken == "" && cs.AuthMechanismPropertiesSet {
			opts.AWSSessionToken = awsSessionToken
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
	opts.ConnString = cs

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
