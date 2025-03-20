/**
 * Sets up a replica set. To make the set running, call {@link #startSet},
 * followed by {@link #initiate} (and optionally,
 * {@link #awaitSecondaryNodes} to block till the  set is fully operational).
 * Note that some of the replica start up parameters are not passed here,
 * but to the #startSet method.
 *
 * @param {Object|string} opts If this value is a string, it specifies the connection string for
 *      a MongoD host to be used for recreating a ReplSetTest from. Otherwise, if it is an object,
 *      it must have the following contents:
 *
 *   {
 *     name {string}: name of this replica set. Default: 'testReplSet'
 *     host {string}: name of the host machine. Hostname will be used
 *        if not specified.
 *     useHostName {boolean}: if true, use hostname of machine,
 *        otherwise use localhost
 *     nodes {number|Object|Array.<Object>}: number of replicas. Default: 0.
 *        Can also be an Object (or Array).
 *        Format for Object:
 *          {
 *            <any string>: replica member option Object. @see MongoRunner.runMongod
 *            <any string2>: and so on...
 *          }
 *          If object has a special "rsConfig" field then those options will be used for each
 *          replica set member config options when used to initialize the replica set, or
 *          building the config with getReplSetConfig()
 *
 *        Format for Array:
 *           An array of replica member option Object. @see MongoRunner.runMongod
 *
 *        Note: For both formats, a special boolean property 'arbiter' can be
 *          specified to denote a member is an arbiter.
 *
 *        Note: A special "bridgeOptions" property can be specified in both the object and array
 *           formats to configure the options for the mongobridge corresponding to that node. These
 *           options are merged with the opts.bridgeOptions options, where the node-specific options
 *           take precedence.
 *
 *     nodeOptions {Object}: Command-line options to apply to all nodes in the replica set.
 *        Format for Object:
 *          { cmdline-param-with-no-arg : "",
 *            param-with-arg : arg }
 *        This turns into "mongod --cmdline-param-with-no-arg --param-with-arg arg"
 *
 *     causallyConsistent {boolean}: Specifies whether the connections to the replica set nodes
 *        should be created with the 'causal consistency' flag enabled, which means they will gossip
 *        the cluster time and add readConcern afterClusterTime where applicable. Defaults to false.
 *
 *     oplogSize {number}: Default: 40
 *     useSeedList {boolean}: Use the connection string format of this set
 *        as the replica set name (overrides the name property). Default: false
 *     keyFile {string}
 *     protocolVersion {number}: protocol version of replset used by the replset initiation.
 *
 *     useBridge {boolean}: If true, then a mongobridge process is started for each node in the
 *        replica set. Both the replica set configuration and the connections returned by startSet()
 *        will be references to the proxied connections. Defaults to false.
 *
 *     bridgeOptions {Object}: Options to apply to all mongobridge processes. Defaults to {}.
 *
 *     settings {object}: Setting used in the replica set config document.
 *        Example:
 *              settings: { chainingAllowed: false, ... }
 *
 *     seedRandomNumberGenerator {boolean}: Indicates whether the random number generator should
 *        be seeded when randomBinVersions is true. For ReplSetTests started by ShardingTest, the
 *        seed is generated as part of ShardingTest.
 *
 *     useAutoBootstrapProcedure {boolean} If true, follow the procedure for auto-bootstrapped
 *        replica sets.
 *   }
 *
 * Member variables:
 *  nodes {Array.<Mongo>} - connection to replica set members
 */

/* global retryOnRetryableError */

