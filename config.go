// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	flags "github.com/btcsuite/go-flags"
	"github.com/btcsuite/go-socks/socks"
	"github.com/decred/dcrd/database"
	_ "github.com/decred/dcrd/database/ldb"
	_ "github.com/decred/dcrd/database/memdb"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

const (
	defaultConfigFilename    = "dcrd.conf"
	defaultDataDirname       = "data"
	defaultLogLevel          = "info"
	defaultLogDirname        = "logs"
	defaultLogFilename       = "dcrd.log"
	defaultMaxPeers          = 125
	defaultBanDuration       = time.Hour * 24
	defaultBanThreshold      = 100
	defaultMaxRPCClients     = 10
	defaultMaxRPCWebsockets  = 25
	defaultVerifyEnabled     = false
	defaultDbType            = "leveldb"
	defaultFreeTxRelayLimit  = 15.0
	defaultBlockMinSize      = 0
	defaultBlockMaxSize      = 375000
	blockMaxSizeMin          = 1000
	blockMaxSizeMax          = wire.MaxBlockPayload - 1000
	defaultBlockPrioritySize = 20000
	defaultGenerate          = false
	defaultAddrIndex         = false
	defaultNonAggressive     = false
	defaultNoMiningStateSync = false
	defaultAllowOldVotes     = false
	defaultSigCacheMaxSize   = 50000
)

var (
	dcrdHomeDir        = dcrutil.AppDataDir("dcrd", false)
	defaultConfigFile  = filepath.Join(dcrdHomeDir, defaultConfigFilename)
	defaultDataDir     = filepath.Join(dcrdHomeDir, defaultDataDirname)
	knownDbTypes       = database.SupportedDBs()
	defaultRPCKeyFile  = filepath.Join(dcrdHomeDir, "rpc.key")
	defaultRPCCertFile = filepath.Join(dcrdHomeDir, "rpc.cert")
	defaultLogDir      = filepath.Join(dcrdHomeDir, defaultLogDirname)
)

// runServiceCommand is only set to a real function on Windows.  It is used
// to parse and execute service commands specified via the -s flag.
var runServiceCommand func(string) error

// minUint32 is a helper function to return the minimum of two uint32s.
// This avoids a math import and the need to cast to floats.
func minUint32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

