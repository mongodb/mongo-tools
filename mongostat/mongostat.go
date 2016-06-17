// Package mongostat provides an overview of the status of a currently running mongod or mongos instance.
package mongostat

import (
	"strings"
	"sync"
	"time"

	"github.com/mongodb/mongo-tools/common/db"
	"github.com/mongodb/mongo-tools/common/log"
	"github.com/mongodb/mongo-tools/common/options"
	"github.com/mongodb/mongo-tools/mongostat/stat_consumer"
	"github.com/mongodb/mongo-tools/mongostat/stat_consumer/line"
	"github.com/mongodb/mongo-tools/mongostat/status"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// MongoStat is a container for the user-specified options and
// internal cluster state used for running mongostat.
type MongoStat struct {
	// Generic mongo tool options.
	Options *options.ToolOptions

	// Mongostat-specific output options.
	StatOptions *StatOptions

	// How long to sleep between printing the rows, and polling the server.
	SleepInterval time.Duration

	// New nodes can be "discovered" by any other node by sending a hostname
	// on this channel.
	Discovered chan string

	// A map of hostname -> NodeMonitor for all the hosts that
	// are being monitored.
	Nodes map[string]*NodeMonitor

	// ClusterMonitor to manage collecting and printing the stats from all nodes.
	Cluster ClusterMonitor

	// Mutex to handle safe concurrent adding to or looping over discovered nodes.
	nodesLock sync.RWMutex

	// Internal storage of the name the user seeded with, for error checking.
	startNode string
}

// ConfigShard holds a mapping for the format of shard hosts as they
// appear in the config.shards collection.
type ConfigShard struct {
	Id   string `bson:"_id"`
	Host string `bson:"host"`
}

// NodeMonitor contains the connection pool for a single host and collects the
// mongostat data for that host on a regular interval.
type NodeMonitor struct {
	host            string
	sessionProvider *db.SessionProvider

	// The time at which the node monitor last processed an update successfully.
	LastUpdate time.Time

	// The most recent error encountered when collecting stats for this node.
	Err error
}

// SyncClusterMonitor is an implementation of ClusterMonitor that writes output
// synchronized with the timing of when the polling samples are collected.
// Only works with a single host at a time.
type SyncClusterMonitor struct {
	// Channel to listen for incoming stat data
	ReportChan chan *status.ServerStatus

	// Channel to listen for incoming errors
	ErrorChan chan *status.NodeError

	// Creates and consumes StatLines using ServerStatuses
	Consumer *stat_consumer.StatConsumer
}

// ClusterMonitor maintains an internal representation of a cluster's state,
// which can be refreshed with calls to Update(), and dumps output representing
// this internal state on an interval.
type ClusterMonitor interface {
	// Monitor() triggers monitoring and dumping output to begin
	// maxRows is the number of times to dump output before exiting. If <0,
	// Monitor() will run indefinitely.
	// done is a channel to send an error if one is encountered. A nil value will
	// be sent on this channel if Monitor() completes with no error.
	// sleep is the interval to sleep between output dumps.
	Monitor(maxRows int, done chan error, sleep time.Duration, startNode string)

	// Update signals the ClusterMonitor implementation to refresh its internal
	// state using the data contained in the provided ServerStatus.
	Update(stat *status.ServerStatus, err *status.NodeError)
}

// AsyncClusterMonitor is an implementation of ClusterMonitor that writes output
// gotten from polling samples collected asynchronously from one or more servers.
type AsyncClusterMonitor struct {
	Discover bool

	// Channel to listen for incoming stat data
	ReportChan chan *status.ServerStatus

	// Channel to listen for incoming errors
	ErrorChan chan *status.NodeError

	// Map of hostname -> latest stat data for the host
	LastStatLines map[string]*line.StatLine

	// Mutex to protect access to LastStatLines
	mapLock sync.Mutex

	// Creates and consumes StatLines using ServerStatuses
	Consumer *stat_consumer.StatConsumer
}

// Update refreshes the internal state of the cluster monitor with the data
// in the StatLine. SyncClusterMonitor's implementation of Update blocks
// until it has written out its state, so that output is always dumped exactly
// once for each poll.
func (cluster *SyncClusterMonitor) Update(stat *status.ServerStatus, err *status.NodeError) {
	if err != nil {
		cluster.ErrorChan <- err
		return
	}
	cluster.ReportChan <- stat
}

// Monitor waits for data on the cluster's report channel. Once new data comes
// in, it formats and then displays it to stdout.
func (cluster *SyncClusterMonitor) Monitor(maxRows int, done chan error, sleep time.Duration, _ string) {
	go func() {
		rowCount := 0
		receivedData := false
		for {
			var statLine *line.StatLine
			var ok bool
			select {
			case stat := <-cluster.ReportChan:
				statLine, ok = cluster.Consumer.Update(stat)
				if !ok {
					continue
				}
			case err := <-cluster.ErrorChan:
				if !receivedData {
					done <- err
					return
				}
				statLine = &line.StatLine{
					Error:  err,
					Fields: map[string]string{"host": err.Host},
				}
			}
			receivedData = true
			cluster.Consumer.FormatLines([]*line.StatLine{statLine})
			rowCount++
			if maxRows > 0 && rowCount >= maxRows {
				break
			}
		}
		done <- nil
	}()
}

// updateHostInfo updates the internal map with the given StatLine data.
// Safe for concurrent accestatus.
func (cluster *AsyncClusterMonitor) updateHostInfo(stat *line.StatLine) {
	cluster.mapLock.Lock()
	defer cluster.mapLock.Unlock()
	host := stat.Fields["host"]
	cluster.LastStatLines[host] = stat
}

// printSnapshot formats and dumps the current state of all the stats collected.
func (cluster *AsyncClusterMonitor) printSnapshot() {
	cluster.mapLock.Lock()
	defer cluster.mapLock.Unlock()
	lines := make([]*line.StatLine, 0, len(cluster.LastStatLines))
	for _, stat := range cluster.LastStatLines {
		lines = append(lines, stat)
	}
	cluster.Consumer.FormatLines(lines)
}

// Update sends a new StatLine on the cluster's report channel.
func (cluster *AsyncClusterMonitor) Update(stat *status.ServerStatus, err *status.NodeError) {
	if err != nil {
		cluster.ErrorChan <- err
		return
	}
	cluster.ReportChan <- stat
}

// The Async implementation of Monitor starts the goroutines that listen for incoming stat data,
// and dump snapshots at a regular interval.
func (cluster *AsyncClusterMonitor) Monitor(maxRows int, done chan error, sleep time.Duration, startNode string) {
	receivedData := false
	gotFirstStat := make(chan struct{})
	go func() {
		for {
			var statLine *line.StatLine
			var ok bool
			select {
			case stat := <-cluster.ReportChan:
				statLine, ok = cluster.Consumer.Update(stat)
				if !ok {
					continue
				}
			case err := <-cluster.ErrorChan:
				if !receivedData {
					done <- err
					return
				}
				statLine = &line.StatLine{
					Error:  err,
					Fields: map[string]string{"host": err.Host},
				}
			}
			cluster.updateHostInfo(statLine)

			// Wait until we get an update from the node the user seeded with
			host := statLine.Fields["host"]
			if !receivedData && host == startNode {
				receivedData = true
				gotFirstStat <- struct{}{}
			}
		}
	}()

	go func() {
		// Wait for the first bit of data to hit the channel before printing anything:
		<-gotFirstStat
		rowCount := 0
		for {
			time.Sleep(sleep)
			cluster.printSnapshot()
			rowCount++
			if maxRows > 0 && rowCount >= maxRows {
				break
			}
		}
		done <- nil
	}()
}

// NewNodeMonitor copies the same connection settings from an instance of
// ToolOptions, but monitors fullHost.
func NewNodeMonitor(opts options.ToolOptions, fullHost string) (*NodeMonitor, error) {
	optsCopy := opts
	host, port := parseHostPort(fullHost)
	optsCopy.Connection = &options.Connection{Host: host, Port: port}
	optsCopy.Direct = true
	sessionProvider, err := db.NewSessionProvider(optsCopy)
	if err != nil {
		return nil, err
	}
	return &NodeMonitor{
		host:            fullHost,
		sessionProvider: sessionProvider,
		LastUpdate:      time.Now(),
		Err:             nil,
	}, nil
}

// Report collects the stat info for a single node, and sends the result on
// the "out" channel. If it fails, the error is stored in the NodeMonitor Err field.
func (node *NodeMonitor) Poll(discover chan string, checkShards bool) (*status.ServerStatus, error) {
	stat := &status.ServerStatus{}
	log.Logf(log.DebugHigh, "getting session on server: %v", node.host)
	s, err := node.sessionProvider.GetSession()
	if err != nil {
		log.Logf(log.DebugLow, "got error getting session to server %v", node.host)
		return nil, err
	}
	log.Logf(log.DebugHigh, "got session on server: %v", node.host)

	// The read pref for the session must be set to 'secondary' to enable using
	// the driver with 'direct' connections, which disables the built-in
	// replset discovery mechanism since we do our own node discovery here.
	s.SetMode(mgo.Eventual, true)

	// Disable the socket timeout - otherwise if db.serverStatus() takes a long time on the server
	// side, the client will close the connection early and report an error.
	s.SetSocketTimeout(0)
	defer s.Close()

	err = s.DB("admin").Run(bson.D{{"serverStatus", 1}, {"recordStats", 0}}, stat)
	if err != nil {
		log.Logf(log.DebugLow, "got error calling serverStatus against server %v", node.host)
		return nil, err
	}

	node.Err = nil
	stat.SampleTime = time.Now()

	if stat.Repl != nil && discover != nil {
		for _, host := range stat.Repl.Hosts {
			discover <- host
		}
		for _, host := range stat.Repl.Passives {
			discover <- host
		}
	}
	stat.Host = node.host
	if discover != nil && stat != nil && status.IsMongos(stat) && checkShards {
		log.Logf(log.DebugLow, "checking config database to discover shards")
		shardCursor := s.DB("config").C("shards").Find(bson.M{}).Iter()
		shard := ConfigShard{}
		for shardCursor.Next(&shard) {
			shardHosts := strings.Split(shard.Host, ",")
			for _, shardHost := range shardHosts {
				discover <- shardHost
			}
		}
		shardCursor.Close()
	}

	return stat, nil
}

// Watch spawns a goroutine to continuously collect and process stats for
// a single node on a regular interval. At each interval, the goroutine triggers
// the node's Report function with the 'discover' and 'out' channels.
func (node *NodeMonitor) Watch(sleep time.Duration, discover chan string, cluster ClusterMonitor) {
	go func() {
		cycle := uint64(0)
		for {
			log.Logf(log.DebugHigh, "polling server: %v", node.host)
			stat, err := node.Poll(discover, cycle%10 == 1)

			if stat != nil {
				log.Logf(log.DebugHigh, "successfully got statline from host: %v", node.host)
			}
			var nodeError *status.NodeError
			if err != nil {
				nodeError = status.NewNodeError(node.host, err)
			}
			cluster.Update(stat, nodeError)
			time.Sleep(sleep)
			cycle++
		}
	}()
}

func parseHostPort(fullHostName string) (string, string) {
	if colon := strings.LastIndex(fullHostName, ":"); colon >= 0 {
		return fullHostName[0:colon], fullHostName[colon+1:]
	}
	return fullHostName, "27017"
}

// AddNewNode adds a new host name to be monitored and spawns
// the necessary goroutines to collect data from it.
func (mstat *MongoStat) AddNewNode(fullhost string) error {
	mstat.nodesLock.Lock()
	defer mstat.nodesLock.Unlock()

	if len(mstat.Nodes) == 0 {
		mstat.startNode = fullhost
	}

	if _, hasKey := mstat.Nodes[fullhost]; !hasKey {
		log.Logf(log.DebugLow, "adding new host to monitoring: %v", fullhost)
		// Create a new node monitor for this host.
		node, err := NewNodeMonitor(*mstat.Options, fullhost)
		if err != nil {
			return err
		}
		mstat.Nodes[fullhost] = node
		node.Watch(mstat.SleepInterval, mstat.Discovered, mstat.Cluster)
	}
	return nil
}

// Run is the top-level function that starts the monitoring
// and discovery goroutines
func (mstat *MongoStat) Run() error {
	if mstat.Discovered != nil {
		go func() {
			for {
				newHost := <-mstat.Discovered
				err := mstat.AddNewNode(newHost)
				if err != nil {
					log.Logf(log.Always, "can't add discovered node %v: %v", newHost, err)
				}
			}
		}()
	}

	// Channel to wait
	finished := make(chan error)
	go mstat.Cluster.Monitor(mstat.StatOptions.RowCount, finished, mstat.SleepInterval, mstat.startNode)
	return <-finished
}