var ReplSetTest = function ReplSetTest(opts) {
    'use strict';

    if (!(this instanceof ReplSetTest)) {
        return new ReplSetTest(opts);
    }

    // Replica set health states
    const Health = {UP: 1, DOWN: 0};

    // Some code still references kDefaultTimeoutMS as a (non-static) member variable, so make sure
    // it's still accessible that way.
    this.kDefaultTimeoutMS = ReplSetTest.kDefaultTimeoutMS;
    const kOplogName = 'oplog.rs';

    // Publicly exposed variables

    /**
     * Tries to load the 'jstests/libs/legacyThreadSupport.js' dependency. Returns true if the file
     * is loaded successfully, and false otherwise.
     */
    function tryLoadParallelTester() {
        if (typeof globalThis.Thread !== 'undefined') {
            return true;
        }

        try {
            /* eslint-disable-next-line no-restricted-syntax */
            load("jstests/libs/legacyThreadSupport.js");  // For Thread.
            return true;
        } catch (e) {
            return false;
        }
    }

    /**
     * Returns the config document reported from the specified connection.
     */
    function _replSetGetConfig(conn) {
        return assert.commandWorked(conn.adminCommand({replSetGetConfig: 1})).config;
    }

    /**
     * Invokes the 'hello' command on each individual node and returns the current primary, or false
     * if none is found. Populates the following cached values:
     * '_primary': the current primary
     * '_secondaries': all nodes other than '_primary' (note this includes arbiters)
     * '_liveNodes': all currently reachable nodes
     */
    function _callHello(rst) {
        rst._liveNodes = [];
        rst._primary = null;
        rst._secondaries = [];

        var twoPrimaries = false;
        let canAcceptWrites = false;
        // Ensure that only one node is in primary state.
        rst.nodes.forEach(function(node) {
            try {
                node.setSecondaryOk();
                var n = node.getDB('admin')._helloOrLegacyHello();
                rst._liveNodes.push(node);
                // We verify that the node has a valid config by checking if n.me exists. Then, we
                // check to see if the node is in primary state.
                if (n.me && n.me == n.primary) {
                    if (rst._primary) {
                        twoPrimaries = true;
                    } else {
                        rst._primary = node;
                        canAcceptWrites = n.isWritablePrimary || n.ismaster;
                    }
                } else {
                    rst._secondaries.push(node);
                }
            } catch (err) {
                print("ReplSetTest Could not call hello/ismaster on node " + node + ": " +
                      tojson(err));
                rst._secondaries.push(node);
            }
        });
        if (twoPrimaries || !rst._primary || !canAcceptWrites) {
            return false;
        }

        return rst._primary;
    }

    /**
     * Attempt to connect to all nodes and returns a list of secondaries in which the connection was
     * successful.
     */
    function _determineLiveSecondaries(rst) {
        _callHello(rst);
        return rst._secondaries.filter(function(n) {
            return rst._liveNodes.indexOf(n) !== -1;
        });
    }

    /**
     * For all unauthenticated connections passed in, authenticates them with the '__system' user.
     * If a connection is already authenticated, we will skip authentication for that connection and
     * assume that it already has the correct privileges. It is up to the caller of this function to
     * ensure that the connection is appropriately authenticated.
     */
    function asCluster(rst, conn, fn, keyFileParam = undefined) {
        let connArray = conn;
        if (conn.length == null)
            connArray = [conn];

        const unauthenticatedConns = connArray.filter(connection => {
            const connStatus = connection.adminCommand({connectionStatus: 1, showPrivileges: true});
            const connIsAuthenticated = connStatus.authInfo.authenticatedUsers.length > 0;
            return !connIsAuthenticated;
        });

        const connOptions = connArray[0].fullOptions || {};
        const authMode = connOptions.clusterAuthMode || connArray[0].clusterAuthMode ||
            jsTest.options().clusterAuthMode;

        keyFileParam = keyFileParam || connOptions.keyFile || rst.keyFile;
        let needsAuth = (keyFileParam || authMode === "x509" || authMode === "sendX509" ||
                         authMode === "sendKeyFile") &&
            unauthenticatedConns.length > 0;

        // There are few cases where we do not auth
        // 1. When transitioning to auth
        // 2. When cluster is running in x509 but shell was not started with TLS (i.e. sslSpecial
        // suite)
        if (needsAuth &&
            (connOptions.transitionToAuth !== undefined ||
             (authMode === "x509" && !connArray[0].isTLS()))) {
            needsAuth = false;
        }

        if (needsAuth) {
            return authutil.asCluster(unauthenticatedConns, keyFileParam, fn);
        } else {
            return fn();
        }
    }

    ReplSetTest.prototype.asCluster = function(conn, fn, keyFileParam = undefined) {
        return asCluster(this, conn, fn, keyFileParam);
    };

    /**
     * Returns 'true' if the "conn" has been configured to run without journaling enabled.
     */
    function _isRunningWithoutJournaling(rst, conn) {
        var result = asCluster(rst, conn, function() {
            // Persistent storage engines (WT) can only run with journal enabled.
            var serverStatus = assert.commandWorked(conn.adminCommand({serverStatus: 1}));
            if (serverStatus.storageEngine.hasOwnProperty('persistent')) {
                if (serverStatus.storageEngine.persistent) {
                    return false;
                }
            }
            return true;
        });
        return result;
    }

    /**
     * Wrap a function so it can accept a node id or connection as its first argument. The argument
     * is converted to a connection.
     */
    function _nodeParamToConn(wrapped) {
        return function(node, ...wrappedArgs) {
            if (node.getDB) {
                return wrapped.call(this, node, ...wrappedArgs);
            }

            assert(this.nodes.hasOwnProperty(node), `${node} not found in own nodes`);
            return wrapped.call(this, this.nodes[node], ...wrappedArgs);
        };
    }

    /**
     * Wrap a function so it can accept a node id or connection as its first argument. The argument
     * is converted to a node id.
     */
    function _nodeParamToId(wrapped) {
        return function(node, ...wrappedArgs) {
            if (node.getDB) {
                return wrapped.call(this, this.getNodeId(node), ...wrappedArgs);
            }

            assert(Number.isInteger(node), `node must be an integer, not ${node}`);
            return wrapped.call(this, node, ...wrappedArgs);
        };
    }

    /**
     * Wrap a function so it accepts a single node or list of them as its first argument. The
     * function is called once per node provided.
     */
    function _nodeParamToSingleNode(wrapped) {
        return function(node, ...wrappedArgs) {
            if (node.hasOwnProperty('length')) {
                let returnValueList = [];
                for (let i = 0; i < node.length; i++) {
                    returnValueList.push(wrapped.call(this, node[i], ...wrappedArgs));
                }

                return returnValueList;
            }

            return wrapped.call(this, node, ...wrappedArgs);
        };
    }

    /**
     * Helper functions for setting/clearing a failpoint.
     */
    function setFailPoint(node, failpoint, data = {}) {
        print("Setting fail point " + failpoint);
        assert.commandWorked(
            node.adminCommand({configureFailPoint: failpoint, mode: "alwaysOn", data: data}));
    }

    function clearFailPoint(node, failpoint) {
        print("Clearing fail point " + failpoint);
        assert.commandWorked(node.adminCommand({configureFailPoint: failpoint, mode: "off"}));
    }

    /**
     * Wait for a rs indicator to go to a particular state or states.
     *
     * @param node is a single node or list of nodes, by id or conn
     * @param states is a single state or list of states
     * @param ind is the indicator specified
     * @param timeout how long to wait for the state to be reached
     * @param reconnectNode indicates that we should reconnect to a node that stepped down
     */
    ReplSetTest.prototype._waitForIndicator = _nodeParamToSingleNode(_nodeParamToConn(function(
        node, ind, states, timeout, reconnectNode) {
        timeout = timeout || this.kDefaultTimeoutMS;
        if (reconnectNode === undefined) {
            reconnectNode = true;
        }

        if (!states.length) {
            states = [states];
        }

        print("ReplSetTest waitForIndicator " + ind + " on " + node);
        printjson(states);
        print("ReplSetTest waitForIndicator from node " + node);

        var lastTime = null;
        var currTime = new Date().getTime();
        var status;

        let foundState;
        assert.soon(() => {
            try {
                var conn = _callHello(this);
                if (!conn) {
                    conn = this._liveNodes[0];
                }

                // Try again to load connection
                if (!conn)
                    return false;

                if (reconnectNode instanceof Function) {
                    // Allow caller to perform tasks on reconnect.
                    reconnectNode(conn);
                }

                asCluster(this, conn, function() {
                    status = conn.getDB('admin').runCommand({replSetGetStatus: 1});
                });
            } catch (ex) {
                print("ReplSetTest waitForIndicator could not get status: " + tojson(ex));
                return false;
            }

            if (status.code == ErrorCodes.Unauthorized) {
                // If we're not authorized already, then we never will be.
                assert.commandWorked(status);  // throws
            }

            var printStatus = false;
            if (lastTime == null || (currTime = new Date().getTime()) - (1000 * 5) > lastTime) {
                if (lastTime == null) {
                    print("ReplSetTest waitForIndicator Initial status (timeout : " + timeout +
                          ") :");
                }

                printjson(status);
                lastTime = new Date().getTime();
                printStatus = true;
            }

            if (typeof status.members == 'undefined') {
                return false;
            }

            for (var i = 0; i < status.members.length; i++) {
                if (printStatus) {
                    print("Status for : " + status.members[i].name + ", checking " + node.host +
                          "/" + node.name);
                }

                if (status.members[i].name == node.host || status.members[i].name == node.name) {
                    for (var j = 0; j < states.length; j++) {
                        if (printStatus) {
                            print("Status -- " +
                                  " current state: " + status.members[i][ind] +
                                  ",  target state : " + states[j]);
                        }

                        if (typeof (states[j]) != "number") {
                            throw new Error("State was not an number -- type:" +
                                            typeof (states[j]) + ", value:" + states[j]);
                        }
                        if (status.members[i][ind] == states[j]) {
                            foundState = states[j];
                            return true;
                        }
                    }
                }
            }

            return false;
        }, "waiting for state indicator " + ind + " for " + timeout + "ms", timeout);

        // If we were waiting for the node to step down, wait until we can connect to it again,
        // since primaries close external connections upon stepdown. This ensures that the
        // connection to the node is usable after the function returns.
        if (reconnectNode && foundState === ReplSetTest.State.SECONDARY) {
            assert.soon(function() {
                try {
                    node.getDB("foo").bar.stats();
                    return true;
                } catch (e) {
                    return false;
                }
            }, "timed out waiting to reconnect to node " + node.name);
        }

        print("ReplSetTest waitForIndicator final status:");
        printjson(status);
    }));

    /**
     * Returns true if the OpTime is empty, else false.
     *
     * Empty OpTime Formats:
     *   PV0: Timestamp(0,0)
     *   PV1: {ts: Timestamp(0,0), t: NumberLong(-1)}
     */
    function _isEmptyOpTime(opTime) {
        if (!opTime.hasOwnProperty("ts") || !opTime.hasOwnProperty("t")) {
            return (opTime.getTime() == 0 && opTime.getInc() == 0);
        }
        return (opTime.ts.getTime() == 0 && opTime.ts.getInc() == 0 && opTime.t == -1);
    }

    /**
     * Returns the OpTime for the specified host by issuing replSetGetStatus.
     */
    function _getLastOpTime(rst, conn) {
        var replSetStatus = asCluster(
            rst,
            conn,
            () => assert.commandWorked(conn.getDB("admin").runCommand({replSetGetStatus: 1})));
        var connStatus = replSetStatus.members.filter(m => m.self)[0];
        var opTime = connStatus.optime;
        if (_isEmptyOpTime(opTime)) {
            throw new Error("last OpTime is empty -- connection: " + conn);
        }
        return opTime;
    }

    /**
     * Returns the {readConcern: majority} OpTime for the host.
     * This is the OpTime of the host's "majority committed" snapshot.
     * This function may return an OpTime with Timestamp(0,0) and Term(0) if read concern majority
     * is not enabled, or if there has not been a committed snapshot yet.
     */
    function _getReadConcernMajorityOpTime(conn) {
        var replSetStatus =
            assert.commandWorked(conn.getDB("admin").runCommand({replSetGetStatus: 1}));
        return (replSetStatus.OpTimes || replSetStatus.optimes).readConcernMajorityOpTime ||
            {ts: Timestamp(0, 0), t: NumberLong(0)};
    }

    /**
     * Returns the {readConcern: majority} OpTime for the host. Throws if not available.
     */
    ReplSetTest.prototype.getReadConcernMajorityOpTimeOrThrow = function(conn) {
        const majorityOpTime = _getReadConcernMajorityOpTime(conn);
        if (friendlyEqual(majorityOpTime, {ts: Timestamp(0, 0), t: NumberLong(0)})) {
            throw new Error("readConcern majority optime not available");
        }
        return majorityOpTime;
    };

    /**
     * Returns the last durable OpTime for the host if running with journaling.
     * Returns the last applied OpTime otherwise.
     */
    function _getDurableOpTime(rst, conn) {
        var replSetStatus = asCluster(
            rst,
            conn,
            () => assert.commandWorked(conn.getDB("admin").runCommand({replSetGetStatus: 1})));

        var opTimeType = "durableOpTime";
        if (_isRunningWithoutJournaling(rst, conn)) {
            opTimeType = "appliedOpTime";
        }
        var opTime = replSetStatus.optimes[opTimeType];
        if (_isEmptyOpTime(opTime)) {
            throw new Error("last durable OpTime is empty -- connection: " + conn);
        }
        return opTime;
    }

    /*
     * Compares Timestamp objects. Returns true if ts1 is 'earlier' than ts2, else false.
     */
    function _isEarlierTimestamp(ts1, ts2) {
        if (ts1.getTime() == ts2.getTime()) {
            return ts1.getInc() < ts2.getInc();
        }
        return ts1.getTime() < ts2.getTime();
    }

    /*
     * Returns true if the node can be elected primary of a replica set.
     */
    function _isElectable(node) {
        return !node.arbiterOnly && (node.priority === undefined || node.priority != 0);
    }

    /**
     * Returns list of nodes as host:port strings.
     */
    ReplSetTest.prototype.nodeList = function() {
        var list = [];
        for (var i = 0; i < this.ports.length; i++) {
            list.push(this.host + ":" + this.ports[i]);
        }

        return list;
    };

    ReplSetTest.prototype.getNodeId = function(node) {
        if (node.toFixed) {
            return parseInt(node);
        }

        for (var i = 0; i < this.nodes.length; i++) {
            if (this.nodes[i] == node) {
                return i;
            }
        }

        if (node instanceof ObjectId) {
            for (i = 0; i < this.nodes.length; i++) {
                if (this.nodes[i].runId == node) {
                    return i;
                }
            }
        }

        if (node.nodeId != null) {
            return parseInt(node.nodeId);
        }

        return undefined;
    };

    ReplSetTest.prototype.getPort = function(n) {
        var n = this.getNodeId(n);
        return this.ports[n];
    };

    ReplSetTest.prototype.getDbPath = function(node) {
        // Get a replica set node (check for use of bridge).
        const n = this.getNodeId(node);
        const replNode = this._useBridge ? this._unbridgedNodes[n] : this.nodes[n];
        return replNode.dbpath;
    };

    ReplSetTest.prototype._addPath = function(p) {
        if (!this._alldbpaths)
            this._alldbpaths = [p];
        else
            this._alldbpaths.push(p);

        return p;
    };

    ReplSetTest.prototype.getReplSetConfig = function() {
        var cfg = {};
        cfg._id = this.name;
        cfg.protocolVersion = 1;

        cfg.members = [];

        for (var i = 0; i < this.ports.length; i++) {
            var member = {};
            member._id = i;

            member.host = this.host;
            if (!member.host.includes('/')) {
                member.host += ":" + this.ports[i];
            }

            var nodeOpts = this.nodeOptions["n" + i];
            if (nodeOpts) {
                if (nodeOpts.arbiter) {
                    member.arbiterOnly = true;
                }

                if (nodeOpts.rsConfig) {
                    Object.extend(member, nodeOpts.rsConfig);
                }
            }

            cfg.members.push(member);
        }

        if (this._configSettings) {
            cfg.settings = this._configSettings;
        }

        return cfg;
    };

    ReplSetTest.prototype.getURL = function() {
        var hosts = [];

        for (var i = 0; i < this.ports.length; i++) {
            hosts.push(this.host + ":" + this.ports[i]);
        }

        return this.name + "/" + hosts.join(",");
    };

    /**
     * Starts each node in the replica set with the given options.
     *
     * @param options - The options passed to {@link MongoRunner.runMongod}
     * @param restart - Boolean indicating whether we are restarting the set (if true,
     *     then `forRestart` should have been passed as true to `stopSet()`.) Defaults to false.
     * @param isMixedVersionCluster - Boolean indicating whether this is a mixed version replica
     *     set. Defaults to false.
     * @param skipStepUpOnRestart - Boolean indicating that this method should skip attempting to
     *     step up a new primary after restarting the set. Defaults to false. This must be set to
     *     true when using the in-memory storage engine, as the replica set must be re-initiated
     *     by the test on restart before a node can be elected.
     *     This option has no effect if `restart` is not also passed as true.
     */
    ReplSetTest.prototype.startSet = function(
        options, restart, isMixedVersionCluster, skipStepUpOnRestart) {
        // If the caller has explicitly specified 'waitForConnect:false', then we will start up all
        // replica set nodes and return without waiting to connect to any of them.
        const skipWaitingForAllConnections = (options && options.waitForConnect === false);

        // Keep a copy of these options
        this.startSetOptions = options;

        // Start up without waiting for connections.
        this.startSetAsync(options, restart, isMixedVersionCluster);

        // Avoid waiting for connections to each node.
        if (skipWaitingForAllConnections) {
            print("ReplSetTest startSet skipping waiting for connections to all nodes in set '" +
                  this.name + "'");
            return this.nodes;
        }

        this.startSetAwait();

        // If the set is being restarted, by default we will try to find a node to step up
        // proactively rather than waiting for the election timeout.
        const triggerStepUp = (restart || (options && options.restart)) && !skipStepUpOnRestart;
        if (!triggerStepUp) {
            print("ReplSetTest startSet skipping stepping a new primary");
            return this.nodes;
        }

        print("ReplSetTest startSet attempting to step up a new primary");

        // Try to step up each node and stop after the first success.
        // We use asCluster as replSetStepUp requires auth.
        return asCluster(this, this.nodes, () => {
            for (const node of this.nodes) {
                if (_isElectable(node)) {
                    this.stepUp(node, {awaitReplicationBeforeStepUp: false});
                    return this.nodes;
                }
            }
            throw Error(
                "Restarted set but failed to get a node to step up, as none were electable");
        });
    };

    /**
     * Starts each node in the replica set with the given options without waiting for a connection
     * to any node. Call 'startSetAwait' subsequently to wait for startup of each node to complete.
     *
     * @param options - The options passed to {@link MongoRunner.runMongod}
     */
    ReplSetTest.prototype.startSetAsync = function(options, restart, isMixedVersionCluster) {
        print("ReplSetTest starting set '" + this.name + "'");
        this.startSetStartTime = new Date();  // Measure the execution time of node startup.

        if (options && options.keyFile) {
            this.keyFile = options.keyFile;
        }

        if (options) {
            this.startOptions = options;
        }

        if (jsTest.options().useRandomBinVersionsWithinReplicaSet &&
            this.seedRandomNumberGenerator) {
            // Set the random seed to the value passed in by TestData. The seed is undefined
            // by default. For sharded clusters, the seed is already initialized as part of
            // ShardingTest.
            Random.setRandomFixtureSeed();
        }

        // If the caller has explicitly set 'waitForConnect', then we prefer that. Otherwise we
        // default to not waiting for a connection. We merge the options object with a new field so
        // as to not modify the original options object that was passed in.
        options = options || {};
        options = (options.waitForConnect === undefined)
            ? Object.merge(options, {waitForConnect: false})
            : options;

        // Start up each node without waiting to connect. This allows startup of replica set nodes
        // to proceed in parallel.
        for (let n = 0; n < this.ports.length; n++) {
            if (n == 0 && this.useAutoBootstrapProcedure && !this._hasAcquiredAutoGeneratedName) {
                // Must wait for connect in order to extract the auto-generated replica set name.
                options.waitForConnect = true;
            }

            this.start(n, options, restart, false, isMixedVersionCluster);
        }
        return this.nodes;
    };

    /**
     * Waits for startup of each replica set node to complete by waiting until a connection can be
     * made to each.
     */
    ReplSetTest.prototype.startSetAwait = function() {
        // Wait until we can establish a connection to each node before proceeding.
        for (let n = 0; n < this.ports.length; n++) {
            this._waitForInitialConnection(n);
        }

        print("ReplSetTest startSet, nodes: " + tojson(this.nodes));

        print("ReplSetTest startSet took " + (new Date() - this.startSetStartTime) + "ms for " +
              this.nodes.length + " nodes.");
        return this.nodes;
    };

    /**
     * Blocks until the secondary nodes have completed recovery and their roles are known. Blocks on
     * all secondary nodes or just 'secondaries', if specified. Does not wait for all 'newlyAdded'
     * fields to be removed by default.
     */
    ReplSetTest.prototype.awaitSecondaryNodes = function(
        timeout, secondaries, retryIntervalMS, waitForNewlyAddedRemoval) {
        timeout = timeout || this.kDefaultTimeoutMS;
        retryIntervalMS = retryIntervalMS || 200;

        assert.soonNoExcept(() => {
            // Reload who the current secondaries are
            this.getPrimary(timeout);

            var secondariesToCheck = secondaries || this._secondaries;
            var len = secondariesToCheck.length;
            var ready = true;

            for (var i = 0; i < len; i++) {
                var hello = secondariesToCheck[i].getDB('admin')._helloOrLegacyHello();
                var arbiter = (hello.arbiterOnly === undefined ? false : hello.arbiterOnly);
                ready = ready && (hello.secondary || arbiter);
            }

            return ready;
        }, "Awaiting secondaries", timeout, retryIntervalMS);

        // We can only wait for newlyAdded field removal if test commands are enabled.
        if (waitForNewlyAddedRemoval && jsTest.options().enableTestCommands) {
            this.waitForAllNewlyAddedRemovals();
        }
    };

    /**
     * A special version of awaitSecondaryNodes() used exclusively by rollback_test.js.
     * Wraps around awaitSecondaryNodes() itself and checks for an unrecoverable rollback
     * if it throws.
     */
    ReplSetTest.prototype.awaitSecondaryNodesForRollbackTest = function(
        timeout, secondaries, connToCheckForUnrecoverableRollback, retryIntervalMS) {
        retryIntervalMS = retryIntervalMS || 200;
        try {
            MongoRunner.runHangAnalyzer.disable();
            this.awaitSecondaryNodes(timeout, secondaries, retryIntervalMS);
            MongoRunner.runHangAnalyzer.enable();
        } catch (originalEx) {
            // There is a special case where we expect the (rare) possibility of unrecoverable
            // rollbacks with EMRC:false in rollback suites with unclean shutdowns.
            jsTestLog("Exception in 'awaitSecondaryNodes', checking for unrecoverable rollback");
            if (connToCheckForUnrecoverableRollback) {
                const conn = connToCheckForUnrecoverableRollback;

                const statusRes = assert.commandWorked(conn.adminCommand({replSetGetStatus: 1}));
                const isRecovering = (statusRes.myState === ReplSetTest.State.RECOVERING);
                const hasNoSyncSource = (statusRes.syncSourceId === -1);

                const cmdLineOptsRes = assert.commandWorked(conn.adminCommand("getCmdLineOpts"));
                const hasEMRCFalse =
                    (cmdLineOptsRes.parsed.replication.enableMajorityReadConcern === false);

                if (isRecovering && hasNoSyncSource && hasEMRCFalse) {
                    try {
                        const n = this.getNodeId(conn);
                        const connToCheck =
                            this._useBridge ? this._unbridgedNodes[n] : this.nodes[n];
                        // Confirm that the node is unable to recover after rolling back.
                        checkLog.contains(
                            connToCheck,
                            "remote oplog does not contain entry with optime matching our required optime",
                            120 * 1000);
                    } catch (checkLogEx) {
                        MongoRunner.runHangAnalyzer.enable();
                        throw originalEx;
                    }
                    // Add this info to the original exception.
                    originalEx.unrecoverableRollbackDetected = true;
                }
            }
            // Re-throw the original exception in all cases.
            MongoRunner.runHangAnalyzer.enable();
            throw originalEx;
        }
    };

    /**
     * Blocks until the specified node says it's syncing from the given upstream node.
     */
    ReplSetTest.prototype.awaitSyncSource = function(node, upstreamNode, timeout) {
        print("Waiting for node " + node.name + " to start syncing from " + upstreamNode.name);
        var status = null;
        assert.soonNoExcept(
            function() {
                status = node.getDB("admin").runCommand({replSetGetStatus: 1});
                for (var j = 0; j < status.members.length; j++) {
                    if (status.members[j].self) {
                        return status.members[j].syncSourceHost === upstreamNode.host;
                    }
                }
                return false;
            },
            "Awaiting node " + node + " syncing from " + upstreamNode + ": " + tojson(status),
            timeout);
    };

    /**
     * Blocks until each node agrees that all other nodes have applied the most recent oplog entry.
     */
    ReplSetTest.prototype.awaitNodesAgreeOnAppliedOpTime = function(timeout, nodes) {
        timeout = timeout || this.kDefaultTimeoutMS;
        nodes = nodes || this.nodes;

        assert.soon(function() {
            let appliedOpTimeConsensus = undefined;
            for (let i = 0; i < nodes.length; i++) {
                let replSetGetStatus;
                try {
                    replSetGetStatus = nodes[i].adminCommand({replSetGetStatus: 1});
                } catch (e) {
                    print("AwaitNodesAgreeOnAppliedOpTime: Retrying because node " + nodes[i].name +
                          " failed to execute replSetGetStatus: " + tojson(e));
                    return false;
                }
                assert.commandWorked(replSetGetStatus);

                if (appliedOpTimeConsensus === undefined) {
                    if (replSetGetStatus.optimes) {
                        appliedOpTimeConsensus = replSetGetStatus.optimes.appliedOpTime;
                    } else {
                        // Older versions of mongod do not include an 'optimes' field in the
                        // replSetGetStatus response. We instead pull an optime from the first
                        // replica set member that includes one in its status. All we need here is
                        // any initial value that we can compare to all the other optimes.
                        let optimeMembers = replSetGetStatus.members.filter(m => m.optime);
                        assert(optimeMembers.length > 0,
                               "AwaitNodesAgreeOnAppliedOpTime: replSetGetStatus did not " +
                                   "include optimes for any members: " + tojson(replSetGetStatus));
                        appliedOpTimeConsensus = optimeMembers[0].optime;
                    }

                    assert(appliedOpTimeConsensus,
                           "AwaitNodesAgreeOnAppliedOpTime: missing appliedOpTime in " +
                               "replSetGetStatus: " + tojson(replSetGetStatus));
                }

                if (replSetGetStatus.optimes &&
                    !friendlyEqual(replSetGetStatus.optimes.appliedOpTime,
                                   appliedOpTimeConsensus)) {
                    print("AwaitNodesAgreeOnAppliedOpTime: Retrying because node " + nodes[i].name +
                          " has appliedOpTime " + tojson(replSetGetStatus.optimes.appliedOpTime) +
                          " that does not match the previously observed appliedOpTime " +
                          tojson(appliedOpTimeConsensus));
                    return false;
                }

                for (let j = 0; j < replSetGetStatus.members.length; j++) {
                    if (replSetGetStatus.members[j].state == ReplSetTest.State.ARBITER) {
                        // ARBITER nodes do not apply oplog entries and do not have an 'optime'
                        // field.
                        continue;
                    }

                    if (!friendlyEqual(replSetGetStatus.members[j].optime,
                                       appliedOpTimeConsensus)) {
                        print("AwaitNodesAgreeOnAppliedOpTime: Retrying because node " +
                              nodes[i].name + " sees optime " +
                              tojson(replSetGetStatus.members[j].optime) + " on node " +
                              replSetGetStatus.members[j].name + " but expects to see optime " +
                              tojson(appliedOpTimeConsensus));
                        return false;
                    }
                }
            }

            print(
                "AwaitNodesAgreeOnAppliedOpTime: All nodes agree that all ops are applied up to " +
                tojson(appliedOpTimeConsensus));
            return true;
        }, "Awaiting nodes to agree that all ops are applied across replica set", timeout);
    };

    ReplSetTest.prototype._findHighestPriorityNodes = function(config) {
        let highestPriority = 0;
        let highPriorityNodes = [];
        for (let i = 0; i < config.members.length; i++) {
            const member = config.members[i];
            if (member.priority > highestPriority) {
                highestPriority = member.priority;
                highPriorityNodes = [this.nodes[i]];
            } else if (member.priority === highestPriority) {
                highPriorityNodes.push(this.nodes[i]);
            }
        }
        return highPriorityNodes;
    };

    /**
     * Blocks until the node with the highest priority is the primary.  If there are multiple
     * nodes tied for highest priority, waits until one of them is the primary.
     */
    ReplSetTest.prototype.awaitHighestPriorityNodeIsPrimary = function(timeout) {
        timeout = timeout || this.kDefaultTimeoutMS;

        // First figure out the set of highest priority nodes.
        const config = asCluster(this, this.nodes, () => this.getReplSetConfigFromNode());
        const highPriorityNodes = this._findHighestPriorityNodes(config);

        // Now wait for the primary to be one of the highest priority nodes.
        assert.soon(
            () => {
                return highPriorityNodes.includes(this.getPrimary());
            },
            () => {
                return "Expected primary to be one of: " + tojson(highPriorityNodes) +
                    ", but found primary to be: " + tojson(this.getPrimary());
            },
            timeout);

        // Finally wait for all nodes to agree on the primary.
        this.awaitNodesAgreeOnPrimary(timeout);
        const primary = this.getPrimary();
        assert(highPriorityNodes.includes(primary),
               "Primary switched away from highest priority node.  Found primary: " +
                   tojson(primary) + ", but expected one of: " + tojson(highPriorityNodes));
    };

    /**
     * Blocks until all nodes agree on who the primary is.
     * Unlike awaitNodesAgreeOnPrimary, this does not require that all nodes are authenticated.
     */
    ReplSetTest.prototype.awaitNodesAgreeOnPrimaryNoAuth = function(timeout, nodes) {
        timeout = timeout || this.kDefaultTimeoutMS;
        nodes = nodes || this.nodes;

        print("AwaitNodesAgreeOnPrimaryNoAuth: Waiting for nodes to agree on any primary.");

        assert.soonNoExcept(function() {
            var primary;

            for (var i = 0; i < nodes.length; i++) {
                var hello = assert.commandWorked(nodes[i].getDB('admin')._helloOrLegacyHello());
                var nodesPrimary = hello.primary;
                // Node doesn't see a primary.
                if (!nodesPrimary) {
                    print("AwaitNodesAgreeOnPrimaryNoAuth: Retrying because " + nodes[i].name +
                          " does not see a primary.");
                    return false;
                }

                if (!primary) {
                    // If we haven't seen a primary yet, set it to this.
                    primary = nodesPrimary;
                } else if (primary !== nodesPrimary) {
                    print("AwaitNodesAgreeOnPrimaryNoAuth: Retrying because " + nodes[i].name +
                          " thinks the primary is " + nodesPrimary + " instead of " + primary);
                    return false;
                }
            }

            print("AwaitNodesAgreeOnPrimaryNoAuth: Nodes agreed on primary " + primary);
            return true;
        }, "Awaiting nodes to agree on primary", timeout);
    };

    /**
     * Blocks until all nodes agree on who the primary is.
     * If 'expectedPrimaryNode' is provided, ensure that every node is seeing this node as the
     * primary. Otherwise, ensure that all the nodes in the set agree with the first node on the
     * identity of the primary.
     */
    ReplSetTest.prototype.awaitNodesAgreeOnPrimary = function(
        timeout, nodes, expectedPrimaryNode, runHangAnalyzerOnTimeout = true) {
        timeout = timeout || this.kDefaultTimeoutMS;
        nodes = nodes || this.nodes;
        // indexOf will return the index of the expected node. If expectedPrimaryNode is undefined,
        // indexOf will return -1.
        const expectedPrimaryNodeIdx = this.nodes.indexOf(expectedPrimaryNode);
        if (expectedPrimaryNodeIdx === -1) {
            print("AwaitNodesAgreeOnPrimary: Waiting for nodes to agree on any primary.");
        } else {
            print("AwaitNodesAgreeOnPrimary: Waiting for nodes to agree on " +
                  expectedPrimaryNode.name + " as primary.");
        }

        assert.soonNoExcept(() => {
            var primary = expectedPrimaryNodeIdx;

            for (var i = 0; i < nodes.length; i++) {
                var replSetGetStatus =
                    assert.commandWorked(nodes[i].getDB("admin").runCommand({replSetGetStatus: 1}));
                var nodesPrimary = -1;
                for (var j = 0; j < replSetGetStatus.members.length; j++) {
                    if (replSetGetStatus.members[j].state === ReplSetTest.State.PRIMARY) {
                        // Node sees two primaries.
                        if (nodesPrimary !== -1) {
                            print("AwaitNodesAgreeOnPrimary: Retrying because " + nodes[i].name +
                                  " thinks both " + this.nodes[nodesPrimary].name + " and " +
                                  this.nodes[j].name + " are primary.");

                            return false;
                        }
                        nodesPrimary = j;
                    }
                }
                // Node doesn't see a primary.
                if (nodesPrimary < 0) {
                    print("AwaitNodesAgreeOnPrimary: Retrying because " + nodes[i].name +
                          " does not see a primary.");
                    return false;
                }

                if (primary < 0) {
                    print("AwaitNodesAgreeOnPrimary: " + nodes[i].name + " thinks the " +
                          " primary is " + this.nodes[nodesPrimary].name +
                          ". Other nodes are expected to agree on the same primary.");
                    // If the nodes haven't seen a primary yet, set primary to nodes[i]'s primary.
                    primary = nodesPrimary;
                } else if (primary !== nodesPrimary) {
                    print("AwaitNodesAgreeOnPrimary: Retrying because " + nodes[i].name +
                          " thinks the primary is " + this.nodes[nodesPrimary].name +
                          " instead of " + this.nodes[primary].name);
                    return false;
                }
            }

            print("AwaitNodesAgreeOnPrimary: Nodes agreed on primary " + this.nodes[primary].name);
            return true;
        }, "Awaiting nodes to agree on primary timed out", timeout, undefined /*interval*/, {
            runHangAnalyzer: runHangAnalyzerOnTimeout
        });
    };

    /**
     * Blocking call, which will wait for a primary to be elected and become writable for some
     * pre-defined timeout. If a primary is available it will return a connection to it.
     * Otherwise throws an exception.
     */
    ReplSetTest.prototype.getPrimary = function(timeout, retryIntervalMS) {
        timeout = timeout || this.kDefaultTimeoutMS;
        retryIntervalMS = retryIntervalMS || 200;
        var primary = null;

        assert.soonNoExcept(() => {
            primary = _callHello(this);
            return primary;
        }, "Finding primary", timeout, retryIntervalMS);

        return primary;
    };

    ReplSetTest.prototype.awaitNoPrimary = function(msg, timeout) {
        msg = msg || "Timed out waiting for there to be no primary in replset: " + this.name;
        timeout = timeout || this.kDefaultTimeoutMS;

        assert.soonNoExcept(() => {
            return _callHello(this) == false;
        }, msg, timeout);
    };

    ReplSetTest.prototype.getSecondaries = function(timeout) {
        var primary = this.getPrimary(timeout);
        var secs = [];
        for (var i = 0; i < this.nodes.length; i++) {
            if (this.nodes[i] != primary) {
                secs.push(this.nodes[i]);
            }
        }

        return secs;
    };

    ReplSetTest.prototype.getSecondary = function(timeout) {
        return this.getSecondaries(timeout)[0];
    };

    function isNodeArbiter(node) {
        return node.getDB('admin')._helloOrLegacyHello().arbiterOnly;
    }

    ReplSetTest.prototype.getArbiters = function() {
        let arbiters = [];
        for (let i = 0; i < this.nodes.length; i++) {
            const node = this.nodes[i];

            let isArbiter = false;

            assert.retryNoExcept(() => {
                isArbiter = isNodeArbiter(node);
                return true;
            }, `Could not call hello/isMaster on ${node}.`, 3, 1000);

            if (isArbiter) {
                arbiters.push(node);
            }
        }
        return arbiters;
    };

    ReplSetTest.prototype.getArbiter = function() {
        return this.getArbiters()[0];
    };

    ReplSetTest.prototype.status = function(timeout) {
        var primary = _callHello(this);
        if (!primary) {
            primary = this._liveNodes[0];
        }

        return primary.getDB("admin").runCommand({replSetGetStatus: 1});
    };

    /**
     * Adds a node to the replica set managed by this instance.
     */
    ReplSetTest.prototype.add = function(config) {
        var nextPort = this._allocatePortForNode();
        print("ReplSetTest Next port: " + nextPort);

        this.ports.push(nextPort);
        printjson(this.ports);

        if (this._useBridge) {
            this._unbridgedPorts.push(this._allocatePortForBridge());
        }

        if (this.isRouterServer) {
            const nextPort = this._allocatePortForNode();
            print("ReplSetTest Next router port: " + nextPort);

            this.routerPorts.push(nextPort);
            printjson(this.routerPorts);
        }

        var nextId = this.nodes.length;
        printjson(this.nodes);

        print("ReplSetTest nextId: " + nextId);
        return this.start(nextId, config);
    };

    /**
     * Calls stop() on the node identifed by nodeId and removes it from the list of nodes managed by
     * ReplSetTest.
     */
    ReplSetTest.prototype.remove = function(nodeId) {
        this.stop(nodeId);
        nodeId = this.getNodeId(nodeId);
        this.nodes.splice(nodeId, 1);
        this.ports.splice(nodeId, 1);

        if (this._useBridge) {
            this._unbridgedPorts.splice(nodeId, 1);
            this._unbridgedNodes.splice(nodeId, 1);
        }
        if (this.isRouterServer) {
            this.routerPorts.splice(nodeId, 1);
        }
    };

    /*
     * If journaling is disabled or we are using an ephemeral storage engine, set
     * 'writeConcernMajorityJournalDefault' to false for the given 'config' object. If the
     * 'writeConcernMajorityJournalDefault' field is already set, it does not override it,
     * and returns the 'config' object unchanged. Does not affect 'config' when running CSRS.
     */
    ReplSetTest.prototype._updateConfigIfNotDurable = function(config) {
        // Get a replica set node (check for use of bridge).
        var replNode = this._useBridge ? this._unbridgedNodes[0] : this.nodes[0];

        // Don't update replset config for sharding config servers since config servers always
        // require durable storage.
        if (replNode.hasOwnProperty("fullOptions") &&
            replNode.fullOptions.hasOwnProperty("configsvr")) {
            return config;
        }

        // Don't override existing value.
        var wcMajorityJournalField = "writeConcernMajorityJournalDefault";
        if (config.hasOwnProperty(wcMajorityJournalField)) {
            return config;
        }

        // Check journaling by sending commands through the bridge if it's used.
        if (_isRunningWithoutJournaling(this, this.nodes[0])) {
            config[wcMajorityJournalField] = false;
        }

        return config;
    };

    ReplSetTest.prototype._setDefaultConfigOptions = function(config) {
        // Update config for non journaling test variants
        this._updateConfigIfNotDurable(config);
        // Add protocolVersion if missing
        if (!config.hasOwnProperty('protocolVersion')) {
            config['protocolVersion'] = 1;
        }
    };

    ReplSetTest.prototype._notX509Auth = function(conn) {
        const nodeId = "n" + this.getNodeId(conn);
        const nodeOptions = this.nodeOptions[nodeId] || {};
        const options =
            (nodeOptions === {} || !this.startOptions) ? nodeOptions : this.startOptions;
        const authMode = options.clusterAuthMode;
        return authMode != "sendX509" && authMode != "x509" && authMode != "sendKeyFile";
    };

    function replSetCommandWithRetry(primary, cmd) {
        print("Running command with retry: " + tojson(cmd));
        const cmdName = Object.keys(cmd)[0];
        const errorMsg = `${cmdName} during initiate failed`;
        assert.retry(() => {
            const result = assert.commandWorkedOrFailedWithCode(
                primary.runCommand(cmd),
                [
                    ErrorCodes.NodeNotFound,
                    ErrorCodes.NewReplicaSetConfigurationIncompatible,
                    ErrorCodes.InterruptedDueToReplStateChange,
                    ErrorCodes.ConfigurationInProgress,
                    ErrorCodes.CurrentConfigNotCommittedYet,
                    ErrorCodes.NotWritablePrimary,
                ],
                errorMsg);
            return result.ok;
        }, errorMsg, 3, 5 * 1000);
    }

    /**
     * Wait until the config on the primary becomes replicated. Callers specify the primary in case
     * this must be called when two nodes are expected to be concurrently primary. This does not
     * necessarily wait for the config to be committed.
     */
    ReplSetTest.prototype.waitForConfigReplication = function(primary, nodes) {
        const nodeHosts = nodes ? tojson(nodes.map((n) => n.host)) : "all nodes";
        print("waitForConfigReplication: Waiting for the config on " + primary.host +
              " to replicate to " + nodeHosts);

        let configVersion = -2;
        let configTerm = -2;
        assert.soon(function() {
            const res = assert.commandWorked(primary.adminCommand({replSetGetStatus: 1}));
            const primaryMember = res.members.find((m) => m.self);
            configVersion = primaryMember.configVersion;
            configTerm = primaryMember.configTerm;
            function hasSameConfig(member) {
                return member.configVersion === primaryMember.configVersion &&
                    member.configTerm === primaryMember.configTerm;
            }
            let members = res.members;
            if (nodes) {
                members = res.members.filter((m) => nodes.some((node) => m.name === node.host));
            }
            return members.every((m) => hasSameConfig(m));
        });

        print("waitForConfigReplication: config on " + primary.host +
              " replicated successfully to " + nodeHosts + " with version " + configVersion +
              " and term " + configTerm);
    };

    /**
     * Waits for all 'newlyAdded' fields to be removed, for that config to be committed, and for
     * the in-memory and on-disk configs to match.
     */
    ReplSetTest.prototype.waitForAllNewlyAddedRemovals = function(timeout) {
        timeout = timeout || this.kDefaultTimeoutMS;
        print("waitForAllNewlyAddedRemovals: starting for set " + this.name);
        const primary = this.getPrimary();

        // Shadow 'db' so that we can call the function on the primary without a separate shell when
        // x509 auth is not needed.
        let db = primary.getDB('admin');
        runFnWithAuthOnPrimary(this, function() {
            assert.soon(function() {
                const getConfigRes =
                    assert.commandWorkedOrFailedWithCode(db.adminCommand({
                        replSetGetConfig: 1,
                        commitmentStatus: true,
                        $_internalIncludeNewlyAdded: true
                    }),
                                                         ErrorCodes.NotWritablePrimary);

                if (!getConfigRes.ok) {
                    print("waitForAllNewlyAddedRemovals: Retrying because the old primary " +
                          " stepped down");
                    return false;
                }

                const config = getConfigRes.config;
                for (let i = 0; i < config.members.length; i++) {
                    const memberConfig = config.members[i];
                    if (memberConfig.hasOwnProperty("newlyAdded")) {
                        assert(memberConfig["newlyAdded"] === true, config);
                        print("waitForAllNewlyAddedRemovals: Retrying because memberIndex " + i +
                              " is still 'newlyAdded'");
                        return false;
                    }
                }
                if (!getConfigRes.hasOwnProperty("commitmentStatus")) {
                    print(
                        "waitForAllNewlyAddedRemovals: Skipping wait due to no commitmentStatus." +
                        " Assuming this is an older version.");
                    return true;
                }

                if (!getConfigRes.commitmentStatus) {
                    print("waitForAllNewlyAddedRemovals: " +
                          "Retrying because primary's config isn't committed. " +
                          "Version: " + config.version + ", Term: " + config.term);
                    return false;
                }

                return true;
            });
        }, "waitForAllNewlyAddedRemovals");

        this.waitForConfigReplication(primary);

        print("waitForAllNewlyAddedRemovals: finished for set " + this.name);
    };

    /**
     * Runs replSetInitiate on the first node of the replica set.
     * Ensures that a primary is elected (not necessarily node 0).
     * initiate() should be preferred instead of this, but this is useful when the connections
     * aren't authorized to run replSetGetStatus.
     * TODO(SERVER-14017): remove this in favor of using initiate() everywhere.
     */
    ReplSetTest.prototype.initiateWithAnyNodeAsPrimary = function(cfg, initCmd, {
        doNotWaitForStableRecoveryTimestamp: doNotWaitForStableRecoveryTimestamp = false,
        doNotWaitForReplication: doNotWaitForReplication = false,
        doNotWaitForNewlyAddedRemovals: doNotWaitForNewlyAddedRemovals = false,
        doNotWaitForPrimaryOnlyServices: doNotWaitForPrimaryOnlyServices = false
    } = {}) {
        let startTime = new Date();  // Measure the execution time of this function.
        var primary = this.nodes[0].getDB("admin");
        var config = cfg || this.getReplSetConfig();
        var cmd = {};
        var cmdKey = initCmd || 'replSetInitiate';

        // Throw an exception if nodes[0] is unelectable in the given config.
        if (!_isElectable(config.members[0])) {
            throw Error("The node at index 0 must be electable");
        }

        // Start up a single node replica set then reconfigure to the correct size (if the config
        // contains more than 1 node), so the primary is elected more quickly.
        var originalMembers, originalSettings;
        if (config.members && config.members.length > 1) {
            originalMembers = config.members.slice();
            config.members = config.members.slice(0, 1);
            originalSettings = config.settings;
            delete config.settings;  // Clear settings to avoid tags referencing sliced nodes.
        }
        this._setDefaultConfigOptions(config);

        cmd[cmdKey] = config;

        // If this ReplSet is started using this.startSet and binVersions (ie:
        // rst.startSet({binVersion: [...]}) we need to make sure the binVersion combination is
        // valid.
        if (typeof (this.startSetOptions) === "object" &&
            this.startSetOptions.hasOwnProperty("binVersion") &&
            typeof (this.startSetOptions.binVersion) === "object") {
            let lastLTSSpecified = false;
            let lastContinuousSpecified = false;
            this.startSetOptions.binVersion.forEach(function(binVersion, _) {
                if (lastLTSSpecified === false) {
                    lastLTSSpecified = MongoRunner.areBinVersionsTheSame(binVersion, lastLTSFCV);
                }
                if ((lastContinuousSpecified === false) && (lastLTSFCV !== lastContinuousFCV)) {
                    lastContinuousSpecified =
                        MongoRunner.areBinVersionsTheSame(binVersion, lastContinuousFCV);
                }
            });
            if (lastLTSSpecified && lastContinuousSpecified) {
                throw new Error("Can only specify one of 'last-lts' and 'last-continuous' " +
                                "in binVersion, not both.");
            }
        }
        // Initiating a replica set with a single node will use "latest" FCV. This will
        // cause IncompatibleServerVersion errors if additional "last-lts"/"last-continuous" binary
        // version nodes are subsequently added to the set, since such nodes cannot set their FCV to
        // "latest". Therefore, we make sure the primary is "last-lts"/"last-continuous" FCV before
        // adding in nodes of different binary versions to the replica set.
        let lastLTSBinVersionWasSpecifiedForSomeNode = false;
        let lastContinuousBinVersionWasSpecifiedForSomeNode = false;
        let explicitBinVersionWasSpecifiedForSomeNode = false;
        Object.keys(this.nodeOptions).forEach(key => {
            let val = this.nodeOptions[key];
            if (typeof (val) === "object" && val.hasOwnProperty("binVersion")) {
                if (lastLTSBinVersionWasSpecifiedForSomeNode === false) {
                    lastLTSBinVersionWasSpecifiedForSomeNode =
                        MongoRunner.areBinVersionsTheSame(val.binVersion, lastLTSFCV);
                }
                if ((lastContinuousBinVersionWasSpecifiedForSomeNode === false) &&
                    (lastLTSFCV !== lastContinuousFCV)) {
                    lastContinuousBinVersionWasSpecifiedForSomeNode =
                        MongoRunner.areBinVersionsTheSame(val.binVersion, lastContinuousFCV);
                }
                explicitBinVersionWasSpecifiedForSomeNode = true;
            }
        });

        if (lastLTSBinVersionWasSpecifiedForSomeNode &&
            lastContinuousBinVersionWasSpecifiedForSomeNode) {
            throw new Error("Can only specify one of 'last-lts' and 'last-continuous' " +
                            "in binVersion, not both.");
        }

        // If no binVersions have been explicitly set, then we should be using the latest binary
        // version, which allows us to use the failpoint below.
        let explicitBinVersion =
            (this.startOptions !== undefined && this.startOptions.hasOwnProperty("binVersion")) ||
            explicitBinVersionWasSpecifiedForSomeNode ||
            jsTest.options().useRandomBinVersionsWithinReplicaSet;

        // If a test has explicitly disabled test commands or if we may be running an older mongod
        // version then we cannot utilize failpoints below, since they may not be supported on older
        // versions.
        const failPointsSupported = jsTest.options().enableTestCommands && !explicitBinVersion;

        // Skip waiting for new data to appear in the oplog buffer when transitioning to primary.
        // This makes step up much faster for a node that doesn't need to drain any oplog
        // operations. This is only an optimization so it's OK if we bypass it in some suites.
        if (failPointsSupported) {
            setFailPoint(this.nodes[0], "skipOplogBatcherWaitForData");
        }

        // replSetInitiate and replSetReconfig commands can fail with a NodeNotFound error if a
        // heartbeat times out during the quorum check. They may also fail with
        // NewReplicaSetConfigurationIncompatible on similar timeout during the config validation
        // stage while deducing isSelf(). This can fail with an InterruptedDueToReplStateChange
        // error when interrupted. We try several times, to reduce the chance of failing this way.
        const initiateStart = new Date();  // Measure the execution time of this section.

        if (this.useAutoBootstrapProcedure) {
            // Auto-bootstrap already initiates automatically on the first node, but if the
            // requested initiate is not empty, we need to apply the requested settings using
            // reconfig.
            if (cmd[cmdKey] != {}) {
                cmd["replSetReconfig"] = cmd[cmdKey];
                delete cmd[cmdKey];

                // We must increase the version of the new config for the reconfig
                // to succeed. The initial default config will always have a version of 1.
                cmd["replSetReconfig"].version = 2;
                replSetCommandWithRetry(primary, cmd);
            }
        } else {
            replSetCommandWithRetry(primary, cmd);
        }

        // Blocks until there is a primary. We use a faster retry interval here since we expect the
        // primary to be ready very soon. We also turn the failpoint off once we have a primary.
        this.getPrimary(this.kDefaultTimeoutMS, 25 /* retryIntervalMS */);
        if (failPointsSupported) {
            clearFailPoint(this.nodes[0], "skipOplogBatcherWaitForData");
        }

        print("ReplSetTest initiate command took " + (new Date() - initiateStart) + "ms for " +
              this.nodes.length + " nodes in set '" + this.name + "'");

        // Set the FCV to 'last-lts'/'last-continuous' if we are running a mixed version replica
        // set. If this is a config server, the FCV will be set as part of ShardingTest.
        // versions are supported with the useRandomBinVersionsWithinReplicaSet option.
        let setLastLTSFCV = (lastLTSBinVersionWasSpecifiedForSomeNode ||
                             jsTest.options().useRandomBinVersionsWithinReplicaSet == 'last-lts') &&
            !this.isConfigServer;
        let setLastContinuousFCV = !setLastLTSFCV &&
            (lastContinuousBinVersionWasSpecifiedForSomeNode ||
             jsTest.options().useRandomBinVersionsWithinReplicaSet == 'last-continuous') &&
            !this.isConfigServer;

        if ((setLastLTSFCV || setLastContinuousFCV) &&
            jsTest.options().replSetFeatureCompatibilityVersion) {
            const fcv = setLastLTSFCV ? lastLTSFCV : lastContinuousFCV;
            throw new Error(
                "The FCV will be set to '" + fcv + "' automatically when starting up a replica " +
                "set with mixed binary versions. Therefore, we expect an empty value for " +
                "'replSetFeatureCompatibilityVersion'.");
        }

        if (setLastLTSFCV || setLastContinuousFCV) {
            // Authenticate before running the command.
            asCluster(this, this.nodes, () => {
                let fcv = setLastLTSFCV ? lastLTSFCV : lastContinuousFCV;

                print("Setting feature compatibility version for replica set to '" + fcv + "'");
                // When latest is not equal to last-continuous, the transition to last-continuous is
                // not allowed. Setting fromConfigServer allows us to bypass this restriction and
                // test last-continuous.
                assert.commandWorked(this.getPrimary().adminCommand(
                    {setFeatureCompatibilityVersion: fcv, fromConfigServer: true, confirm: true}));
                checkFCV(this.getPrimary().getDB("admin"), fcv);

                // The server has a practice of adding a reconfig as part of upgrade/downgrade logic
                // in the setFeatureCompatibilityVersion command.
                print(
                    "Fetch the config version from primary since last-lts or last-continuous downgrade might " +
                    "perform a reconfig.");
                config.version = this.getReplSetConfigFromNode().version;
            });
        }

        // Wait for 2 keys to appear before adding the other nodes. This is to prevent replica
        // set configurations from interfering with the primary to generate the keys. One example
        // of problematic configuration are delayed secondaries, which impedes the primary from
        // generating the second key due to timeout waiting for write concern.
        let shouldWaitForKeys = true;
        if (this.waitForKeys != undefined) {
            shouldWaitForKeys = this.waitForKeys;
            print("Set shouldWaitForKeys from RS options: " + shouldWaitForKeys);
        } else {
            Object.keys(this.nodeOptions).forEach(key => {
                let val = this.nodeOptions[key];
                if (typeof (val) === "object" &&
                    (val.hasOwnProperty("shardsvr") ||
                     val.hasOwnProperty("binVersion") &&
                         // Should not wait for keys if version is less than 3.6
                         MongoRunner.compareBinVersions(val.binVersion, "3.6") == -1)) {
                    shouldWaitForKeys = false;
                    print("Set shouldWaitForKeys from node options: " + shouldWaitForKeys);
                }
            });
            if (this.startOptions != undefined) {
                let val = this.startOptions;
                if (typeof (val) === "object" &&
                    (val.hasOwnProperty("shardsvr") ||
                     val.hasOwnProperty("binVersion") &&
                         // Should not wait for keys if version is less than 3.6
                         MongoRunner.compareBinVersions(val.binVersion, "3.6") == -1)) {
                    shouldWaitForKeys = false;
                    print("Set shouldWaitForKeys from start options: " + shouldWaitForKeys);
                }
            }
        }
        /**
         * Blocks until the primary node generates cluster time sign keys.
         */
        if (shouldWaitForKeys) {
            asCluster(this, this.nodes, timeout => {
                print("Waiting for keys to sign $clusterTime to be generated");
                assert.soonNoExcept(timeout => {
                    var keyCnt = this.getPrimary(timeout)
                                     .getCollection('admin.system.keys')
                                     .find({purpose: 'HMAC'})
                                     .itcount();
                    return keyCnt >= 2;
                }, "Awaiting keys", timeout);
            });
        }

        // Allow nodes to find sync sources more quickly. We also turn down the heartbeat interval
        // to speed up the initiation process. We use a failpoint so that we can easily turn this
        // behavior on/off without doing a reconfig. This is only an optimization so it's OK if we
        // bypass it in some suites.
        if (failPointsSupported) {
            this.nodes.forEach(function(conn) {
                setFailPoint(conn, "forceSyncSourceRetryWaitForInitialSync", {retryMS: 25});
                setFailPoint(conn, "forceHeartbeatIntervalMS", {intervalMS: 200});
                setFailPoint(conn, "forceBgSyncSyncSourceRetryWaitMS", {sleepMS: 25});
            });
        }

        // Reconfigure the set to contain the correct number of nodes (if necessary).
        const reconfigStart = new Date();  // Measure duration of reconfig and awaitSecondaryNodes.
        if (originalMembers) {
            config.members = originalMembers;
            if (originalSettings) {
                config.settings = originalSettings;
            }
            config.version = config.version ? config.version + 1 : 2;

            // Nodes started with the --configsvr flag must have configsvr = true in their config.
            if (this.nodes[0].hasOwnProperty("fullOptions") &&
                this.nodes[0].fullOptions.hasOwnProperty("configsvr")) {
                config.configsvr = true;
            }

            // Add in nodes 1 at a time since non-force reconfig allows only single node
            // addition/removal.
            print("Reconfiguring replica set to add in other nodes");
            for (let i = 2; i <= originalMembers.length; i++) {
                print("ReplSetTest adding in node " + i);
                assert.soon(() => {
                    primary = this.getPrimary().getDB("admin");
                    const statusRes =
                        assert.commandWorked(primary.adminCommand({replSetGetStatus: 1}));
                    const primaryMember = statusRes.members.find((m) => m.self);
                    config.version = primaryMember.configVersion + 1;

                    config.members = originalMembers.slice(0, i);
                    cmd = {replSetReconfig: config, maxTimeMS: ReplSetTest.kDefaultTimeoutMS};
                    print("Running reconfig command: " + tojsononeline(cmd));
                    const reconfigRes = primary.adminCommand(cmd);
                    const retryableReconfigCodes = [
                        ErrorCodes.NodeNotFound,
                        ErrorCodes.NewReplicaSetConfigurationIncompatible,
                        ErrorCodes.InterruptedDueToReplStateChange,
                        ErrorCodes.ConfigurationInProgress,
                        ErrorCodes.CurrentConfigNotCommittedYet,
                        ErrorCodes.NotWritablePrimary
                    ];
                    if (retryableReconfigCodes.includes(reconfigRes.code)) {
                        print("Retrying reconfig due to " + tojsononeline(reconfigRes));
                        return false;
                    }
                    assert.commandWorked(reconfigRes);
                    return true;
                }, "reconfig for fixture set up failed", ReplSetTest.kDefaultTimeoutMS, 1000);
            }
        }

        // Setup authentication if running test with authentication
        if ((jsTestOptions().keyFile || this.clusterAuthMode === "x509") &&
            cmdKey === 'replSetInitiate') {
            primary = this.getPrimary();
            // The sslSpecial suite sets up cluster with x509 but the shell was not started with TLS
            // so we need to rely on the test to auth if needed.
            if (!(this.clusterAuthMode === "x509" && !primary.isTLS())) {
                jsTest.authenticateNodes(this.nodes);
            }
        }

        // Wait for initial sync to complete on all nodes. Use a faster polling interval so we can
        // detect initial sync completion more quickly.
        this.awaitSecondaryNodes(
            this.kDefaultTimeoutMS, null /* secondaries */, 25 /* retryIntervalMS */);

        // If test commands are not enabled, we cannot wait for 'newlyAdded' removals. Tests that
        // disable test commands must ensure 'newlyAdded' removals mid-test are acceptable.
        if (!doNotWaitForNewlyAddedRemovals && jsTest.options().enableTestCommands) {
            this.waitForAllNewlyAddedRemovals();
        }

        print("ReplSetTest initiate reconfig and awaitSecondaryNodes took " +
              (new Date() - reconfigStart) + "ms for " + this.nodes.length + " nodes in set '" +
              this.name + "'");

        try {
            this.awaitHighestPriorityNodeIsPrimary();
        } catch (e) {
            // Due to SERVER-14017, the call to awaitHighestPriorityNodeIsPrimary() may fail
            // in certain configurations due to being unauthorized.  In that case we proceed
            // even though we aren't guaranteed that the highest priority node is the one that
            // became primary.
            // TODO(SERVER-14017): Unconditionally expect awaitHighestPriorityNodeIsPrimary to pass.
            assert.eq(ErrorCodes.Unauthorized, e.code, tojson(e));
            print("Running awaitHighestPriorityNodeIsPrimary() during ReplSetTest initialization " +
                  "failed with Unauthorized error, proceeding even though we aren't guaranteed " +
                  "that the highest priority node is primary");
        }

        // Set 'featureCompatibilityVersion' for the entire replica set, if specified.
        if (jsTest.options().replSetFeatureCompatibilityVersion) {
            // Authenticate before running the command.
            asCluster(this, this.nodes, () => {
                let fcv = jsTest.options().replSetFeatureCompatibilityVersion;
                print("Setting feature compatibility version for replica set to '" + fcv + "'");
                assert.commandWorked(this.getPrimary().adminCommand(
                    {setFeatureCompatibilityVersion: fcv, confirm: true}));

                // Wait for the new 'featureCompatibilityVersion' to propagate to all nodes in the
                // replica set. The 'setFeatureCompatibilityVersion' command only waits for
                // replication to a majority of nodes by default.
                this.awaitReplication();
            });
        }

        // We need to disable the enableDefaultWriteConcernUpdatesForInitiate parameter
        // to disallow updating the default write concern after initiating is complete.
        asCluster(this, this.nodes, () => {
            for (let node of this.nodes) {
                // asCluster() currently does not validate connections with X509 authentication.
                // If the test is using X509, we skip disabling the server parameter as the
                // 'setParameter' command will fail.
                // TODO(SERVER-57924): cleanup asCluster() to avoid checking here.
                if (this._notX509Auth(node) || node.isTLS()) {
                    const serverStatus =
                        assert.commandWorked(node.getDB("admin").runCommand({serverStatus: 1}));
                    const currVersion = serverStatus.version;
                    const olderThan50 = MongoRunner.compareBinVersions(
                                            MongoRunner.getBinVersionFor("5.0"),
                                            MongoRunner.getBinVersionFor(currVersion)) === 1;

                    // The following params are available only on versions greater than or equal to
                    // 5.0.
                    if (olderThan50) {
                        continue;
                    }

                    assert.commandWorked(node.adminCommand({
                        setParameter: 1,
                        enableDefaultWriteConcernUpdatesForInitiate: false,
                    }));

                    // Re-enable the reconfig check to ensure that committed writes cannot be rolled
                    // back. We disabled this check during initialization to ensure that replica
                    // sets will not fail to start up.
                    if (jsTestOptions().enableTestCommands) {
                        assert.commandWorked(node.adminCommand(
                            {setParameter: 1, enableReconfigRollbackCommittedWritesCheck: true}));
                    }
                }
            }
        });

        const awaitTsStart = new Date();  // Measure duration of awaitLastStableRecoveryTimestamp.
        if (!doNotWaitForStableRecoveryTimestamp) {
            // Speed up the polling interval so we can detect recovery timestamps more quickly.
            this.awaitLastStableRecoveryTimestamp(25 /* retryIntervalMS */);
        }
        print("ReplSetTest initiate awaitLastStableRecoveryTimestamp took " +
              (new Date() - awaitTsStart) + "ms for " + this.nodes.length + " nodes in set '" +
              this.name + "'");

        // Waits for the services which write on step-up to finish rebuilding to avoid background
        // writes after initiation is done. PrimaryOnlyServices wait for the stepup optime to be
        // majority committed before rebuilding services, so we skip waiting for PrimaryOnlyServices
        // if we do not wait for replication.
        if (!doNotWaitForReplication && !doNotWaitForPrimaryOnlyServices) {
            primary = this.getPrimary();
            // TODO(SERVER-57924): cleanup asCluster() to avoid checking here.
            if (this._notX509Auth(primary) || primary.isTLS()) {
                asCluster(this, primary, () => this.waitForStepUpWrites(primary));
            }
        }

        // Make sure all nodes are up to date. Bypass this if the heartbeat interval wasn't turned
        // down or the test specifies that we should not wait for replication. This is only an
        // optimization so it's OK if we bypass it in some suites.
        if (failPointsSupported && !doNotWaitForReplication) {
            asCluster(this, this.nodes, () => this.awaitNodesAgreeOnAppliedOpTime());
        }

        // Turn off the failpoints now that initial sync and initial setup is complete.
        if (failPointsSupported) {
            this.nodes.forEach(function(conn) {
                clearFailPoint(conn, "forceSyncSourceRetryWaitForInitialSync");
                clearFailPoint(conn, "forceHeartbeatIntervalMS");
                clearFailPoint(conn, "forceBgSyncSyncSourceRetryWaitMS");
            });
        }

        print("ReplSetTest initiateWithAnyNodeAsPrimary took " + (new Date() - startTime) +
              "ms for " + this.nodes.length + " nodes.");
    };

    /**
     * Runs replSetInitiate on the replica set and requests the first node to step up as primary.
     * This version should be prefered where possible but requires all connections in the
     * ReplSetTest to be authorized to run replSetGetStatus.
     */
    ReplSetTest.prototype.initiateWithNodeZeroAsPrimary = function(cfg, initCmd, {
        doNotWaitForPrimaryOnlyServices: doNotWaitForPrimaryOnlyServices = false,
    } = {}) {
        let startTime = new Date();  // Measure the execution time of this function.
        this.initiateWithAnyNodeAsPrimary(cfg, initCmd, {doNotWaitForPrimaryOnlyServices: true});

        // Most of the time node 0 will already be primary so we can skip the step-up.
        let primary = this.getPrimary();
        if (this.getNodeId(this.nodes[0]) == this.getNodeId(primary)) {
            print("ReplSetTest initiateWithNodeZeroAsPrimary skipping step-up because node 0 is " +
                  "already primary");
            asCluster(this, primary, () => {
                if (!doNotWaitForPrimaryOnlyServices) {
                    this.waitForStepUpWrites(primary);
                }
            });
        } else {
            // stepUp() calls awaitReplication() which requires all nodes to be authorized to run
            // replSetGetStatus.
            asCluster(this, this.nodes, () => {
                const newPrimary = this.nodes[0];
                this.stepUp(newPrimary,
                            {doNotWaitForPrimaryOnlyServices: doNotWaitForPrimaryOnlyServices});
                if (!doNotWaitForPrimaryOnlyServices) {
                    this.waitForStepUpWrites(newPrimary);
                }
            });
        }

        print("ReplSetTest initiateWithNodeZeroAsPrimary took " + (new Date() - startTime) +
              "ms for " + this.nodes.length + " nodes.");
    };

    /**
     * Runs replSetInitiate on the replica set and requests the first node to step up as
     * primary.
     */
    ReplSetTest.prototype.initiate = function(cfg, initCmd, {
        doNotWaitForPrimaryOnlyServices: doNotWaitForPrimaryOnlyServices = false,
    } = {}) {
        this.initiateWithNodeZeroAsPrimary(
            cfg, initCmd, {doNotWaitForPrimaryOnlyServices: doNotWaitForPrimaryOnlyServices});
    };

    /**
     * Modifies the election timeout to be 24 hours so that no unplanned elections happen. Then
     * runs replSetInitiate on the replica set with the new config.
     */
    ReplSetTest.prototype.initiateWithHighElectionTimeout = function(config) {
        config = config || this.getReplSetConfig();
        config.settings = config.settings || {};
        config.settings["electionTimeoutMillis"] = ReplSetTest.kForeverMillis;
        this.initiate(config);
    };

    /**
     * Steps up 'node' as primary and by default it waits for the stepped up node to become a
     * writable primary and waits for all nodes to reach the same optime before sending the
     * replSetStepUp command to 'node'.
     *
     * Calls awaitReplication() which requires all connections in 'nodes' to be authenticated.
     * This stepUp() assumes that there is no network partition in the replica set.
     */
    ReplSetTest.prototype.stepUp = function(node, {
        awaitReplicationBeforeStepUp: awaitReplicationBeforeStepUp = true,
        awaitWritablePrimary: awaitWritablePrimary = true,
        doNotWaitForPrimaryOnlyServices = false,
    } = {}) {
        jsTest.log("ReplSetTest stepUp: Stepping up " + node.host);

        if (awaitReplicationBeforeStepUp) {
            if (!doNotWaitForPrimaryOnlyServices) {
                this.waitForStepUpWrites();
            }
            this.awaitReplication();
        }

        assert.soonNoExcept(() => {
            const res = node.adminCommand({replSetStepUp: 1});
            // This error is possible if we are running mongoDB binary < 3.4 as
            // part of multi-version upgrade test. So, for those older branches,
            // simply wait for the requested node to get elected as primary due
            // to election timeout.
            if (!res.ok && res.code === ErrorCodes.CommandNotFound) {
                jsTest.log(
                    'replSetStepUp command not supported on node ' + node.host +
                    " ; so wait for the requested node to get elected due to election timeout.");
                if (this.getPrimary() === node) {
                    return true;
                }
            }
            assert.commandWorked(res);

            // Since assert.soon() timeout is 10 minutes (default), setting
            // awaitNodesAgreeOnPrimary() timeout as 1 minute to allow retry of replSetStepUp
            // command on failure of the replica set to agree on the primary.
            // We should not run hangAnalyzer when awaitNodesAgreeOnPrimary() timeout, otherwise the
            // mongo processes will be killed and we cannot retry.
            const timeout = 60 * 1000;
            this.awaitNodesAgreeOnPrimary(
                timeout, this.nodes, node, false /*runHangAnalyzerOnTimeout*/);

            if (!awaitWritablePrimary) {
                return true;
            }

            // getPrimary() guarantees that there will be only one writable primary for a replica
            // set.
            const newPrimary = this.getPrimary();
            if (newPrimary.host === node.host) {
                return true;
            }

            jsTest.log(node.host + ' is not primary after stepUp command, ' + newPrimary.host +
                       ' is the primary');
            return false;
        }, "Timed out while waiting for stepUp to succeed on node in port: " + node.port);

        jsTest.log("ReplSetTest stepUp: Finished stepping up " + node.host);
        return node;
    };

    /**
     * Wait for writes which may happen when nodes are stepped up.  This currently includes
     * primary-only service writes and writes from the query analysis writer, the latter being
     * a replica-set-aware service for which there is no generic way to wait.
     */
    ReplSetTest.prototype.waitForStepUpWrites = function(primary) {
        primary = primary || this.getPrimary();
        this.waitForPrimaryOnlyServices(primary);
        this.waitForQueryAnalysisWriterSetup(primary);
    };

    /**
     * Waits for primary only services to finish the rebuilding stage after a primary is elected.
     * This is useful for tests that are expecting particular write timestamps since some primary
     * only services can do background writes (e.g. build indexes) during rebuilding stage that
     * could advance the last write timestamp.
     */
    ReplSetTest.prototype.waitForPrimaryOnlyServices = function(primary) {
        jsTest.log("Waiting for primary only services to finish rebuilding");
        primary = primary || this.getPrimary();

        assert.soonNoExcept(function() {
            const res = assert.commandWorked(primary.adminCommand({serverStatus: 1, repl: 1}));
            // 'PrimaryOnlyServices' does not exist prior to v5.0, using empty
            // object to skip waiting in case of multiversion tests.
            const services = res.repl.primaryOnlyServices || {};
            return Object.keys(services).every((s) => {
                return services[s].state === undefined || services[s].state === "running";
            });
        }, "Timed out waiting for primary only services to finish rebuilding");
    };

    /**
     * If query sampling is supported, waits for the query analysis writer to finish setting up
     * after a primary is elected. This is useful for tests that expect particular write timestamps
     * since the query analysis writer setup involves building indexes for the config.sampledQueries
     * and config.sampledQueriesDiff collections.
     */
    ReplSetTest.prototype.waitForQueryAnalysisWriterSetup = function(primary) {
        primary = primary || this.getPrimary();

        const serverStatusRes = assert.commandWorked(primary.adminCommand({serverStatus: 1}));
        if (!serverStatusRes.hasOwnProperty("queryAnalyzers")) {
            // Query sampling is not supported on this replica set. That is, either it uses binaries
            // released before query sampling was introduced or it uses binaries where query
            // sampling is guarded by a feature flag and the feature flag is not enabled.
            return;
        }

        const getParamsRes = primary.adminCommand({getParameter: 1, multitenancySupport: 1});
        if (!getParamsRes.ok || getParamsRes["multitenancySupport"]) {
            // Query sampling is not supported on a multi-tenant replica set.
            return;
        }

        jsTest.log("Waiting for query analysis writer to finish setting up");

        assert.soonNoExcept(function() {
            const sampledQueriesIndexes =
                primary.getCollection("config.sampledQueries").getIndexes();
            const sampledQueriesDiffIndexes =
                primary.getCollection("config.sampledQueriesDiff").getIndexes();
            // There should be two indexes: _id index and TTL index.
            return sampledQueriesIndexes.length == 2 && sampledQueriesDiffIndexes.length == 2;
        }, "Timed out waiting for query analysis writer to finish setting up");
    };

    /**
     * Gets the current replica set config from the specified node index. If no nodeId is specified,
     * uses the primary node.
     */
    ReplSetTest.prototype.getReplSetConfigFromNode = function(nodeId) {
        if (nodeId == undefined) {
            // Use 90 seconds timeout for finding a primary
            return _replSetGetConfig(this.getPrimary(90 * 1000));
        }

        if (!isNumber(nodeId)) {
            throw Error(nodeId + ' is not a number');
        }

        return _replSetGetConfig(this.nodes[nodeId]);
    };

    ReplSetTest.prototype.reInitiate = function() {
        var config = this.getReplSetConfigFromNode();
        var newConfig = this.getReplSetConfig();
        // Only reset members.
        config.members = newConfig.members;
        config.version += 1;

        this._setDefaultConfigOptions(config);

        // Set a maxTimeMS so reconfig fails if it times out.
        assert.adminCommandWorkedAllowingNetworkError(
            this.getPrimary(), {replSetReconfig: config, maxTimeMS: ReplSetTest.kDefaultTimeoutMS});
    };

    /**
     * Blocks until all nodes in the replica set have the same config version as the primary.
     **/
    ReplSetTest.prototype.awaitNodesAgreeOnConfigVersion = function(timeout) {
        timeout = timeout || this.kDefaultTimeoutMS;

        assert.soonNoExcept(() => {
            var primaryVersion = this.getPrimary().getDB('admin')._helloOrLegacyHello().setVersion;

            for (var i = 0; i < this.nodes.length; i++) {
                var version = this.nodes[i].getDB('admin')._helloOrLegacyHello().setVersion;
                assert.eq(version,
                          primaryVersion,
                          "waiting for secondary node " + this.nodes[i].host +
                              " with config version of " + version +
                              " to match the version of the primary " + primaryVersion);
            }

            return true;
        }, "Awaiting nodes to agree on config version", timeout);
    };

    /**
     * Waits for the last oplog entry on the primary to be visible in the committed snapshot view
     * of the oplog on *all* secondaries. When majority read concern is disabled, there is no
     * committed snapshot view, so this function waits for the knowledge of the majority commit
     * point on each node to advance to the optime of the last oplog entry on the primary.
     * Returns last oplog entry.
     */
    ReplSetTest.prototype.awaitLastOpCommitted = function(timeout, members) {
        var rst = this;
        var primary = rst.getPrimary();
        var primaryOpTime = _getLastOpTime(this, primary);

        let membersToCheck;
        if (members !== undefined) {
            print("Waiting for op with OpTime " + tojson(primaryOpTime) + " to be committed on " +
                  members.map(s => s.host));

            membersToCheck = members;
        } else {
            print("Waiting for op with OpTime " + tojson(primaryOpTime) +
                  " to be committed on all secondaries");

            membersToCheck = rst.nodes;
        }

        assert.soonNoExcept(
            function() {
                for (var i = 0; i < membersToCheck.length; i++) {
                    var node = membersToCheck[i];

                    // Continue if we're connected to an arbiter
                    var res = assert.commandWorked(node.adminCommand({replSetGetStatus: 1}));
                    if (res.myState == ReplSetTest.State.ARBITER) {
                        continue;
                    }
                    var rcmOpTime = _getReadConcernMajorityOpTime(node);
                    if (friendlyEqual(rcmOpTime, {ts: Timestamp(0, 0), t: NumberLong(0)})) {
                        return false;
                    }
                    if (globalThis.rs.compareOpTimes(rcmOpTime, primaryOpTime) < 0) {
                        return false;
                    }
                }

                return true;
            },
            "Op with OpTime " + tojson(primaryOpTime) +
                " failed to be committed on all secondaries",
            timeout);

        print("Op with OpTime " + tojson(primaryOpTime) +
              " successfully committed on all secondaries");
        return primaryOpTime;
    };

    // TODO(SERVER-14017): Remove this extra sub-shell in favor of a cleaner authentication
    // solution.
    function runFnWithAuthOnPrimary(rst, fn, fnName) {
        const primary = rst.getPrimary();
        const primaryId = "n" + rst.getNodeId(primary);
        const primaryOptions = rst.nodeOptions[primaryId] || {};
        const options =
            (primaryOptions === {} || !rst.startOptions) ? primaryOptions : rst.startOptions;
        const authMode = options.clusterAuthMode;
        if (authMode === "x509") {
            print(fnName + ": authenticating on separate shell with x509 for " + rst.name);
            const caFile = options.sslCAFile ? options.sslCAFile : options.tlsCAFile;
            const keyFile =
                options.sslPEMKeyFile ? options.sslPEMKeyFile : options.tlsCertificateKeyFile;
            const subShellArgs = [
                'mongo',
                '--ssl',
                '--sslCAFile=' + caFile,
                '--sslPEMKeyFile=' + keyFile,
                '--sslAllowInvalidHostnames',
                '--authenticationDatabase=$external',
                '--authenticationMechanism=MONGODB-X509',
                primary.host,
                '--eval',
                `(${fn.toString()})();`
            ];

            const retVal = _runMongoProgram(...subShellArgs);
            assert.eq(retVal, 0, 'mongo shell did not succeed with exit code 0');
        } else {
            print(fnName + ": authenticating with authMode '" + authMode + "' for " + rst.name);
            asCluster(rst, primary, fn, primaryOptions.keyFile);
        }
    }

    /**
     * This function performs some writes and then waits for all nodes in this replica set to
     * establish a stable recovery timestamp. The writes are necessary to prompt storage engines to
     * quickly establish stable recovery timestamps.
     *
     * A stable recovery timestamp ensures recoverable rollback is possible, as well as startup
     * recovery without re-initial syncing in the case of durable storage engines. By waiting for
     * all nodes to report having a stable recovery timestamp, we ensure a degree of stability in
     * our tests to run as expected.
     */
    ReplSetTest.prototype.awaitLastStableRecoveryTimestamp = function(retryIntervalMS) {
        let rst = this;
        let primary = rst.getPrimary();
        let id = tojson(rst.nodeList());
        retryIntervalMS = retryIntervalMS || 200;

        // All nodes must be in primary/secondary state prior to this point. Perform a majority
        // write to ensure there is a committed operation on the set. The commit point will
        // propagate to all members and trigger a stable checkpoint on all persisted storage engines
        // nodes.
        function advanceCommitPoint(rst, primary) {
            // Shadow 'db' so that we can call the function on the primary without a separate shell
            // when x509 auth is not needed.
            let db = primary.getDB('admin');
            const appendOplogNoteFn = function() {
                assert.commandWorked(db.adminCommand({
                    "appendOplogNote": 1,
                    "data": {"awaitLastStableRecoveryTimestamp": 1},
                    "writeConcern": {"w": "majority", "wtimeout": ReplSetTest.kDefaultTimeoutMS}
                }));
            };

            runFnWithAuthOnPrimary(rst, appendOplogNoteFn, "AwaitLastStableRecoveryTimestamp");
        }

        print("AwaitLastStableRecoveryTimestamp: Beginning for " + id);

        let replSetStatus = assert.commandWorked(primary.adminCommand("replSetGetStatus"));
        if (replSetStatus["configsvr"]) {
            // Performing dummy replicated writes against a configsvr is hard, especially if auth
            // is also enabled.
            return;
        }

        rst.awaitNodesAgreeOnPrimary();
        primary = rst.getPrimary();

        print("AwaitLastStableRecoveryTimestamp: ensuring the commit point advances for " + id);
        advanceCommitPoint(this, primary);

        print("AwaitLastStableRecoveryTimestamp: Waiting for stable recovery timestamps for " + id);

        assert.soonNoExcept(
            function() {
                for (let node of rst.nodes) {
                    // The `lastStableRecoveryTimestamp` field contains a stable timestamp
                    // guaranteed to exist on storage engine recovery to stable timestamp.
                    let res = assert.commandWorked(node.adminCommand({replSetGetStatus: 1}));

                    // Continue if we're connected to an arbiter.
                    if (res.myState === ReplSetTest.State.ARBITER) {
                        continue;
                    }

                    // A missing `lastStableRecoveryTimestamp` field indicates that the storage
                    // engine does not support `recover to a stable timestamp`.
                    //
                    // A null `lastStableRecoveryTimestamp` indicates that the storage engine
                    // supports "recover to a stable timestamp", but does not have a stable recovery
                    // timestamp yet.
                    if (res.hasOwnProperty("lastStableRecoveryTimestamp") &&
                        res.lastStableRecoveryTimestamp.getTime() === 0) {
                        print("AwaitLastStableRecoveryTimestamp: " + node.host +
                              " does not have a stable recovery timestamp yet.");
                        return false;
                    }
                }

                return true;
            },
            "Not all members have a stable recovery timestamp",
            ReplSetTest.kDefaultTimeoutMS,
            retryIntervalMS);

        print("AwaitLastStableRecoveryTimestamp: A stable recovery timestamp has successfully " +
              "established on " + id);
    };

    // Wait until the optime of the specified type reaches the primary or the targetNode's last
    // applied optime if provided. Blocks on all secondary nodes or just 'secondaries', if
    // specified. The timeout will reset if any of the secondaries makes progress.
    ReplSetTest.prototype.awaitReplication = function(
        timeout, secondaryOpTimeType, secondaries, retryIntervalMS, targetNode) {
        if (secondaries !== undefined && secondaries !== this._secondaries) {
            print("ReplSetTest awaitReplication: going to check only " +
                  secondaries.map(s => s.host));
        }

        if (targetNode !== undefined) {
            print(`ReplSetTest awaitReplication: wait against targetNode ${
                targetNode.host} instead of primary.`);
        }

        timeout = timeout || this.kDefaultTimeoutMS;
        retryIntervalMS = retryIntervalMS || 200;

        secondaryOpTimeType = secondaryOpTimeType || ReplSetTest.OpTimeType.LAST_APPLIED;

        var targetLatestOpTime;

        // Blocking call, which will wait for the last optime written on the target to be available
        var awaitLastOpTimeWrittenFn = function(rst) {
            var target = targetNode || rst.getPrimary();
            assert.soonNoExcept(function() {
                try {
                    targetLatestOpTime = _getLastOpTime(rst, target);
                } catch (e) {
                    print("ReplSetTest caught exception " + e);
                    return false;
                }

                return true;
            }, "awaiting oplog query", timeout);
        };
        awaitLastOpTimeWrittenFn(this);

        // get the latest config version from target (with a few retries in case of error)
        var targetConfigVersion;
        var targetName;
        var target;
        var num_attempts = 3;

        assert.retryNoExcept(() => {
            target = targetNode || this.getPrimary();
            targetConfigVersion =
                asCluster(this, target, () => this.getReplSetConfigFromNode(this.getNodeId(target)))
                    .version;
            targetName = target.host;
            return true;
        }, "ReplSetTest awaitReplication: couldnt get repl set config.", num_attempts, 1000);

        print("ReplSetTest awaitReplication: starting: optime for target, " + targetName + ", is " +
              tojson(targetLatestOpTime));

        let nodesCaughtUp = false;
        let secondariesToCheck = secondaries || this._secondaries;
        let nodeProgress = Array(secondariesToCheck.length);

        const Progress = Object.freeze({
            Skip: 'Skip',
            CaughtUp: 'CaughtUp',
            InProgress: 'InProgress',
            Stuck: 'Stuck',
            ConfigMismatch: 'ConfigMismatch'
        });

        function checkProgressSingleNode(rst, index, secondaryCount) {
            var secondary = secondariesToCheck[index];
            var secondaryName = secondary.host;

            var secondaryConfigVersion = asCluster(rst,
                                                   secondary,
                                                   () => secondary.getDB("local")['system.replset']
                                                             .find()
                                                             .readConcern("local")
                                                             .limit(1)
                                                             .next()
                                                             .version);

            if (targetConfigVersion != secondaryConfigVersion) {
                print("ReplSetTest awaitReplication: secondary #" + secondaryCount + ", " +
                      secondaryName + ", has config version #" + secondaryConfigVersion +
                      ", but expected config version #" + targetConfigVersion);

                if (secondaryConfigVersion > targetConfigVersion) {
                    target = targetNode || rst.getPrimary();
                    targetConfigVersion = target.getDB("local")['system.replset']
                                              .find()
                                              .readConcern("local")
                                              .limit(1)
                                              .next()
                                              .version;
                    targetName = target.host;

                    print("ReplSetTest awaitReplication: optime for target, " + targetName +
                          ", is " + tojson(targetLatestOpTime));
                }

                return Progress.ConfigMismatch;
            }

            // Skip this node if we're connected to an arbiter
            var res = asCluster(
                rst,
                secondary,
                () => assert.commandWorked(secondary.adminCommand({replSetGetStatus: 1})));
            if (res.myState == ReplSetTest.State.ARBITER) {
                return Progress.Skip;
            }

            print("ReplSetTest awaitReplication: checking secondary #" + secondaryCount + ": " +
                  secondaryName);

            secondary.getDB("admin").getMongo().setSecondaryOk();

            var secondaryOpTime;
            if (secondaryOpTimeType == ReplSetTest.OpTimeType.LAST_DURABLE) {
                secondaryOpTime = _getDurableOpTime(rst, secondary);
            } else {
                secondaryOpTime = _getLastOpTime(rst, secondary);
            }

            // If the node doesn't have a valid opTime, it likely hasn't received any writes from
            // the primary yet.
            if (!globalThis.rs.isValidOpTime(secondaryOpTime)) {
                print("ReplSetTest awaitReplication: optime for secondary #" + secondaryCount +
                      ", " + secondaryName + ", is " + tojson(secondaryOpTime) +
                      ", which is NOT valid.");
                return Progress.Stuck;
            }

            // See if the node made progress. We count it as progress even if the node's last optime
            // went backwards because that means the node is in rollback.
            let madeProgress =
                (nodeProgress[index] &&
                 (globalThis.rs.compareOpTimes(nodeProgress[index], secondaryOpTime) != 0));
            nodeProgress[index] = secondaryOpTime;

            if (globalThis.rs.compareOpTimes(targetLatestOpTime, secondaryOpTime) < 0) {
                targetLatestOpTime = _getLastOpTime(rst, target);
                print("ReplSetTest awaitReplication: optime for " + secondaryName +
                      " is newer, resetting latest target optime to " + tojson(targetLatestOpTime) +
                      ". Also resetting awaitReplication timeout");
                return Progress.InProgress;
            }

            if (!friendlyEqual(targetLatestOpTime, secondaryOpTime)) {
                print("ReplSetTest awaitReplication: optime for secondary #" + secondaryCount +
                      ", " + secondaryName + ", is " + tojson(secondaryOpTime) + " but latest is " +
                      tojson(targetLatestOpTime));
                print("ReplSetTest awaitReplication: secondary #" + secondaryCount + ", " +
                      secondaryName + ", is NOT synced");

                // Reset the timeout if a node makes progress, but isn't caught up yet.
                if (madeProgress) {
                    print("ReplSetTest awaitReplication: secondary #" + secondaryCount + ", " +
                          secondaryName +
                          ", has made progress. Resetting awaitReplication timeout");
                    return Progress.InProgress;
                }
                return Progress.Stuck;
            }

            print("ReplSetTest awaitReplication: secondary #" + secondaryCount + ", " +
                  secondaryName + ", is synced");
            return Progress.CaughtUp;
        }

        // We will reset the timeout if a nodes makes progress, but still isn't caught up yet.
        while (!nodesCaughtUp) {
            assert.soonNoExcept(() => {
                try {
                    print("ReplSetTest awaitReplication: checking secondaries against latest " +
                          "target optime " + tojson(targetLatestOpTime));
                    var secondaryCount = 0;

                    for (var i = 0; i < secondariesToCheck.length; i++) {
                        const action = checkProgressSingleNode(this, i, secondaryCount);

                        switch (action) {
                            case Progress.CaughtUp:
                                // We only need to increment the secondaryCount if this node is
                                // caught up.
                                secondaryCount++;
                                continue;
                            case Progress.Skip:
                                // Don't increment secondaryCount because this node is an arbiter.
                                continue;
                            case Progress.InProgress:
                                return true;
                            case Progress.Stuck:
                            case Progress.ConfigMismatch:
                                return false;
                            default:
                                throw Error("invalid action: " + tojson(action));
                        }
                    }

                    print("ReplSetTest awaitReplication: finished: all " + secondaryCount +
                          " secondaries synced at optime " + tojson(targetLatestOpTime));
                    nodesCaughtUp = true;
                    return true;
                } catch (e) {
                    print("ReplSetTest awaitReplication: caught exception " + e);

                    // We might have a new primary now
                    awaitLastOpTimeWrittenFn();

                    print("ReplSetTest awaitReplication: resetting: optime for target " + target +
                          " is " + tojson(targetLatestOpTime));

                    return false;
                }
            }, "awaitReplication timed out", timeout, retryIntervalMS);
        }
    };

    ReplSetTest.prototype.getHashesUsingSessions = function(
        sessions, dbName, {readAtClusterTime,
                           skipTempCollections = false} = {skipTempCollections: false}) {
        return sessions.map(session => {
            const commandObj = {dbHash: 1};
            const db = session.getDatabase(dbName);
            // If eMRC=false, we use the old behavior using $_internalReadAtClusterTime.
            // Otherwise, we use snapshot read concern for dbhash.
            if (readAtClusterTime !== undefined) {
                if (jsTest.options().enableMajorityReadConcern !== false) {
                    commandObj.readConcern = {level: "snapshot", atClusterTime: readAtClusterTime};
                } else {
                    commandObj.$_internalReadAtClusterTime = readAtClusterTime;
                }
            }
            if (skipTempCollections) {
                commandObj.skipTempCollections = 1;
            }

            return assert.commandWorked(db.runCommand(commandObj));
        });
    };

    // Gets the dbhash for the current primary and for all secondaries (or the members of
    // 'secondaries', if specified).
    ReplSetTest.prototype.getHashes = function(dbName, secondaries, skipTempCollections) {
        assert.neq(dbName, 'local', 'Cannot run getHashes() on the "local" database');

        // _determineLiveSecondaries() repopulates both 'self._secondaries' and 'self._primary'. If
        // we're passed an explicit set of secondaries we don't want to do that.
        secondaries = secondaries || _determineLiveSecondaries(this);

        const sessions = [
            this._primary,
            ...secondaries.filter(conn => {
                return !conn.getDB('admin')._helloOrLegacyHello().arbiterOnly;
            })
        ].map(conn => conn.getDB('test').getSession());

        const hashes = this.getHashesUsingSessions(sessions, dbName, {skipTempCollections});
        return {primary: hashes[0], secondaries: hashes.slice(1)};
    };

    ReplSetTest.prototype.findOplog = function(conn, query, limit) {
        return conn.getDB('local')
            .getCollection(kOplogName)
            .find(query)
            .sort({$natural: -1})
            .limit(limit);
    };

    ReplSetTest.prototype.dumpOplog = function(conn, query = {}, limit = 10) {
        var log = 'Dumping the latest ' + limit + ' documents that match ' + tojson(query) +
            ' from the oplog ' + kOplogName + ' of ' + conn.host;
        let entries = [];
        let cursor = this.findOplog(conn, query, limit);
        cursor.forEach(function(entry) {
            log = log + '\n' + tojsononeline(entry);
            entries.push(entry);
        });
        jsTestLog(log);
        return entries;
    };

    // Call the provided checkerFunction, after the replica set has been write locked.
    ReplSetTest.prototype.checkReplicaSet = function(
        checkerFunction, secondaries, ...checkerFunctionArgs) {
        assert.eq(typeof checkerFunction,
                  "function",
                  "Expected checkerFunction parameter to be a function");

        assert(secondaries, 'must pass list of live nodes to checkReplicaSet');

        // Call getPrimary to populate rst with information about the nodes.
        var primary = this.getPrimary();
        assert(primary, 'calling getPrimary() failed');

        // Prevent an election, which could start, then hang due to the fsyncLock.
        jsTestLog(`Freezing nodes: [${secondaries.map((n) => n.host)}]`);
        this.freeze(secondaries);

        // Await primary in case freeze() had to step down a node that was unexpectedly primary.
        this.getPrimary();

        // Lock the primary to prevent writes in the background while we are getting the
        // dbhashes of the replica set members. It's not important if the storage engine fails
        // to perform its fsync operation. The only requirement is that writes are locked out.
        assert.commandWorked(primary.adminCommand({fsync: 1, lock: 1, allowFsyncFailure: true}),
                             'failed to lock the primary');

        function postApplyCheckerFunction() {
            // Unfreeze secondaries and unlock primary.
            try {
                assert.commandWorked(primary.adminCommand({fsyncUnlock: 1}));
            } catch (e) {
                print(`Continuing after fsyncUnlock error: ${e}`);
            }

            secondaries.forEach(secondary => {
                try {
                    assert.commandWorked(secondary.adminCommand({replSetFreeze: 0}));
                } catch (e) {
                    print(`Continuing after replSetFreeze error: ${e}`);
                }
            });
        }

        let activeException = false;
        try {
            this.awaitReplication(null, null, secondaries);
            checkerFunction.apply(this, checkerFunctionArgs);
        } catch (e) {
            activeException = true;
            throw e;
        } finally {
            if (activeException) {
                try {
                    postApplyCheckerFunction();
                } catch (e) {
                    // Print the postApplyCheckerFunction error, propagate the original.
                    print(e);
                }
            } else {
                postApplyCheckerFunction();
            }
        }
    };

    // Check the replicated data hashes for all live nodes in the set.
    ReplSetTest.prototype.checkReplicatedDataHashes = function(
        msgPrefix = 'checkReplicatedDataHashes', excludedDBs = [], ignoreUUIDs = false) {
        // Return items that are in either Array `a` or `b` but not both. Note that this will
        // not work with arrays containing NaN. Array.indexOf(NaN) will always return -1.

        var collectionPrinted = new Set();

        function checkDBHashesForReplSet(
            rst, dbDenylist = [], secondaries, msgPrefix, ignoreUUIDs) {
            // We don't expect the local database to match because some of its
            // collections are not replicated.
            dbDenylist.push('local');
            secondaries = secondaries || rst._secondaries;

            let success = true;
            let hasDumpedOplog = false;

            // Use '_primary' instead of getPrimary() to avoid the detection of a new primary.
            // '_primary' must have been populated.
            const primary = rst._primary;

            let combinedDBs = new Map();
            primary.getDBs().databases.map(db => {
                const key = `${db.tenantId}_${db.name}`;
                const obj = {"name": db.name, "tenant": db.tenantId};
                combinedDBs.set(key, obj);
            });

            const replSetConfig = rst.getReplSetConfigFromNode();

            print("checkDBHashesForReplSet waiting for secondaries to be ready: " +
                  tojson(secondaries));
            this.awaitSecondaryNodes(rst.kDefaultTimeoutMS, secondaries);

            print("checkDBHashesForReplSet checking data hashes against primary: " + primary.host);

            secondaries.forEach(node => {
                // Arbiters have no replicated data.
                if (isNodeArbiter(node)) {
                    print("checkDBHashesForReplSet skipping data of arbiter: " + node.host);
                    return;
                }
                print("checkDBHashesForReplSet going to check data hashes on secondary: " +
                      node.host);
                node.getDBs().databases.forEach(db => {
                    const key = `${db.tenantId}_${db.name}`;
                    const obj = {"name": db.name, "tenant": db.tenantId};
                    combinedDBs.set(key, obj);
                });
            });

            const expectPrefix =
                (typeof TestData !== "undefined" && TestData.multitenancyExpectPrefix) ? true
                                                                                       : false;

            for (const [key, db] of combinedDBs) {
                const tenant = db.tenant;
                const dbName = expectPrefix && tenant ? tenant + '_' + db.name : db.name;

                if (Array.contains(dbDenylist, dbName)) {
                    continue;
                }

                const token = db.tenant ? _createTenantToken({tenant, expectPrefix}) : undefined;
                try {
                    primary._setSecurityToken(token);
                    secondaries.forEach(node => node._setSecurityToken(token));

                    const dbHashes = rst.getHashes(dbName, secondaries);
                    const primaryDBHash = dbHashes.primary;
                    const primaryCollections = Object.keys(primaryDBHash.collections);
                    assert.commandWorked(primaryDBHash);

                    // Filter only collections that were retrieved by the dbhash.
                    // listCollections may include non-replicated collections like
                    // system.profile.
                    const primaryCollInfos = new CollInfos(primary, 'primary', dbName);
                    primaryCollInfos.filter(primaryCollections);

                    dbHashes.secondaries.forEach(secondaryDBHash => {
                        assert.commandWorked(secondaryDBHash);

                        const secondary = secondaryDBHash._mongo;
                        const secondaryCollections = Object.keys(secondaryDBHash.collections);
                        // Check that collection information is consistent on the primary and
                        // secondaries.
                        const secondaryCollInfos = new CollInfos(secondary, 'secondary', dbName);
                        secondaryCollInfos.filter(secondaryCollections);

                        const hasSecondaryIndexes =
                            replSetConfig.members[rst.getNodeId(secondary)].buildIndexes !== false;

                        print(`checking db hash between primary: ${primary.host}, and secondary: ${
                            secondary.host}`);
                        success = DataConsistencyChecker.checkDBHash(primaryDBHash,
                                                                     primaryCollInfos,
                                                                     secondaryDBHash,
                                                                     secondaryCollInfos,
                                                                     msgPrefix,
                                                                     ignoreUUIDs,
                                                                     hasSecondaryIndexes,
                                                                     collectionPrinted) &&
                            success;

                        if (!success) {
                            if (!hasDumpedOplog) {
                                print("checkDBHashesForReplSet dumping oplogs from all nodes");
                                this.dumpOplog(primary, {}, 100);
                                rst.getSecondaries().forEach(
                                    secondary => this.dumpOplog(secondary, {}, 100));
                                hasDumpedOplog = true;
                            }
                        }
                    });
                } finally {
                    primary._setSecurityToken(undefined);
                    secondaries.forEach(node => node._setSecurityToken(undefined));
                }
            }

            assert(success, 'dbhash mismatch between primary and secondary');
        }

        const liveSecondaries = _determineLiveSecondaries(this);
        this.checkReplicaSet(checkDBHashesForReplSet,
                             liveSecondaries,
                             this,
                             excludedDBs,
                             liveSecondaries,
                             msgPrefix,
                             ignoreUUIDs);
    };

    ReplSetTest.prototype.checkOplogs = function(msgPrefix) {
        var liveSecondaries = _determineLiveSecondaries(this);
        this.checkReplicaSet(checkOplogs, liveSecondaries, this, liveSecondaries, msgPrefix);
    };

    const ReverseReader = function(mongo, coll, query) {
        this.kCappedPositionLostSentinel = Object.create(null);

        this._safelyPerformCursorOperation = function(name, operation, onCappedPositionLost) {
            if (!this.cursor) {
                throw new Error("ReverseReader is not open!");
            }

            if (this._cursorExhausted) {
                return onCappedPositionLost;
            }

            try {
                return operation(this.cursor);
            } catch (err) {
                print("Error: " + name + " threw '" + err.message + "' on " + this.mongo.host);
                // Occasionally, the capped collection will get truncated while we are iterating
                // over it. Since we are iterating over the collection in reverse, getting a
                // truncated item means we've reached the end of the list, so return false.
                if (err.code === ErrorCodes.CappedPositionLost) {
                    this.cursor.close();
                    this._cursorExhausted = true;
                    return onCappedPositionLost;
                }

                throw err;
            }
        };

        this.next = function() {
            return this._safelyPerformCursorOperation('next', function(cursor) {
                return cursor.next();
            }, this.kCappedPositionLostSentinel);
        };

        this.hasNext = function() {
            return this._safelyPerformCursorOperation('hasNext', function(cursor) {
                return cursor.hasNext();
            }, false);
        };

        this.query = function() {
            // Set the cursor to read backwards, from last to first. We also set the cursor not
            // to time out since it may take a while to process each batch and a test may have
            // changed "cursorTimeoutMillis" to a short time period.
            // TODO SERVER-75496 remove the batchSize once the the following issue is fixed: The
            // find{...} will always run with apiStrict:false, however getMore may run with
            // apiStrict: true on specific suites. Use a big batch size to prevent getMore from
            // running.
            this._cursorExhausted = false;
            this.cursor = coll.find(query)
                              .sort({$natural: -1})
                              .noCursorTimeout()
                              .readConcern("local")
                              .batchSize(200);
        };

        this.getFirstDoc = function() {
            return coll.find(query).sort({$natural: 1}).readConcern("local").limit(-1).next();
        };

        this.cursor = null;
        this._cursorExhausted = true;
        this.mongo = mongo;
    };

    /**
     * Check oplogs on all nodes, by reading from the last time. Since the oplog is a capped
     * collection, each node may not contain the same number of entries and stop if the cursor
     * is exhausted on any node being checked.
     */
    function checkOplogs(rst, secondaries, msgPrefix = 'checkOplogs') {
        secondaries = secondaries || rst._secondaries;

        function assertOplogEntriesEq(oplogEntry0, oplogEntry1, reader0, reader1, prevOplogEntry) {
            if (!bsonBinaryEqual(oplogEntry0, oplogEntry1)) {
                const query = prevOplogEntry ? {ts: {$lte: prevOplogEntry.ts}} : {};
                rst.nodes.forEach(node => rst.dumpOplog(node, query, 100));
                const log = msgPrefix + ", non-matching oplog entries for the following nodes: \n" +
                    reader0.mongo.host + ": " + tojsononeline(oplogEntry0) + "\n" +
                    reader1.mongo.host + ": " + tojsononeline(oplogEntry1);
                assert(false, log);
            }
        }

        print("checkOplogs starting oplog checks.");
        print("checkOplogs waiting for secondaries to be ready.");
        rst.awaitSecondaryNodes(rst.kDefaultTimeoutMS, secondaries);
        if (secondaries.length >= 1) {
            let readers = [];
            let smallestTS = new Timestamp(Math.pow(2, 32) - 1, Math.pow(2, 32) - 1);
            const nodes = rst.nodes;
            let firstReaderIndex;
            for (let i = 0; i < nodes.length; i++) {
                const node = nodes[i];

                if (rst._primary !== node && !secondaries.includes(node)) {
                    print("checkOplogs skipping oplog of node: " + node.host);
                    continue;
                }

                // Arbiters have no documents in the oplog.
                if (isNodeArbiter(node)) {
                    jsTestLog("checkOplogs skipping oplog of arbiter: " + node.host);
                    continue;
                }

                print("checkOplogs going to check oplog of node: " + node.host);
                readers[i] = new ReverseReader(
                    node, node.getDB("local")[kOplogName], {ts: {$gte: new Timestamp()}});
                const currTS = readers[i].getFirstDoc().ts;
                // Find the reader which has the smallestTS. This reader should have the most
                // number of documents in the oplog.
                if (timestampCmp(currTS, smallestTS) < 0) {
                    smallestTS = currTS;
                    firstReaderIndex = i;
                }
                // Start all oplogReaders at their last document.
                readers[i].query();
            }

            // Read from the reader which has the most oplog entries.
            // Note, we read the oplog backwards from last to first.
            const firstReader = readers[firstReaderIndex];
            let prevOplogEntry;
            assert(firstReader.hasNext(), "oplog is empty while checkOplogs is called");
            // Track the number of bytes we are reading as we check the oplog. We use this to avoid
            // out-of-memory issues by calling to garbage collect whenever the memory footprint is
            // large.
            let bytesSinceGC = 0;
            while (firstReader.hasNext()) {
                const oplogEntry = firstReader.next();
                bytesSinceGC += Object.bsonsize(oplogEntry);
                if (oplogEntry === firstReader.kCappedPositionLostSentinel) {
                    // When using legacy OP_QUERY/OP_GET_MORE reads against mongos, it is
                    // possible for hasNext() to return true but for next() to throw an exception.
                    break;
                }

                for (let i = 0; i < nodes.length; i++) {
                    // Skip reading from this reader if the index is the same as firstReader or
                    // the cursor is exhausted.
                    if (i === firstReaderIndex || !(readers[i] && readers[i].hasNext())) {
                        continue;
                    }

                    const otherOplogEntry = readers[i].next();
                    bytesSinceGC += Object.bsonsize(otherOplogEntry);
                    if (otherOplogEntry &&
                        otherOplogEntry !== readers[i].kCappedPositionLostSentinel) {
                        assertOplogEntriesEq.call(this,
                                                  oplogEntry,
                                                  otherOplogEntry,
                                                  firstReader,
                                                  readers[i],
                                                  prevOplogEntry);
                    }
                }
                // Garbage collect every 10MB.
                if (bytesSinceGC > (10 * 1024 * 1024)) {
                    gc();
                    bytesSinceGC = 0;
                }
                prevOplogEntry = oplogEntry;
            }
        }
        print("checkOplogs oplog checks complete.");
    }

    function getPreImageReaders(msgPrefix, rst, secondaries, nsUUID) {
        const readers = [];
        const nodes = rst.nodes;
        for (let i = 0; i < nodes.length; i++) {
            const node = nodes[i];

            if (rst._primary !== node && !secondaries.includes(node)) {
                print(`${msgPrefix} -- skipping preimages of node as it's not in our list of ` +
                      `secondaries: ${node.host}`);
                continue;
            }

            // Arbiters have no documents in the oplog and thus don't have preimages
            // content.
            if (isNodeArbiter(node)) {
                jsTestLog(`${msgPrefix} -- skipping preimages of arbiter node: ${node.host}`);
                continue;
            }

            print(`${msgPrefix} -- going to check preimages of ${nsUUID} of node: ${node.host}`);
            readers[i] = new ReverseReader(
                node, node.getDB("config")["system.preimages"], {"_id.nsUUID": nsUUID});
            // Start all reverseReaders at their last document for the collection.
            readers[i].query();
        }

        return readers;
    }

    function dumpPreImagesCollection(msgPrefix, node, nsUUID, timestamp, limit) {
        const beforeCursor =
            node.getDB("config")["system.preimages"]
                .find({"_id.nsUUID": nsUUID, "_id.ts": {"$lt": timestamp}})
                .sort({$natural: -1})
                .noCursorTimeout()
                .readConcern("local")
                .limit(limit / 2);  // We print up to half of the limit in the before part so that
                                    // the timestamp is centered.
        const beforeEntries = beforeCursor.toArray().reverse();

        let log = `${msgPrefix} -- Dumping a window of ${
            limit} entries for preimages of collection ${nsUUID} from host ${
            node.host} centered around timestamp ${timestamp.toStringIncomparable()}`;

        beforeEntries.forEach(entry => {
            log += '\n' + tojsononeline(entry);
        });

        const remainingWindow = limit - beforeEntries.length;
        const cursor = node.getDB("config")["system.preimages"]
                           .find({"_id.nsUUID": nsUUID, "_id.ts": {"$gte": timestamp}})
                           .sort({$natural: 1})
                           .noCursorTimeout()
                           .readConcern("local")
                           .limit(remainingWindow);
        cursor.forEach(entry => {
            log += '\n' + tojsononeline(entry);
        });

        jsTestLog(log);
    }

    /**
     * Check preimages on all nodes, by reading reading from the last time. Since the preimage may
     * or may not be maintained independently, each node may not contain the same number of entries
     * and stop if the cursor is exhausted on any node being checked.
     */
    function checkPreImageCollection(rst, secondaries, msgPrefix = 'checkPreImageCollection') {
        secondaries = secondaries || rst._secondaries;

        const originalPreferences = [];

        print(`${msgPrefix} -- starting preimage checks.`);
        print(`${msgPrefix} -- waiting for secondaries to be ready.`);
        rst.awaitSecondaryNodes(rst.kDefaultTimeoutMS, secondaries);
        if (secondaries.length >= 1) {
            let collectionsWithPreimages = {};
            const nodes = rst.nodes;
            for (let i = 0; i < nodes.length; i++) {
                const node = nodes[i];

                if (rst._primary !== node && !secondaries.includes(node)) {
                    print(`${msgPrefix} -- skipping preimages of node as it's not in our list of ` +
                          `secondaries: ${node.host}`);
                    continue;
                }

                // Arbiters have no documents in the oplog and thus don't have preimages content.
                if (isNodeArbiter(node)) {
                    jsTestLog(`${msgPrefix} -- skipping preimages of arbiter node: ${node.host}`);
                    continue;
                }

                const preImageColl = node.getDB("config")["system.preimages"];
                // Reset connection preferences in case the test has modified them. We'll restore
                // them back to what they were originally in the end.
                originalPreferences[i] = {
                    secondaryOk: preImageColl.getMongo().getSecondaryOk(),
                    readPref: preImageColl.getMongo().getReadPref()
                };

                preImageColl.getMongo().setSecondaryOk(true);
                preImageColl.getMongo().setReadPref(rst._primary === node ? "primary"
                                                                          : "secondary");

                // Find all collections participating in pre-images.
                const collectionsInPreimages =
                    preImageColl.aggregate([{$group: {_id: "$_id.nsUUID"}}]).toArray();
                for (const collTs of collectionsInPreimages) {
                    collectionsWithPreimages[collTs._id] = collTs._id;
                }
            }
            for (const nsUUID of Object.values(collectionsWithPreimages)) {
                const readers = getPreImageReaders(msgPrefix, rst, secondaries, nsUUID);

                while (true) {
                    let preImageEntryToCompare = undefined;
                    let originNode = undefined;
                    for (const reader of readers) {
                        if (reader.hasNext()) {
                            const preImageEntry = reader.next();
                            if (preImageEntryToCompare === undefined) {
                                preImageEntryToCompare = preImageEntry;
                                originNode = reader.mongo;
                            } else {
                                if (!bsonBinaryEqual(preImageEntryToCompare, preImageEntry)) {
                                    // TODO SERVER-55756: Investigate if we can remove this since
                                    // we'll have the data files present in case this fails with
                                    // PeriodicKillSecondaries.
                                    print(
                                        `${msgPrefix} -- preimage inconsistency detected.` +
                                        "\n" +
                                        `${originNode.host} -> ${
                                            tojsononeline(preImageEntryToCompare)}` +
                                        "\n" +
                                        `${reader.mongo.host} -> ${tojsononeline(preImageEntry)}`);
                                    print("Printing previous entries:");
                                    dumpPreImagesCollection(msgPrefix,
                                                            originNode,
                                                            nsUUID,
                                                            preImageEntryToCompare._id.ts,
                                                            100);
                                    dumpPreImagesCollection(
                                        msgPrefix, reader.mongo, nsUUID, preImageEntry._id.ts, 100);
                                    const log = `${msgPrefix} -- non-matching preimage entries:\n` +
                                        `${originNode.host} -> ${
                                                    tojsononeline(preImageEntryToCompare)}\n` +
                                        `${reader.mongo.host} -> ${tojsononeline(preImageEntry)}`;
                                    assert(false, log);
                                }
                            }
                        }
                    }
                    if (preImageEntryToCompare === undefined) {
                        break;
                    }
                }
            }
        }
        print(`${msgPrefix} -- preimages check complete.`);

        // Restore original read preferences used by the connection.
        for (const idx in originalPreferences) {
            const node = rst.nodes[idx];
            const conn = node.getDB("config").getMongo();
            conn.setSecondaryOk(originalPreferences[idx].secondaryOk);
            conn.setReadPref(originalPreferences[idx].readPref);
        }
    }

    ReplSetTest.prototype.checkPreImageCollection = function(msgPrefix) {
        var liveSecondaries = _determineLiveSecondaries(this);
        this.checkReplicaSet(
            checkPreImageCollection, liveSecondaries, this, liveSecondaries, msgPrefix);
    };

    function dumpChangeCollection(node, tenantDatabaseName, timestamp, limit, msgPrefix) {
        const beforeCursor =
            node.getDB(tenantDatabaseName)["system.change_collection"]
                .find({"_id": {"$lt": timestamp}})
                .sort({$natural: -1})
                .noCursorTimeout()
                .readConcern("local")
                .limit(limit / 2);  // We print up to half of the limit in the before part so that
                                    // the timestamp is centered.
        const beforeEntries = beforeCursor.toArray().reverse();

        let log = `${msgPrefix} -- Dumping a window of ${limit} entries for ${
            tenantDatabaseName}.system.change_collection from host ${node.host} centered around ${
            timestamp.toStringIncomparable()}`;

        beforeEntries.forEach(entry => {
            log += '\n' + tojsononeline(entry);
        });

        const remainingWindow = limit - beforeEntries.length;
        const cursor = node.getDB(tenantDatabaseName)["system.change_collection"]
                           .find({"_id": {"$gte": timestamp}})
                           .sort({$natural: 1})
                           .noCursorTimeout()
                           .readConcern("local")
                           .limit(remainingWindow);
        cursor.forEach(entry => {
            log += '\n' + tojsononeline(entry);
        });

        jsTestLog(log);
    }

    function checkTenantChangeCollection(
        rst, secondaries, db, msgPrefix = 'checkTenantChangeCollection') {
        const tenantDatabaseName = db.name;
        print(`${msgPrefix} -- starting check on ${db.tenantId} ${
            tenantDatabaseName}.system.change_collection`);

        // Prepare reverse read from the primary and specified secondaries.
        const nodes = [rst.getPrimary(), ...secondaries];
        let reverseReaders = nodes.map(node => {
            let reader =
                new ReverseReader(node, node.getDB(tenantDatabaseName)["system.change_collection"]);
            // Start all reverseReaders at their last document for the collection.
            reader.query();
            return reader;
        });

        let inspectedEntryCount = 0;
        while (true) {
            const entryAndNodeSet = reverseReaders.map(reader => {
                if (reader.hasNext()) {
                    return {entry: reader.next(), node: reader.mongo};
                }
                return undefined;
            });
            let baselineEntryAndNode = undefined;

            entryAndNodeSet.forEach(entryAndNode => {
                if (entryAndNode === undefined) {
                    return;
                }

                if (baselineEntryAndNode === undefined) {
                    inspectedEntryCount++;
                    baselineEntryAndNode = entryAndNode;
                    return;
                }
                if (!bsonBinaryEqual(baselineEntryAndNode.entry, entryAndNode.entry)) {
                    print(`${msgPrefix} -- inconsistency detected in ${
                              tenantDatabaseName}.system.change_collection` +
                          "\n" +
                          `${baselineEntryAndNode.node.host} -> ${
                              tojsononeline(baselineEntryAndNode.entry)}` +
                          "\n" +
                          `${entryAndNode.node.host} -> ${tojsononeline(entryAndNode.entry)}`);

                    dumpChangeCollection(baselineEntryAndNode.node,
                                         tenantDatabaseName,
                                         baselineEntryAndNode.entry._id,
                                         100,
                                         msgPrefix);
                    dumpChangeCollection(entryAndNode.node,
                                         tenantDatabaseName,
                                         entryAndNode.entry._id,
                                         100,
                                         msgPrefix);
                    assert(
                        false,
                        `Found inconsistency in '${tenantDatabaseName}.system.change_collection'`);
                }
            });

            if (baselineEntryAndNode === undefined) {
                break;
            }
        }
        print(`${msgPrefix} -- finished check on ${
            tenantDatabaseName}.system.change_collection, inspected ${
            inspectedEntryCount} unique entries`);
    }

    /**
     * Check change_collection for all tenants on all nodes, by doing a reverse scan. This check
     * accounts for the fact that each node might independently truncate the change collection, and
     * not contain the same number of entries.
     */
    function checkChangeCollection(rst, secondaries, msgPrefix = 'checkChangeCollection') {
        secondaries = secondaries || rst._secondaries;
        secondaries = secondaries.filter((node) => !isNodeArbiter(node));

        if (secondaries.length == 0) {
            print(`${msgPrefix} -- no data bearing secondaries specified, nothing to do.`);
            return;
        }

        print(`${msgPrefix} -- starting change_collection checks.`);
        print(`${msgPrefix} -- waiting for secondaries to be ready.`);
        rst.awaitSecondaryNodes(rst.kDefaultTimeoutMS, secondaries);

        // Get all change_collections for all tenants.
        let dbs = rst.getPrimary().getDBs();
        dbs = dbs.databases.filter((db) => db.name.endsWith("_config") || db.name == "config");
        dbs.forEach(db => {
            if (db.tenantId) {
                try {
                    const token = _createTenantToken({tenant: db.tenantId});
                    rst.nodes.forEach(node => node._setSecurityToken(token));
                    checkTenantChangeCollection(rst, secondaries, db);
                } finally {
                    rst.nodes.forEach(node => node._setSecurityToken(undefined));
                }
            } else {
                checkTenantChangeCollection(rst, secondaries, db);
            }
        });
        print(`${msgPrefix} -- change_collection check complete.`);
    }

    ReplSetTest.prototype.checkChangeCollection = function(msgPrefix) {
        var liveSecondaries = _determineLiveSecondaries(this);
        this.checkReplicaSet(
            checkChangeCollection, liveSecondaries, this, liveSecondaries, msgPrefix);
    };

    /**
     * Waits for an initial connection to a given node. Should only be called after the node's
     * process has already been started. Updates the corresponding entry in 'this.nodes' with the
     * newly established connection object.
     *
     * @param {int} [n] the node id.
     * @param {boolean} [waitForHealth] If true, wait for the health indicator of the replica set
     *     node after waiting for a connection. Default: false.
     * @returns a new Mongo connection object to the node.
     */
    ReplSetTest.prototype._waitForInitialConnection = function(n, waitForHealth) {
        print("ReplSetTest waiting for an initial connection to node " + n);

        // If we are using a bridge, then we want to get at the underlying mongod node object.
        let node = this._useBridge ? this._unbridgedNodes[n] : this.nodes[n];
        let pid = node.pid;
        let port = node.port;
        let conn = MongoRunner.awaitConnection({pid, port});
        if (!conn) {
            throw new Error("Failed to connect to node " + n);
        }

        // Attach the original node properties to the connection object.
        Object.assign(conn, node);

        // Delete the session since it's linked to the other mongo object.
        delete conn._defaultSession;

        // Authenticate again since this is a new connection.
        if ((jsTestOptions().keyFile || this.clusterAuthMode === "x509")) {
            // The sslSpecial suite sets up cluster with x509 but the shell was not started with TLS
            // so we need to rely on the test to auth if needed.
            if (!(this.clusterAuthMode === "x509" && !conn.isTLS())) {
                jsTest.authenticate(conn);
            }
        }

        // Save the new connection object. If we are using a bridge, then we need to connect to it.
        if (this._useBridge) {
            this.nodes[n].connectToBridge();
            this.nodes[n].nodeId = n;
            this._unbridgedNodes[n] = conn;
        } else {
            this.nodes[n] = conn;
        }

        print("ReplSetTest made initial connection to node: " + tojson(this.nodes[n]));

        waitForHealth = waitForHealth || false;
        if (waitForHealth) {
            // Wait for node to start up.
            this._waitForIndicator(this.nodes[n], "health", Health.UP);
        }

        if (this._causalConsistency) {
            this.nodes[n].setCausalConsistency(true);
        }
        return this.nodes[n];
    };

    /**
     * Starts up a server.  Options are saved by default for subsequent starts.
     *
     *
     * Options { remember : true } re-applies the saved options from a prior start.
     * Options { noRemember : true } ignores the current properties.
     * Options { appendOptions : true } appends the current options to those remembered.
     * Options { startClean : true } clears the data directory before starting.
     *
     * @param {int|conn|[int|conn]} n array or single server number (0, 1, 2, ...) or conn
     * @param {object} [options]
     * @param {boolean} [restart] If false, the data directory will be cleared
     *   before the server starts.  Default: false.
     * @param {boolean} [waitForHealth] If true, wait for the health indicator of the replica set
     *     node after waiting for a connection. Default: false.
     * @param {boolean} [isMixedVersionCluster] If true, it tells mongorunner that this node is part
     *     of a mixed version cluster, and will add --upgradeBackCompat when appropriate.
     *     Default: false.
     */
    ReplSetTest.prototype.start = _nodeParamToSingleNode(_nodeParamToId(function(
        n, options, restart, waitForHealth, isMixedVersionCluster) {
        print("ReplSetTest n is : " + n);

        var defaults = {
            useHostName: this.useHostName,
            oplogSize: this.oplogSize,
            keyFile: this.keyFile,
            port: this._useBridge ? this._unbridgedPorts[n] : this.ports[n],
            dbpath: "$set-$node"
        };
        if (this.isRouterServer) {
            defaults.routerPort = this.routerPorts[n];
        }

        if (this.useAutoBootstrapProcedure) {
            if (n == 0) {
                // No --replSet for the first node.
            } else {
                defaults.replSet = this.name;
            }
        } else if (this.serverless == null) {
            defaults.replSet = this.useSeedList ? this.getURL() : this.name;
        } else {
            defaults.serverless = true;
        }

        const nodeOptions = this.nodeOptions["n" + n];
        const hasBinVersion =
            (options && options.binVersion) || (nodeOptions && nodeOptions.binVersion);
        if (hasBinVersion && jsTest.options().useRandomBinVersionsWithinReplicaSet) {
            throw new Error(
                "Can only specify one of binVersion and useRandomBinVersionsWithinReplicaSet, not both.");
        }

        // Note : this replaces the binVersion of the shared startSet() options the first time
        // through, so the full set is guaranteed to have different versions if size > 1.  If using
        // start() independently, independent version choices will be made
        //
        if (options && options.binVersion) {
            options.binVersion = MongoRunner.versionIterator(options.binVersion);
        }

        // Always set log format
        if (options && options.logFormat) {
            options.logFormat = jsTest.options().logFormat;
        }

        // If restarting a node, use its existing options as the defaults unless remember is false.
        var baseOptions;
        if ((options && options.restart) || restart) {
            if (options && options.remember === false) {
                baseOptions = defaults;
            } else {
                baseOptions = this._useBridge ? this._unbridgedNodes[n].fullOptions
                                              : this.nodes[n].fullOptions;
            }
        } else {
            baseOptions = defaults;
        }
        baseOptions = Object.merge(baseOptions, nodeOptions);
        options = Object.merge(baseOptions, options);
        if (options.hasOwnProperty("rsConfig")) {
            this.nodeOptions["n" + n] =
                Object.merge(this.nodeOptions["n" + n], {rsConfig: options.rsConfig});
        }
        delete options.rsConfig;

        if (jsTest.options().useRandomBinVersionsWithinReplicaSet) {
            if (this.isConfigServer) {
                // Our documented upgrade/downgrade paths for a sharded cluster lets us assume that
                // config server nodes will always be fully upgraded before the shard nodes.
                options.binVersion = "latest";
                options.upgradeBackCompat = '';
            } else {
                if (Random.rand() < 0.5) {
                    options.binVersion = "latest";
                    options.upgradeBackCompat = '';
                } else {
                    options.binVersion = jsTest.options().useRandomBinVersionsWithinReplicaSet;
                    options.removeOptions = (options.removeOptions ? options.removeOptions : [])
                                                .concat("upgradeBackCompat");
                    delete options.upgradeBackCompat;
                }
            }
            print("Randomly assigned binary version: " + options.binVersion + " to node: " + n);
        }

        options.restart = options.restart || restart;

        var pathOpts = {node: n, set: this.name};
        options.pathOpts = Object.merge(options.pathOpts || {}, pathOpts);

        // Turn off periodic noop writes for replica sets by default.
        options.setParameter = options.setParameter || {};
        if (typeof (options.setParameter) === "string") {
            var eqIdx = options.setParameter.indexOf("=");
            if (eqIdx != -1) {
                var param = options.setParameter.substring(0, eqIdx);
                var value = options.setParameter.substring(eqIdx + 1);
                options.setParameter = {};
                options.setParameter[param] = value;
            }
        }
        options.setParameter.writePeriodicNoops = options.setParameter.writePeriodicNoops || false;

        // We raise the number of initial sync connect attempts for tests that disallow chaining.
        // Disabling chaining can cause sync source selection to take longer so we must increase
        // the number of connection attempts.
        options.setParameter.numInitialSyncConnectAttempts =
            options.setParameter.numInitialSyncConnectAttempts || 60;

        // The default time for stepdown and quiesce mode in response to SIGTERM is 15 seconds.
        // Reduce this to 100ms for faster shutdown.
        options.setParameter.shutdownTimeoutMillisForSignaledShutdown =
            options.setParameter.shutdownTimeoutMillisForSignaledShutdown || 100;

        // This parameter is enabled to allow the default write concern to change while
        // initiating a ReplSetTest. This is due to our testing optimization to initiate
        // with a single node, and reconfig the full membership set in.
        // We need to recalculate the DWC after each reconfig until the full set is included.
        options.setParameter.enableDefaultWriteConcernUpdatesForInitiate = true;

        if (baseOptions.hasOwnProperty("setParameter") &&
            baseOptions.setParameter.hasOwnProperty("featureFlagTransitionToCatalogShard") &&
            baseOptions.setParameter.featureFlagTransitionToCatalogShard) {
            options.setParameter.featureFlagTransitionToCatalogShard = true;
        }

        // Disable a check in reconfig that will prevent certain configs with arbiters from
        // spinning up. We will re-enable this check after the replica set has finished initiating.
        if (jsTestOptions().enableTestCommands) {
            options.setParameter.enableReconfigRollbackCommittedWritesCheck = false;
            options.setParameter.disableTransitionFromLatestToLastContinuous =
                options.setParameter.disableTransitionFromLatestToLastContinuous || false;
        }

        if (jsTestOptions().performTimeseriesCompressionIntermediateDataIntegrityCheckOnInsert) {
            options.setParameter
                .performTimeseriesCompressionIntermediateDataIntegrityCheckOnInsert = true;
        }

        if (this.useAutoBootstrapProcedure) {
            options.setParameter.featureFlagAllMongodsAreSharded = true;
        }

        if (jsTest.options().nonClusteredConfigTransactions) {
            options.setParameter.featureFlagClusteredConfigTransactions = false;
        }
        const olderThan73 =
            MongoRunner.compareBinVersions(MongoRunner.getBinVersionFor('7.3'),
                                           MongoRunner.getBinVersionFor(options.binVersion)) === 1;
        if (olderThan73) {
            delete options.setParameter.featureFlagClusteredConfigTransactions;
        }

        const olderThan80 =
            MongoRunner.compareBinVersions(MongoRunner.getBinVersionFor(options.binVersion),
                                           MongoRunner.getBinVersionFor('8.0')) === -1;
        if (olderThan80) {
            delete options.setParameter
                .performTimeseriesCompressionIntermediateDataIntegrityCheckOnInsert;
        }

        if (tojson(options) != tojson({}))
            printjson(options);

        print("ReplSetTest " + (restart ? "(Re)" : "") + "Starting....");

        if (this._useBridge && (restart === undefined || !restart)) {
            // We leave the mongobridge process running when the mongod process is restarted so we
            // don't need to start a new one.
            var bridgeOptions = Object.merge(this._bridgeOptions, options.bridgeOptions || {});
            bridgeOptions = Object.merge(bridgeOptions, {
                hostName: this.host,
                port: this.ports[n],
                // The mongod processes identify themselves to mongobridge as host:port, where the
                // host is the actual hostname of the machine and not localhost.
                dest: getHostName() + ":" + this._unbridgedPorts[n],
            });

            if (jsTestOptions().networkMessageCompressors) {
                bridgeOptions["networkMessageCompressors"] =
                    jsTestOptions().networkMessageCompressors;
            }

            this.nodes[n] = new MongoBridge(bridgeOptions);
        }

        // Save this property since it may be deleted inside 'runMongod'.
        var waitForConnect = options.waitForConnect;

        // Never wait for a connection inside runMongod. We will do so below if needed.
        options.waitForConnect = false;
        var conn = MongoRunner.runMongod(options, isMixedVersionCluster === true);
        if (!conn) {
            throw new Error("Failed to start node " + n);
        }

        // Make sure to call _addPath, otherwise folders won't be cleaned.
        this._addPath(conn.dbpath);

        // We don't want to persist 'waitForConnect' across node restarts.
        delete conn.fullOptions.waitForConnect;

        // Save the node object in the appropriate location.
        if (this._useBridge) {
            this._unbridgedNodes[n] = conn;
        } else {
            this.nodes[n] = conn;
            this.nodes[n].nodeId = n;
        }

        // Clean up after noReplSet to ensure it doesn't effect future restarts.
        if (options.noReplSet) {
            this.nodes[n].fullOptions.replSet = defaults.replSet;
            delete this.nodes[n].fullOptions.noReplSet;
        }

        // Wait for a connection to the node if necessary.
        if (waitForConnect === false) {
            print("ReplSetTest start skip waiting for a connection to node " + n);
            return this.nodes[n];
        }

        const connection = this._waitForInitialConnection(n, waitForHealth);

        if (n == 0 && this.useAutoBootstrapProcedure && !this._hasAcquiredAutoGeneratedName) {
            const helloReply = connection.getDB('admin')._helloOrLegacyHello();
            print('ReplSetTest start using auto generated replSet name ' + helloReply.setName +
                  ' instead of ' + this.name);
            this.name = helloReply.setName;
            this._hasAcquiredAutoGeneratedName = true;
        }

        return connection;
    }));

    /**
     * Restarts a db without clearing the data directory by default, and using the node(s)'s
     * original startup options by default.
     *
     * When using this method with mongobridge, be aware that mongobridge may not do a good
     * job of detecting that a node was restarted. For example, when mongobridge is being used
     * between some Node A and Node B, on restarting Node B mongobridge will not aggressively
     * close its connection with Node A, leading Node A to think the connection with Node B is
     * still healthy.
     *
     * Option { startClean : true } forces clearing the data directory.
     * Option { auth : Object } object that contains the auth details for admin credentials.
     *   Should contain the fields 'user' and 'pwd'
     *
     * In order not to use the original startup options, use stop() (or stopSet()) followed by
     * start() (or startSet()) without passing restart: true as part of the options.
     *
     * @param {int|conn|[int|conn]} n array or single server number (0, 1, 2, ...) or conn
     */
    ReplSetTest.prototype.restart = function(n, options, signal, wait) {
        // Can specify wait as third parameter, if using default signal
        if (signal == true || signal == false) {
            wait = signal;
            signal = undefined;
        }

        this.stop(n, signal, options, {forRestart: true});

        var started = this.start(n, options, true, wait);

        // We should not attempt to reauthenticate the connection if we did not wait for it
        // to be reestablished in the first place.
        const skipWaitForConnection = (options && options.waitForConnect === false);
        if (jsTestOptions().keyFile && !skipWaitForConnection) {
            if (started.length) {
                // if n was an array of conns, start will return an array of connections
                for (var i = 0; i < started.length; i++) {
                    assert(jsTest.authenticate(started[i]), "Failed authentication during restart");
                }
            } else {
                assert(jsTest.authenticate(started), "Failed authentication during restart");
            }
        }
        return started;
    };

    /**
     * Step down and freeze a particular node or nodes.
     *
     * @param node is a single node or list of nodes, by id or conn
     */
    ReplSetTest.prototype.freeze = _nodeParamToSingleNode(_nodeParamToConn(function(node) {
        assert.soon(() => {
            try {
                // Ensure node is authenticated.
                asCluster(this, node, () => {
                    // Ensure node is not primary. Ignore errors, probably means it's already
                    // secondary.
                    node.adminCommand({replSetStepDown: ReplSetTest.kForeverSecs, force: true});
                    // Prevent node from running election. Fails if it already started an election.
                    assert.commandWorked(
                        node.adminCommand({replSetFreeze: ReplSetTest.kForeverSecs}));
                });
                return true;
            } catch (e) {
                if (isNetworkError(e) || e.code === ErrorCodes.NotSecondary ||
                    e.code === ErrorCodes.NotYetInitialized) {
                    jsTestLog(`Failed to freeze node ${node.host}: ${e}`);
                    return false;
                }

                throw e;
            }
        }, `Failed to run replSetFreeze cmd on ${node.host}`);
    }));

    /**
     * Unfreeze a particular node or nodes.
     *
     * @param node is a single node or list of nodes, by id or conn
     */
    ReplSetTest.prototype.unfreeze = _nodeParamToSingleNode(_nodeParamToConn(function(node) {
        // Ensure node is authenticated.
        asCluster(this, node, () => assert.commandWorked(node.adminCommand({replSetFreeze: 0})));
    }));

    ReplSetTest.prototype.stopPrimary = function(signal, opts) {
        var primary = this.getPrimary();
        var primary_id = this.getNodeId(primary);
        return this.stop(primary_id, signal, opts);
    };

    /**
     * Stops a particular node or nodes, specified by conn or id. If we expect the node to exit with
     * a nonzero exit code, call this function and pass in allowedExitCode as a field of opts.
     *
     * If _useBridge=true, then the mongobridge process(es) corresponding to the node(s) are also
     * terminated unless forRestart=true. The mongobridge process(es) are left running across
     * restarts to ensure their configuration remains intact.
     *
     * @param {number|Mongo} n the index or connection object of the replica set member to stop.
     * @param {number} signal the signal number to use for killing
     * @param {Object} opts @see MongoRunner.stopMongod
     * @param {Object} [extraOptions={}]
     * @param {boolean} [extraOptions.forRestart=false] indicates whether stop() is being called
     * with the intent to call start() with restart=true for the same node(s) n.
     * @param {boolean} [extraOptions.waitPid=true] if true, we will wait for the process to
     * terminate after stopping it.
     */
    ReplSetTest.prototype.stop = _nodeParamToSingleNode(_nodeParamToConn(function(
        n, signal, opts, {forRestart: forRestart = false, waitpid: waitPid = true} = {}) {
        // Can specify wait as second parameter, if using default signal
        if (signal == true || signal == false) {
            signal = undefined;
        }

        n = this.getNodeId(n);

        var conn = this._useBridge ? this._unbridgedNodes[n] : this.nodes[n];

        print('ReplSetTest stop *** Shutting down mongod in port ' + conn.port +
              ', wait for process termination: ' + waitPid + ' ***');
        var ret = MongoRunner.stopMongod(conn, signal, opts, waitPid);

        // We only expect the process to have terminated if we actually called 'waitpid'.
        if (waitPid) {
            print('ReplSetTest stop *** Mongod in port ' + conn.port + ' shutdown with code (' +
                  ret + ') ***');
        }

        if (this._useBridge && !forRestart) {
            // We leave the mongobridge process running when the mongod process is being restarted.
            const bridge = this.nodes[n];
            print('ReplSetTest stop *** Shutting down mongobridge on port ' + bridge.port + ' ***');
            const exitCode = bridge.stop();  // calls MongoBridge#stop()
            print('ReplSetTest stop *** mongobridge on port ' + bridge.port +
                  ' exited with code (' + exitCode + ') ***');
        }

        return ret;
    }));

    /**
     * Performs collection validation on all nodes in the given 'ports' array in parallel.
     *
     * @param {int[]} ports the array of mongo ports to run validation on
     */
    ReplSetTest.prototype._validateNodes = function(ports) {
        // Perform collection validation on each node in parallel.
        let validators = [];
        for (let i = 0; i < ports.length; i++) {
            const validator = new Thread(async function(port) {
                const {CommandSequenceWithRetries} =
                    await import("jstests/libs/command_sequence_with_retries.js");
                const {validateCollections} = await import("jstests/hooks/validate_collections.js");
                await import("jstests/libs/override_methods/validate_collections_on_shutdown.js");
                MongoRunner.validateCollectionsCallback(
                    port, {CommandSequenceWithRetries, validateCollections});
            }, ports[i]);
            validators.push(validator);
            validators[i].start();
        }
        // Wait for all validators to finish.
        for (let i = 0; i < ports.length; i++) {
            validators[i].join();
        }
    };

    /**
     * Kill all members of this replica set. When calling this function, we expect all live nodes to
     * exit cleanly. If we expect a node to exit with a nonzero exit code, use the stop function to
     * terminate that node before calling stopSet.
     *
     * @param {number} signal The signal number to use for killing the members
     * @param {boolean} forRestart will not cleanup data directory
     * @param {Object} opts @see MongoRunner.stopMongod
     */
    ReplSetTest.prototype.stopSet = function(signal, forRestart, opts = {}) {
        if (jsTestOptions().alwaysUseLogFiles) {
            if (opts.noCleanData === false) {
                throw new Error("Always using log files, but received conflicting option.");
            }

            opts.noCleanData = true;
        }
        // Check to make sure data is the same on all nodes.
        const skipChecks = jsTest.options().skipCheckDBHashes || (opts && opts.skipCheckDBHashes);
        if (!skipChecks) {
            let startTime = new Date();  // Measure the execution time of consistency checks.
            print("ReplSetTest stopSet going to run data consistency checks.");
            // To skip this check add TestData.skipCheckDBHashes = true or pass in {opts:
            // skipCheckDBHashes} Reasons to skip this test include:
            // - the primary goes down and none can be elected (so fsync lock/unlock commands fail)
            // - the replica set is in an unrecoverable inconsistent state. E.g. the replica set
            //   is partitioned.
            let primary = _callHello(this);
            if (primary && this._liveNodes.length > 1) {  // skip for sets with 1 live node
                // Auth only on live nodes because authutil.assertAuthenticate
                // refuses to log in live connections if some secondaries are down.
                print("ReplSetTest stopSet checking oplogs.");
                asCluster(this, this._liveNodes, () => this.checkOplogs());
                print("ReplSetTest stopSet checking preimages.");
                asCluster(this, this._liveNodes, () => this.checkPreImageCollection());
                print("ReplSetTest stopSet checking change_collection(s).");
                asCluster(this, this._liveNodes, () => this.checkChangeCollection());
                print("ReplSetTest stopSet checking replicated data hashes.");
                asCluster(this, this._liveNodes, () => this.checkReplicatedDataHashes());
            } else {
                print(
                    "ReplSetTest stopSet skipped data consistency checks. Number of _liveNodes: " +
                    this._liveNodes.length + ", _callHello response: " + primary);
            }
            print("ReplSetTest stopSet data consistency checks finished, took " +
                  (new Date() - startTime) + "ms for " + this.nodes.length + " nodes.");
        }

        let startTime = new Date();  // Measure the execution time of shutting down nodes.

        // Optionally validate collections on all nodes. Parallel validation depends on use of the
        // 'Thread' object, so we check for and load that dependency here. If the dependency is not
        // met, we validate each node serially on shutdown.
        const parallelValidate = tryLoadParallelTester();
        if (opts.skipValidation) {
            print("ReplSetTest stopSet skipping validation before stopping nodes.");
        } else if (parallelValidate) {
            print("ReplSetTest stopSet validating all replica set nodes before stopping them.");
            this._validateNodes(this.ports);
        }

        // Stop all nodes without waiting for them to terminate. We can skip validation on shutdown
        // if we have already done it above.
        opts = Object.merge(opts, {skipValidation: (parallelValidate || opts.skipValidation)});
        for (let i = 0; i < this.ports.length; i++) {
            this.stop(i, signal, opts, {waitpid: false});
        }

        // Wait for all processes to terminate.
        for (let i = 0; i < this.ports.length; i++) {
            let conn = this._useBridge ? this._unbridgedNodes[i] : this.nodes[i];
            let port = parseInt(conn.port);
            print("ReplSetTest stopSet waiting for mongo program on port " + port + " to stop.");
            let exitCode = waitMongoProgram(port);
            if (exitCode !== MongoRunner.EXIT_CLEAN && !opts.skipValidatingExitCode) {
                throw new Error("ReplSetTest stopSet mongo program on port " + port +
                                " shut down unexpectedly with code " + exitCode + " when code " +
                                MongoRunner.EXIT_CLEAN + " was expected.");
            }
            print("ReplSetTest stopSet mongo program on port " + port + " shut down with code " +
                  exitCode);
        }

        print("ReplSetTest stopSet stopped all replica set nodes, took " +
              (new Date() - startTime) + "ms for " + this.ports.length + " nodes.");

        if (forRestart) {
            print("ReplSetTest stopSet returning since forRestart=true.");
            return;
        }

        if ((!opts.noCleanData) && this._alldbpaths) {
            print("ReplSetTest stopSet deleting all dbpaths");
            for (var i = 0; i < this._alldbpaths.length; i++) {
                print("ReplSetTest stopSet deleting dbpath: " + this._alldbpaths[i]);
                resetDbpath(this._alldbpaths[i]);
            }
            print("ReplSetTest stopSet deleted all dbpaths");
        }

        _forgetReplSet(this.name);

        print('ReplSetTest stopSet *** Shut down repl set - test worked ****');
    };

    /**
     * Returns whether or not this ReplSetTest uses mongobridge.
     */
    ReplSetTest.prototype.usesBridge = function() {
        return this._useBridge;
    };

    /**
     * Wait for a state indicator to go to a particular state or states.
     *
     * Note that this waits for the state as indicated by the primary node.  If you want to wait for
     * a node to actually reach SECONDARY state, as reported by itself, use awaitSecondaryNodes
     * instead.
     *
     * @param node is a single node or list of nodes, by id or conn
     * @param state is a single state or list of states
     * @param timeout how long to wait for the state to be reached
     * @param reconnectNode indicates that we should reconnect to a node that stepped down
     *
     */
    ReplSetTest.prototype.waitForState = function(node, state, timeout, reconnectNode) {
        this._waitForIndicator(node, "state", state, timeout, reconnectNode);
    };

    /**
     * Waits until there is a primary node.
     */
    ReplSetTest.prototype.waitForPrimary = function(timeout) {
        var primary;
        assert.soonNoExcept(() => {
            return (primary = this.getPrimary());
        }, "waiting for primary", timeout);

        return primary;
    };

    //
    // ReplSetTest constructors
    //

    /**
     * Constructor, which initializes the ReplSetTest object by starting new instances.
     */
    function _constructStartNewInstances(rst, opts) {
        rst.name = opts.name || jsTest.name();
        print('Starting new replica set ' + rst.name);

        rst.serverless = opts.serverless;
        rst.useHostName = opts.useHostName == undefined ? true : opts.useHostName;
        rst.host = rst.useHostName ? (opts.host || getHostName()) : 'localhost';
        rst.oplogSize = opts.oplogSize || 40;
        rst.useSeedList = opts.useSeedList || false;
        rst.keyFile = opts.keyFile;

        rst.clusterAuthMode = undefined;
        if (opts.clusterAuthMode) {
            rst.clusterAuthMode = opts.clusterAuthMode;
        }

        rst.protocolVersion = opts.protocolVersion;
        rst.waitForKeys = opts.waitForKeys;

        rst.seedRandomNumberGenerator = opts.hasOwnProperty('seedRandomNumberGenerator')
            ? opts.seedRandomNumberGenerator
            : true;
        rst.isConfigServer = opts.isConfigServer;
        rst.isRouterServer = opts.isRouterServer || false;

        rst._useBridge = opts.useBridge || false;
        if (rst._useBridge) {
            assert(
                !jsTestOptions().tlsMode,
                'useBridge cannot be true when using TLS. Add the requires_mongobridge tag to the test to ensure it will be skipped on variants that use TLS.');
        }

        rst._bridgeOptions = opts.bridgeOptions || {};

        rst._causalConsistency = opts.causallyConsistent || false;

        rst._configSettings = opts.settings || false;
        rst.useAutoBootstrapProcedure = opts.useAutoBootstrapProcedure || false;
        rst._hasAcquiredAutoGeneratedName = false;

        rst.nodeOptions = {};

        var numNodes;

        if (isObject(opts.nodes)) {
            var len = 0;
            for (var i in opts.nodes) {
                // opts.nodeOptions and opts.nodes[i] may contain nested objects that have
                // the same key, e.g. setParameter. So we need to recursively merge them.
                // Object.assign and Object.merge do not merge nested objects of the same key.
                var options = rst.nodeOptions["n" + len] =
                    _deepObjectMerge(opts.nodeOptions, opts.nodes[i]);
                if (i.startsWith("a")) {
                    options.arbiter = true;
                }

                len++;
            }

            numNodes = len;
        } else if (Array.isArray(opts.nodes)) {
            for (var i = 0; i < opts.nodes.length; i++) {
                rst.nodeOptions["n" + i] = Object.merge(opts.nodeOptions, opts.nodes[i]);
            }

            numNodes = opts.nodes.length;
        } else {
            for (var i = 0; i < opts.nodes; i++) {
                rst.nodeOptions["n" + i] = opts.nodeOptions;
            }

            numNodes = opts.nodes;
        }

        for (let i = 0; i < numNodes; i++) {
            if (rst.nodeOptions["n" + i] !== undefined &&
                rst.nodeOptions["n" + i].clusterAuthMode == "x509") {
                rst.clusterAuthMode = "x509";
            }
        }

        if (rst._useBridge) {
            let makeAllocatePortFn = (preallocatedPorts) => {
                let idxNextNodePort = 0;

                return function() {
                    if (idxNextNodePort >= preallocatedPorts.length) {
                        throw new Error("Cannot use a replica set larger than " +
                                        preallocatedPorts.length + " members with useBridge=true");
                    }

                    const nextPort = preallocatedPorts[idxNextNodePort];
                    ++idxNextNodePort;
                    return nextPort;
                };
            };

            rst._allocatePortForBridge =
                makeAllocatePortFn(allocatePorts(MongoBridge.kBridgeOffset));
            rst._allocatePortForNode = makeAllocatePortFn(allocatePorts(MongoBridge.kBridgeOffset));
        } else {
            rst._allocatePortForBridge = function() {
                throw new Error("Using mongobridge isn't enabled for this replica set");
            };
            rst._allocatePortForNode = allocatePort;
        }

        rst.nodes = [];

        if (rst._useBridge) {
            rst.ports = Array.from({length: numNodes}, rst._allocatePortForBridge);
            rst._unbridgedPorts = Array.from({length: numNodes}, rst._allocatePortForNode);
            rst._unbridgedNodes = [];
        } else {
            rst.ports = Array.from({length: numNodes}, rst._allocatePortForNode);
        }

        for (let i = 0; i < numNodes; i++) {
            const nodeOpts = rst.nodeOptions["n" + i];
            if (nodeOpts && nodeOpts.hasOwnProperty("port")) {
                if (rst._useBridge) {
                    rst._unbridgedPorts[i] = nodeOpts.port;
                } else {
                    rst.ports[i] = nodeOpts.port;
                }
            }
        }

        if (rst.isRouterServer) {
            rst.routerPorts = Array.from({length: numNodes}, rst._allocatePortForNode);
        }
    }

    /**
     * Constructor, which instantiates the ReplSetTest object from an existing set.
     */
    function _constructFromExistingSeedNode(rst, seedNode) {
        const conn = new Mongo(seedNode);
        if (jsTest.options().keyFile) {
            rst.keyFile = jsTest.options().keyFile;
        }
        var conf = asCluster(rst, conn, () => _replSetGetConfig(conn));
        print('Recreating replica set from config ' + tojson(conf));

        var existingNodes = conf.members.map(member => member.host);
        rst.ports = existingNodes.map(node => node.split(':')[1]);
        rst.nodes = existingNodes.map(node => {
            // Note: the seed node is required to be operational in order for the Mongo
            // shell to connect to it. In this code there is no fallback to other nodes.
            let conn = new Mongo(node);
            conn.name = conn.host;
            return conn;
        });
        rst.waitForKeys = false;
        rst.host = existingNodes[0].split(':')[0];
        rst.name = conf._id;
    }

    /**
     * Constructor, which instantiates the ReplSetTest object from existing nodes.
     */
    function _constructFromExistingNodes(rst, {
        name,
        serverless,
        nodeHosts,
        nodeOptions,
        keyFile,
        host,
        waitForKeys,
        useAutoBootstrapProcedure,
        pidValue = undefined
    }) {
        print('Recreating replica set from existing nodes ' + tojson(nodeHosts));

        rst.name = name;
        rst.serverless = serverless;
        rst.ports = nodeHosts.map(node => node.split(':')[1]);

        let i = 0;
        rst.nodes = nodeHosts.map((node) => {
            const conn = Mongo(node);
            conn.name = conn.host;
            conn.port = node.split(':')[1];
            if (pidValue !== undefined && pidValue[i] !== undefined) {
                conn.pid = pidValue[i];
                i++;
            }
            return conn;
        });

        rst.host = host;
        rst.waitForKeys = waitForKeys;
        rst.keyFile = keyFile;
        rst.nodeOptions = nodeOptions;
        rst.useAutoBootstrapProcedure = useAutoBootstrapProcedure || false;
    }

    // If opts is passed in as a string, let it pass unmodified since strings are pass-by-value.
    // if it is an object, though, pass in a deep copy.
    if (typeof opts === 'string' || opts instanceof String) {
        // TODO SERVER-92022: Re-evaluate retry on NotYetInitialized errors
        retryOnRetryableError(() => {
            // The primary may unexpectedly step down during startup if under heavy load
            // and too slowly processing heartbeats. When it steps down, it closes all of
            // its connections.
            _constructFromExistingSeedNode(this, opts);
        }, ReplSetTest.kDefaultRetries, 1000, [ErrorCodes.NotYetInitialized]);
    } else if (typeof opts.rstArgs === "object") {
        _constructFromExistingNodes(this, Object.extend({}, opts.rstArgs, true));
    } else {
        _constructStartNewInstances(this, Object.extend({}, opts, true));
    }

    /**
     * Recursively merge the target and source object.
     */
    function _deepObjectMerge(target, source) {
        if (!(target instanceof Object)) {
            return (source === undefined || source === null) ? target : source;
        }

        if (!(source instanceof Object)) {
            return target;
        }

        let res = Object.assign({}, target);
        Object.keys(source).forEach(k => {
            res[k] = _deepObjectMerge(target[k], source[k]);
        });

        return res;
    }
};

/**
 *  Global default timeout (10 minutes).
 */
ReplSetTest.kDefaultTimeoutMS = 10 * 60 * 1000;
ReplSetTest.kDefaultRetries = 240;

/**
 *  Global default number that's effectively infinite.
 */
ReplSetTest.kForeverSecs = 24 * 60 * 60;
ReplSetTest.kForeverMillis = ReplSetTest.kForeverSecs * 1000;

/**
 * Set of states that the replica set can be in. Used for the wait functions.
 */
ReplSetTest.State = {
    PRIMARY: 1,
    SECONDARY: 2,
    RECOVERING: 3,
    // Note there is no state 4
    STARTUP_2: 5,
    UNKNOWN: 6,
    ARBITER: 7,
    DOWN: 8,
    ROLLBACK: 9,
    REMOVED: 10,
};

ReplSetTest.OpTimeType = {
    LAST_APPLIED: 1,
    LAST_DURABLE: 2,
};

// Adding this to put ReplSetTest in globalThis
globalThis.ReplSetTest = ReplSetTest;