// config defines the configuration options for dcrd.
//
// See loadConfig for details on the configuration load process.
type config struct {
	DcrdHomeDir        string        `short:"A" long:"appdata" description:"Path to dcrd home directory"`
	ShowVersion        bool          `short:"V" long:"version" description:"Display version information and exit"`
	ConfigFile         string        `short:"C" long:"configfile" description:"Path to configuration file"`
	DataDir            string        `short:"b" long:"datadir" description:"Directory to store data"`
	LogDir             string        `long:"logdir" description:"Directory to log output."`
	AddPeers           []string      `short:"a" long:"addpeer" description:"Add a peer to connect with at startup"`
	ConnectPeers       []string      `long:"connect" description:"Connect only to the specified peers at startup"`
	DisableListen      bool          `long:"nolisten" description:"Disable listening for incoming connections -- NOTE: Listening is automatically disabled if the --connect or --proxy options are used without also specifying listen interfaces via --listen"`
	Listeners          []string      `long:"listen" description:"Add an interface/port to listen for connections (default all interfaces port: 9108, testnet: 19108)"`
	MaxPeers           int           `long:"maxpeers" description:"Max number of inbound and outbound peers"`
	DisableBanning     bool          `long:"nobanning" description:"Disable banning of misbehaving peers"`
	BanDuration        time.Duration `long:"banduration" description:"How long to ban misbehaving peers.  Valid time units are {s, m, h}.  Minimum 1 second"`
	BanThreshold       uint32        `long:"banthreshold" description:"Maximum allowed ban score before disconnecting and banning misbehaving peers."`
	RPCUser            string        `short:"u" long:"rpcuser" description:"Username for RPC connections"`
	RPCPass            string        `short:"P" long:"rpcpass" default-mask:"-" description:"Password for RPC connections"`
	RPCLimitUser       string        `long:"rpclimituser" description:"Username for limited RPC connections"`
	RPCLimitPass       string        `long:"rpclimitpass" default-mask:"-" description:"Password for limited RPC connections"`
	RPCListeners       []string      `long:"rpclisten" description:"Add an interface/port to listen for RPC connections (default port: 9109, testnet: 19109)"`
	RPCCert            string        `long:"rpccert" description:"File containing the certificate file"`
	RPCKey             string        `long:"rpckey" description:"File containing the certificate key"`
	RPCMaxClients      int           `long:"rpcmaxclients" description:"Max number of RPC clients for standard connections"`
	RPCMaxWebsockets   int           `long:"rpcmaxwebsockets" description:"Max number of RPC websocket connections"`
	DisableRPC         bool          `long:"norpc" description:"Disable built-in RPC server -- NOTE: The RPC server is disabled by default if no rpcuser/rpcpass or rpclimituser/rpclimitpass is specified"`
	DisableTLS         bool          `long:"notls" description:"Disable TLS for the RPC server -- NOTE: This is only allowed if the RPC server is bound to localhost"`
	DisableDNSSeed     bool          `long:"nodnsseed" description:"Disable DNS seeding for peers"`
	ExternalIPs        []string      `long:"externalip" description:"Add an ip to the list of local addresses we claim to listen on to peers"`
	Proxy              string        `long:"proxy" description:"Connect via SOCKS5 proxy (eg. 127.0.0.1:9050)"`
	ProxyUser          string        `long:"proxyuser" description:"Username for proxy server"`
	ProxyPass          string        `long:"proxypass" default-mask:"-" description:"Password for proxy server"`
	OnionProxy         string        `long:"onion" description:"Connect to tor hidden services via SOCKS5 proxy (eg. 127.0.0.1:9050)"`
	OnionProxyUser     string        `long:"onionuser" description:"Username for onion proxy server"`
	OnionProxyPass     string        `long:"onionpass" default-mask:"-" description:"Password for onion proxy server"`
	NoOnion            bool          `long:"noonion" description:"Disable connecting to tor hidden services"`
	TorIsolation       bool          `long:"torisolation" description:"Enable Tor stream isolation by randomizing user credentials for each connection."`
	TestNet            bool          `long:"testnet" description:"Use the test network"`
	SimNet             bool          `long:"simnet" description:"Use the simulation test network"`
	DisableCheckpoints bool          `long:"nocheckpoints" description:"Disable built-in checkpoints.  Don't do this unless you know what you're doing."`
	DbType             string        `long:"dbtype" description:"Database backend to use for the Block Chain"`
	Profile            string        `long:"profile" description:"Enable HTTP profiling on given port -- NOTE port must be between 1024 and 65536"`
	CPUProfile         string        `long:"cpuprofile" description:"Write CPU profile to the specified file"`
	MemProfile         string        `long:"memprofile" description:"Write mem profile to the specified file"`
	DumpBlockchain     string        `long:"dumpblockchain" description:"Write blockchain as a gob-encoded map to the specified file"`
	MiningTimeOffset   int           `long:"miningtimeoffset" description:"Offset the mining timestamp of a block by this many seconds (positive values are in the past)"`
	DebugLevel         string        `short:"d" long:"debuglevel" description:"Logging level for all subsystems {trace, debug, info, warn, error, critical} -- You may also specify <subsystem>=<level>,<subsystem2>=<level>,... to set the log level for individual subsystems -- Use show to list available subsystems"`
	Upnp               bool          `long:"upnp" description:"Use UPnP to map our listening port outside of NAT"`
	MinRelayTxFee      float64       `long:"minrelaytxfee" description:"The minimum transaction fee in DCR/kB to be considered a non-zero fee."`
	FreeTxRelayLimit   float64       `long:"limitfreerelay" description:"Limit relay of transactions with no transaction fee to the given amount in thousands of bytes per minute"`
	NoRelayPriority    bool          `long:"norelaypriority" description:"Do not require free or low-fee transactions to have high priority for relaying"`
	MaxOrphanTxs       int           `long:"maxorphantx" description:"Max number of orphan transactions to keep in memory"`
	Generate           bool          `long:"generate" description:"Generate (mine) coins using the CPU"`
	MiningAddrs        []string      `long:"miningaddr" description:"Add the specified payment address to the list of addresses to use for generated blocks -- At least one address is required if the generate option is set"`
	BlockMinSize       uint32        `long:"blockminsize" description:"Mininum block size in bytes to be used when creating a block"`
	BlockMaxSize       uint32        `long:"blockmaxsize" description:"Maximum block size in bytes to be used when creating a block"`
	BlockPrioritySize  uint32        `long:"blockprioritysize" description:"Size in bytes for high-priority/low-fee transactions when creating a block"`
	GetWorkKeys        []string      `long:"getworkkey" description:"DEPRECATED -- Use the --miningaddr option instead"`
	NoAddrIndex        bool          `long:"addrindex" description:"Disable building and maintaining a full address index. Currently only supported by leveldb. Will prevent wallet resyncing from seed."`
	DropAddrIndex      bool          `long:"dropaddrindex" description:"Deletes the address-based transaction index from the database on start up, and then exits."`
	NonAggressive      bool          `long:"nonaggressive" description:"Disable mining off of the parent block of the blockchain if there aren't enough voters"`
	NoMiningStateSync  bool          `long:"nominingstatesync" description:"Disable synchronizing the mining state with other nodes"`
	AllowOldVotes      bool          `long:"allowoldvotes" description:"Enable the addition of very old votes to the mempool"`
	NoPeerBloomFilters bool          `long:"nopeerbloomfilters" description:"Disable bloom filtering support."`
	SigCacheMaxSize    uint          `long:"sigcachemaxsize" description:"The maximum number of entries in the signature verification cache."`
	onionlookup        func(string) ([]net.IP, error)
	lookup             func(string) ([]net.IP, error)
	oniondial          func(string, string) (net.Conn, error)
	dial               func(string, string) (net.Conn, error)
	miningAddrs        []dcrutil.Address
	minRelayTxFee      dcrutil.Amount
}

// serviceOptions defines the configuration options for the daemon as a service on
// Windows.
type serviceOptions struct {
	ServiceCommand string `short:"s" long:"service" description:"Service command {install, remove, start, stop}"`
}

// cleanAndExpandPath expands environment variables and leading ~ in the
// passed path, cleans the result, and returns it.
func cleanAndExpandPath(path string) string {
	// Expand initial ~ to OS specific home directory.
	if strings.HasPrefix(path, "~") {
		homeDir := filepath.Dir(dcrdHomeDir)
		path = strings.Replace(path, "~", homeDir, 1)
	}

	// NOTE: The os.ExpandEnv doesn't work with Windows-style %VARIABLE%,
	// but they variables can still be expanded via POSIX-style $VARIABLE.
	return filepath.Clean(os.ExpandEnv(path))
}

// validLogLevel returns whether or not logLevel is a valid debug log level.
func validLogLevel(logLevel string) bool {
	switch logLevel {
	case "trace":
		fallthrough
	case "debug":
		fallthrough
	case "info":
		fallthrough
	case "warn":
		fallthrough
	case "error":
		fallthrough
	case "critical":
		return true
	}
	return false
}

// supportedSubsystems returns a sorted slice of the supported subsystems for
// logging purposes.
func supportedSubsystems() []string {
	// Convert the subsystemLoggers map keys to a slice.
	subsystems := make([]string, 0, len(subsystemLoggers))
	for subsysID := range subsystemLoggers {
		subsystems = append(subsystems, subsysID)
	}

	// Sort the subsystems for stable display.
	sort.Strings(subsystems)
	return subsystems
}

// parseAndSetDebugLevels attempts to parse the specified debug level and set
// the levels accordingly.  An appropriate error is returned if anything is
// invalid.
func parseAndSetDebugLevels(debugLevel string) error {
	// When the specified string doesn't have any delimters, treat it as
	// the log level for all subsystems.
	if !strings.Contains(debugLevel, ",") && !strings.Contains(debugLevel, "=") {
		// Validate debug log level.
		if !validLogLevel(debugLevel) {
			str := "The specified debug level [%v] is invalid"
			return fmt.Errorf(str, debugLevel)
		}

		// Change the logging level for all subsystems.
		setLogLevels(debugLevel)

		return nil
	}

	// Split the specified string into subsystem/level pairs while detecting
	// issues and update the log levels accordingly.
	for _, logLevelPair := range strings.Split(debugLevel, ",") {
		if !strings.Contains(logLevelPair, "=") {
			str := "The specified debug level contains an invalid " +
				"subsystem/level pair [%v]"
			return fmt.Errorf(str, logLevelPair)
		}

		// Extract the specified subsystem and log level.
		fields := strings.Split(logLevelPair, "=")
		subsysID, logLevel := fields[0], fields[1]

		// Validate subsystem.
		if _, exists := subsystemLoggers[subsysID]; !exists {
			str := "The specified subsystem [%v] is invalid -- " +
				"supported subsytems %v"
			return fmt.Errorf(str, subsysID, supportedSubsystems())
		}

		// Validate log level.
		if !validLogLevel(logLevel) {
			str := "The specified debug level [%v] is invalid"
			return fmt.Errorf(str, logLevel)
		}

		setLogLevel(subsysID, logLevel)
	}

	return nil
}

// validDbType returns whether or not dbType is a supported database type.
func validDbType(dbType string) bool {
	for _, knownType := range knownDbTypes {
		if dbType == knownType {
			return true
		}
	}

	return false
}

// removeDuplicateAddresses returns a new slice with all duplicate entries in
// addrs removed.
func removeDuplicateAddresses(addrs []string) []string {
	result := make([]string, 0, len(addrs))
	seen := map[string]struct{}{}
	for _, val := range addrs {
		if _, ok := seen[val]; !ok {
			result = append(result, val)
			seen[val] = struct{}{}
		}
	}
	return result
}

// normalizeAddress returns addr with the passed default port appended if
// there is not already a port specified.
func normalizeAddress(addr, defaultPort string) string {
	_, _, err := net.SplitHostPort(addr)
	if err != nil {
		return net.JoinHostPort(addr, defaultPort)
	}
	return addr
}

// normalizeAddresses returns a new slice with all the passed peer addresses
// normalized with the given default port, and all duplicates removed.
func normalizeAddresses(addrs []string, defaultPort string) []string {
	for i, addr := range addrs {
		addrs[i] = normalizeAddress(addr, defaultPort)
	}

	return removeDuplicateAddresses(addrs)
}

// filesExists reports whether the named file or directory exists.
func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// newConfigParser returns a new command line flags parser.
func newConfigParser(cfg *config, so *serviceOptions, options flags.Options) *flags.Parser {
	parser := flags.NewParser(cfg, options)
	if runtime.GOOS == "windows" {
		parser.AddGroup("Service Options", "Service Options", so)
	}
	return parser
}

// loadConfig initializes and parses the config using a config file and command
// line options.
//
// The configuration proceeds as follows:
// 	1) Start with a default config with sane settings
// 	2) Pre-parse the command line to check for an alternative config file
// 	3) Load configuration file overwriting defaults with any specified options
// 	4) Parse CLI options and overwrite/add any specified options
//
// The above results in daemon functioning properly without any config settings
// while still allowing the user to override settings with config files and
// command line options.  Command line options always take precedence.
func loadConfig() (*config, []string, error) {
	// Default config.
	cfg := config{
		DcrdHomeDir:       dcrdHomeDir,
		ConfigFile:        defaultConfigFile,
		DebugLevel:        defaultLogLevel,
		MaxPeers:          defaultMaxPeers,
		BanDuration:       defaultBanDuration,
		BanThreshold:      defaultBanThreshold,
		RPCMaxClients:     defaultMaxRPCClients,
		RPCMaxWebsockets:  defaultMaxRPCWebsockets,
		DataDir:           defaultDataDir,
		LogDir:            defaultLogDir,
		DbType:            defaultDbType,
		RPCKey:            defaultRPCKeyFile,
		RPCCert:           defaultRPCCertFile,
		MinRelayTxFee:     defaultMinRelayTxFee.ToCoin(),
		FreeTxRelayLimit:  defaultFreeTxRelayLimit,
		BlockMinSize:      defaultBlockMinSize,
		BlockMaxSize:      defaultBlockMaxSize,
		BlockPrioritySize: defaultBlockPrioritySize,
		SigCacheMaxSize:   defaultSigCacheMaxSize,
		MaxOrphanTxs:      maxOrphanTransactions,
		Generate:          defaultGenerate,
		NoAddrIndex:       defaultAddrIndex,
		NoMiningStateSync: defaultNoMiningStateSync,
		AllowOldVotes:     defaultAllowOldVotes,
	}

	// Service options which are only added on Windows.
	serviceOpts := serviceOptions{}

	// Pre-parse the command line options to see if an alternative config
	// file or the version flag was specified.  Any errors aside from the
	// help message error can be ignored here since they will be caught by
	// the final parse below.
	preCfg := cfg
	preParser := newConfigParser(&preCfg, &serviceOpts, flags.HelpFlag)
	_, err := preParser.Parse()
	if err != nil {
		if e, ok := err.(*flags.Error); ok && e.Type == flags.ErrHelp {
			fmt.Fprintln(os.Stderr, err)
			return nil, nil, err
		}
	}

	// Show the version and exit if the version flag was specified.
	appName := filepath.Base(os.Args[0])
	appName = strings.TrimSuffix(appName, filepath.Ext(appName))
	usageMessage := fmt.Sprintf("Use %s -h to show usage", appName)
	if preCfg.ShowVersion {
		fmt.Println(appName, "version", version())
		os.Exit(0)
	}

	// Perform service command and exit if specified.  Invalid service
	// commands show an appropriate error.  Only runs on Windows since
	// the runServiceCommand function will be nil when not on Windows.
	if serviceOpts.ServiceCommand != "" && runServiceCommand != nil {
		err := runServiceCommand(serviceOpts.ServiceCommand)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(0)
	}

	// Update the home directory for dcrd if specified. Since the home
	// directory is updated, other variables need to be updated to
	// reflect the new changes.
	if len(preCfg.DcrdHomeDir) > 0 {
		cfg.DcrdHomeDir, _ = filepath.Abs(preCfg.DcrdHomeDir)
		cfg.ConfigFile = filepath.Join(cfg.DcrdHomeDir, defaultConfigFilename)
		cfg.DataDir = filepath.Join(cfg.DcrdHomeDir, defaultDataDirname)
		cfg.RPCKey = filepath.Join(cfg.DcrdHomeDir, "rpc.key")
		cfg.RPCCert = filepath.Join(cfg.DcrdHomeDir, "rpc.cert")
		cfg.LogDir = filepath.Join(cfg.DcrdHomeDir, defaultLogDirname)
	}

	// Load additional config from file.
	var configFileError error
	parser := newConfigParser(&cfg, &serviceOpts, flags.Default)
	if !(preCfg.SimNet) || preCfg.ConfigFile !=
		defaultConfigFile {

		err := flags.NewIniParser(parser).ParseFile(preCfg.ConfigFile)
		if err != nil {
			if _, ok := err.(*os.PathError); !ok {
				fmt.Fprintf(os.Stderr, "Error parsing config "+
					"file: %v\n", err)
				fmt.Fprintln(os.Stderr, usageMessage)
				return nil, nil, err
			}
			configFileError = err
		}
	}

	// Parse command line options again to ensure they take precedence.
	remainingArgs, err := parser.Parse()
	if err != nil {
		if e, ok := err.(*flags.Error); !ok || e.Type != flags.ErrHelp {
			fmt.Fprintln(os.Stderr, usageMessage)
		}
		return nil, nil, err
	}

	// Create the home directory if it doesn't already exist.
	funcName := "loadConfig"
	err = os.MkdirAll(dcrdHomeDir, 0700)
	if err != nil {
		// Show a nicer error message if it's because a symlink is
		// linked to a directory that does not exist (probably because
		// it's not mounted).
		if e, ok := err.(*os.PathError); ok && os.IsExist(err) {
			if link, lerr := os.Readlink(e.Path); lerr == nil {
				str := "is symlink %s -> %s mounted?"
				err = fmt.Errorf(str, e.Path, link)
			}
		}

		str := "%s: Failed to create home directory: %v"
		err := fmt.Errorf(str, funcName, err)
		fmt.Fprintln(os.Stderr, err)
		return nil, nil, err
	}

	// Multiple networks can't be selected simultaneously.
	numNets := 0

	// Count number of network flags passed; assign active network params
	// while we're at it
	if cfg.TestNet {
		numNets++
		activeNetParams = &testNetParams
	}
	if cfg.SimNet {
		numNets++
		// Also disable dns seeding on the simulation test network.
		activeNetParams = &simNetParams
		cfg.DisableDNSSeed = true
	}
	if numNets > 1 {
		str := "%s: The testnet and simnet params can't be " +
			"used together -- choose one of the three"
		err := fmt.Errorf(str, funcName)
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, usageMessage)
		return nil, nil, err
	}

	// Append the network type to the data directory so it is "namespaced"
	// per network.  In addition to the block database, there are other
	// pieces of data that are saved to disk such as address manager state.
	// All data is specific to a network, so namespacing the data directory
	// means each individual piece of serialized data does not have to
	// worry about changing names per network and such.
	cfg.DataDir = cleanAndExpandPath(cfg.DataDir)
	cfg.DataDir = filepath.Join(cfg.DataDir, netName(activeNetParams))

	// Append the network type to the log directory so it is "namespaced"
	// per network in the same fashion as the data directory.
	cfg.LogDir = cleanAndExpandPath(cfg.LogDir)
	cfg.LogDir = filepath.Join(cfg.LogDir, netName(activeNetParams))

	// Special show command to list supported subsystems and exit.
	if cfg.DebugLevel == "show" {
		fmt.Println("Supported subsystems", supportedSubsystems())
		os.Exit(0)
	}

	// Initialize logging at the default logging level.
	initSeelogLogger(filepath.Join(cfg.LogDir, defaultLogFilename))
	setLogLevels(defaultLogLevel)

	// Parse, validate, and set debug log level(s).
	if err := parseAndSetDebugLevels(cfg.DebugLevel); err != nil {
		err := fmt.Errorf("%s: %v", funcName, err.Error())
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, usageMessage)
		return nil, nil, err
	}

	// Validate database type.
	if !validDbType(cfg.DbType) {
		str := "%s: The specified database type [%v] is invalid -- " +
			"supported types %v"
		err := fmt.Errorf(str, funcName, cfg.DbType, knownDbTypes)
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, usageMessage)
		return nil, nil, err
	}

	if !cfg.NoAddrIndex && cfg.DropAddrIndex {
		err := fmt.Errorf("addrindex and dropaddrindex cannot be " +
			"activated at the same")
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, usageMessage)
		return nil, nil, err
	}

	// Memdb does not currently support the addrindex.
	if cfg.DbType == "memdb" && !cfg.NoAddrIndex {
		err := fmt.Errorf("memdb does not currently support the addrindex")
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, usageMessage)
		return nil, nil, err
	}

	// Validate profile port number
	if cfg.Profile != "" {
		profilePort, err := strconv.Atoi(cfg.Profile)
		if err != nil || profilePort < 1024 || profilePort > 65535 {
			str := "%s: The profile port must be between 1024 and 65535"
			err := fmt.Errorf(str, funcName)
			fmt.Fprintln(os.Stderr, err)
			fmt.Fprintln(os.Stderr, usageMessage)
			return nil, nil, err
		}
	}

	// Don't allow ban durations that are too short.
	if cfg.BanDuration < time.Duration(time.Second) {
		str := "%s: The banduration option may not be less than 1s -- parsed [%v]"
		err := fmt.Errorf(str, funcName, cfg.BanDuration)
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, usageMessage)
		return nil, nil, err
	}

	// --addPeer and --connect do not mix.
	if len(cfg.AddPeers) > 0 && len(cfg.ConnectPeers) > 0 {
		str := "%s: the --addpeer and --connect options can not be " +
			"mixed"
		err := fmt.Errorf(str, funcName)
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, usageMessage)
		return nil, nil, err
	}

	// --proxy or --connect without --listen disables listening.
	if (cfg.Proxy != "" || len(cfg.ConnectPeers) > 0) &&
		len(cfg.Listeners) == 0 {
		cfg.DisableListen = true
	}

	// Connect means no DNS seeding.
	if len(cfg.ConnectPeers) > 0 {
		cfg.DisableDNSSeed = true
	}

	// Add the default listener if none were specified. The default
	// listener is all addresses on the listen port for the network
	// we are to connect to.
	if len(cfg.Listeners) == 0 {
		cfg.Listeners = []string{
			net.JoinHostPort("", activeNetParams.DefaultPort),
		}
	}

	// Check to make sure limited and admin users don't have the same username
	if cfg.RPCUser == cfg.RPCLimitUser && cfg.RPCUser != "" {
		str := "%s: --rpcuser and --rpclimituser must not specify the " +
			"same username"
		err := fmt.Errorf(str, funcName)
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, usageMessage)
		return nil, nil, err
	}

	// Check to make sure limited and admin users don't have the same password
	if cfg.RPCPass == cfg.RPCLimitPass && cfg.RPCPass != "" {
		str := "%s: --rpcpass and --rpclimitpass must not specify the " +
			"same password"
		err := fmt.Errorf(str, funcName)
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, usageMessage)
		return nil, nil, err
	}

	// The RPC server is disabled if no username or password is provided.
	if (cfg.RPCUser == "" || cfg.RPCPass == "") &&
		(cfg.RPCLimitUser == "" || cfg.RPCLimitPass == "") {
		cfg.DisableRPC = true
	}

	// Default RPC to listen on localhost only.
	if !cfg.DisableRPC && len(cfg.RPCListeners) == 0 {
		addrs, err := net.LookupHost("localhost")
		if err != nil {
			return nil, nil, err
		}
		cfg.RPCListeners = make([]string, 0, len(addrs))
		for _, addr := range addrs {
			addr = net.JoinHostPort(addr, activeNetParams.rpcPort)
			cfg.RPCListeners = append(cfg.RPCListeners, addr)
		}
	}

	// Validate the the minrelaytxfee.
	cfg.minRelayTxFee, err = dcrutil.NewAmount(cfg.MinRelayTxFee)
	if err != nil {
		str := "%s: invalid minrelaytxfee: %v"
		err := fmt.Errorf(str, funcName, err)
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, usageMessage)
		return nil, nil, err
	}

	// Limit the max block size to a sane value.
	if cfg.BlockMaxSize < blockMaxSizeMin || cfg.BlockMaxSize >
		blockMaxSizeMax {

		str := "%s: The blockmaxsize option must be in between %d " +
			"and %d -- parsed [%d]"
		err := fmt.Errorf(str, funcName, blockMaxSizeMin,
			blockMaxSizeMax, cfg.BlockMaxSize)
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, usageMessage)
		return nil, nil, err
	}

	// Limit the max orphan count to a sane vlue.
	if cfg.MaxOrphanTxs < 0 {
		str := "%s: The maxorphantx option may not be less than 0 " +
			"-- parsed [%d]"
		err := fmt.Errorf(str, funcName, cfg.MaxOrphanTxs)
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, usageMessage)
		return nil, nil, err
	}

	// Limit the block priority and minimum block sizes to max block size.
	cfg.BlockPrioritySize = minUint32(cfg.BlockPrioritySize, cfg.BlockMaxSize)
	cfg.BlockMinSize = minUint32(cfg.BlockMinSize, cfg.BlockMaxSize)

	// Check getwork keys are valid and saved parsed versions.
	cfg.miningAddrs = make([]dcrutil.Address, 0, len(cfg.GetWorkKeys)+
		len(cfg.MiningAddrs))
	for _, strAddr := range cfg.GetWorkKeys {
		addr, err := dcrutil.DecodeAddress(strAddr,
			activeNetParams.Params)
		if err != nil {
			str := "%s: getworkkey '%s' failed to decode: %v"
			err := fmt.Errorf(str, funcName, strAddr, err)
			fmt.Fprintln(os.Stderr, err)
			fmt.Fprintln(os.Stderr, usageMessage)
			return nil, nil, err
		}
		if !addr.IsForNet(activeNetParams.Params) {
			str := "%s: getworkkey '%s' is on the wrong network"
			err := fmt.Errorf(str, funcName, strAddr)
			fmt.Fprintln(os.Stderr, err)
			fmt.Fprintln(os.Stderr, usageMessage)
			return nil, nil, err
		}
		cfg.miningAddrs = append(cfg.miningAddrs, addr)
	}

	// Check mining addresses are valid and saved parsed versions.
	for _, strAddr := range cfg.MiningAddrs {
		addr, err := dcrutil.DecodeAddress(strAddr, activeNetParams.Params)
		if err != nil {
			str := "%s: mining address '%s' failed to decode: %v"
			err := fmt.Errorf(str, funcName, strAddr, err)
			fmt.Fprintln(os.Stderr, err)
			fmt.Fprintln(os.Stderr, usageMessage)
			return nil, nil, err
		}
		if !addr.IsForNet(activeNetParams.Params) {
			str := "%s: mining address '%s' is on the wrong network"
			err := fmt.Errorf(str, funcName, strAddr)
			fmt.Fprintln(os.Stderr, err)
			fmt.Fprintln(os.Stderr, usageMessage)
			return nil, nil, err
		}
		cfg.miningAddrs = append(cfg.miningAddrs, addr)
	}

	// Ensure there is at least one mining address when the generate flag is
	// set.
	if cfg.Generate && len(cfg.MiningAddrs) == 0 {
		str := "%s: the generate flag is set, but there are no mining " +
			"addresses specified "
		err := fmt.Errorf(str, funcName)
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, usageMessage)
		return nil, nil, err
	}

	// Add default port to all listener addresses if needed and remove
	// duplicate addresses.
	cfg.Listeners = normalizeAddresses(cfg.Listeners,
		activeNetParams.DefaultPort)

	// Add default port to all rpc listener addresses if needed and remove
	// duplicate addresses.
	cfg.RPCListeners = normalizeAddresses(cfg.RPCListeners,
		activeNetParams.rpcPort)

	// Only allow TLS to be disabled if the RPC is bound to localhost
	// addresses.
	if !cfg.DisableRPC && cfg.DisableTLS {
		allowedTLSListeners := map[string]struct{}{
			"localhost": {},
			"127.0.0.1": {},
			"::1":       {},
		}
		for _, addr := range cfg.RPCListeners {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				str := "%s: RPC listen interface '%s' is " +
					"invalid: %v"
				err := fmt.Errorf(str, funcName, addr, err)
				fmt.Fprintln(os.Stderr, err)
				fmt.Fprintln(os.Stderr, usageMessage)
				return nil, nil, err
			}
			if _, ok := allowedTLSListeners[host]; !ok {
				str := "%s: the --notls option may not be used " +
					"when binding RPC to non localhost " +
					"addresses: %s"
				err := fmt.Errorf(str, funcName, addr)
				fmt.Fprintln(os.Stderr, err)
				fmt.Fprintln(os.Stderr, usageMessage)
				return nil, nil, err
			}
		}
	}

	// Add default port to all added peer addresses if needed and remove
	// duplicate addresses.
	cfg.AddPeers = normalizeAddresses(cfg.AddPeers,
		activeNetParams.DefaultPort)
	cfg.ConnectPeers = normalizeAddresses(cfg.ConnectPeers,
		activeNetParams.DefaultPort)

	// Tor stream isolation requires either proxy or onion proxy to be set.
	if cfg.TorIsolation && cfg.Proxy == "" && cfg.OnionProxy == "" {
		str := "%s: Tor stream isolation requires either proxy or " +
			"onionproxy to be set"
		err := fmt.Errorf(str, funcName)
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, usageMessage)
		return nil, nil, err
	}

	// Setup dial and DNS resolution (lookup) functions depending on the
	// specified options.  The default is to use the standard net.Dial
	// function as well as the system DNS resolver.  When a proxy is
	// specified, the dial function is set to the proxy specific dial
	// function and the lookup is set to use tor (unless --noonion is
	// specified in which case the system DNS resolver is used).
	cfg.dial = net.Dial
	cfg.lookup = net.LookupIP
	if cfg.Proxy != "" {
		_, _, err := net.SplitHostPort(cfg.Proxy)
		if err != nil {
			str := "%s: Proxy address '%s' is invalid: %v"
			err := fmt.Errorf(str, funcName, cfg.Proxy, err)
			fmt.Fprintln(os.Stderr, err)
			fmt.Fprintln(os.Stderr, usageMessage)
			return nil, nil, err
		}

		if cfg.TorIsolation &&
			(cfg.ProxyUser != "" || cfg.ProxyPass != "") {
			dcrdLog.Warn("Tor isolation set -- overriding " +
				"specified proxy user credentials")
		}

		proxy := &socks.Proxy{
			Addr:         cfg.Proxy,
			Username:     cfg.ProxyUser,
			Password:     cfg.ProxyPass,
			TorIsolation: cfg.TorIsolation,
		}
		cfg.dial = proxy.Dial
		if !cfg.NoOnion {
			cfg.lookup = func(host string) ([]net.IP, error) {
				return torLookupIP(host, cfg.Proxy)
			}
		}
	}

	// Setup onion address dial and DNS resolution (lookup) functions
	// depending on the specified options.  The default is to use the
	// same dial and lookup functions selected above.  However, when an
	// onion-specific proxy is specified, the onion address dial and
	// lookup functions are set to use the onion-specific proxy while
	// leaving the normal dial and lookup functions as selected above.
	// This allows .onion address traffic to be routed through a different
	// proxy than normal traffic.
	if cfg.OnionProxy != "" {
		_, _, err := net.SplitHostPort(cfg.OnionProxy)
		if err != nil {
			str := "%s: Onion proxy address '%s' is invalid: %v"
			err := fmt.Errorf(str, funcName, cfg.OnionProxy, err)
			fmt.Fprintln(os.Stderr, err)
			fmt.Fprintln(os.Stderr, usageMessage)
			return nil, nil, err
		}

		if cfg.TorIsolation &&
			(cfg.OnionProxyUser != "" || cfg.OnionProxyPass != "") {
			dcrdLog.Warn("Tor isolation set -- overriding " +
				"specified onionproxy user credentials ")
		}

		cfg.oniondial = func(a, b string) (net.Conn, error) {
			proxy := &socks.Proxy{
				Addr:         cfg.OnionProxy,
				Username:     cfg.OnionProxyUser,
				Password:     cfg.OnionProxyPass,
				TorIsolation: cfg.TorIsolation,
			}
			return proxy.Dial(a, b)
		}
		cfg.onionlookup = func(host string) ([]net.IP, error) {
			return torLookupIP(host, cfg.OnionProxy)
		}
	} else {
		cfg.oniondial = cfg.dial
		cfg.onionlookup = cfg.lookup
	}

	// Specifying --noonion means the onion address dial and DNS resolution
	// (lookup) functions result in an error.
	if cfg.NoOnion {
		cfg.oniondial = func(a, b string) (net.Conn, error) {
			return nil, errors.New("tor has been disabled")
		}
		cfg.onionlookup = func(a string) ([]net.IP, error) {
			return nil, errors.New("tor has been disabled")
		}
	}

	// Warn about missing config file only after all other configuration is
	// done.  This prevents the warning on help messages and invalid
	// options.  Note this should go directly before the return.
	if configFileError != nil {
		dcrdLog.Warnf("%v", configFileError)
	}

	return &cfg, remainingArgs, nil
}

// dcrdDial connects to the address on the named network using the appropriate
// dial function depending on the address and configuration options.  For
// example, .onion addresses will be dialed using the onion specific proxy if
// one was specified, but will otherwise use the normal dial function (which
// could itself use a proxy or not).
func dcrdDial(network, address string) (net.Conn, error) {
	if strings.HasSuffix(address, ".onion") {
		return cfg.oniondial(network, address)
	}
	return cfg.dial(network, address)
}

// dcrdLookup returns the correct DNS lookup function to use depending on the
// passed host and configuration options.  For example, .onion addresses will be
// resolved using the onion specific proxy if one was specified, but will
// otherwise treat the normal proxy as tor unless --noonion was specified in
// which case the lookup will fail.  Meanwhile, normal IP addresses will be
// resolved using tor if a proxy was specified unless --noonion was also
// specified in which case the normal system DNS resolver will be used.
func dcrdLookup(host string) ([]net.IP, error) {
	if strings.HasSuffix(host, ".onion") {
		return cfg.onionlookup(host)
	}
	return cfg.lookup(host)
}
