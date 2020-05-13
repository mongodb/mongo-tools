/**
 * Utilities for testing writeConcern.
 */

// Shards a collection with 'numDocs' documents and creates 2 chunks, one on each of two shards.
function shardCollectionWithChunks(st, coll, numDocs) {
    var _db = coll.getDB();
    var numberDoc = numDocs || 20;
    coll.ensureIndex({x: 1}, {unique: true});
    st.ensurePrimaryShard(_db.toString(), st.shard0.shardName);
    st.shardColl(
        coll.getName(), {x: 1}, {x: numberDoc / 2}, {x: numberDoc / 2}, _db.toString(), true);

    for (var i = 0; i < numberDoc; i++) {
        coll.insert({x: i});
    }
    assert.eq(coll.count(), numberDoc);
}

// Stops replication on the given server(s).
function stopServerReplication(conn) {
    if (conn.length) {
        conn.forEach(function(n) {
            stopServerReplication(n);
        });
        return;
    }

    // Clear ramlog so checkLog can't find log messages from previous times this fail point was
    // enabled.
    assert.commandWorked(conn.adminCommand({clearLog: 'global'}));
    var errMsg = 'Failed to enable stopReplProducer failpoint.';
    assert.commandWorked(
        conn.adminCommand({configureFailPoint: 'stopReplProducer', mode: 'alwaysOn'}), errMsg);

    // Wait until the fail point is actually hit. Don't wait if the node is the primary, because
    // the fail point won't be hit until the node transitions from being the primary.
    if (assert.commandWorked(conn.adminCommand('replSetGetStatus')).myState !=
        ReplSetTest.State.PRIMARY) {
        checkLog.contains(conn, 'bgsync - stopReplProducer fail point enabled');
    }
}

// Stops replication at all replicaset secondaries.
function stopReplicationOnSecondaries(rs) {
    stopServerReplication(rs.getSecondaries());
}

// Stops replication at all shard secondaries.
function stopReplicationOnSecondariesOfAllShards(st) {
    st._rsObjects.forEach(stopReplicationOnSecondaries);
}

// Restarts replication on the given server(s).
function restartServerReplication(conn) {
    if (conn.length) {
        conn.forEach(function(n) {
            restartServerReplication(n);
        });
        return;
    }

    var errMsg = 'Failed to disable stopReplProducer failpoint.';
    assert.commandWorked(
        conn.getDB('admin').runCommand({configureFailPoint: 'stopReplProducer', mode: 'off'}),
        errMsg);
}

// Restarts replication at all nodes in a replicaset.
function restartReplSetReplication(rs) {
    restartServerReplication(rs.nodes);
}

// Restarts replication at all replicaset secondaries.
function restartReplicationOnSecondaries(rs) {
    restartServerReplication(rs.getSecondaries());
}

// Restarts replication at all nodes in a sharded cluster.
function restartReplicationOnAllShards(st) {
    st._rsObjects.forEach(restartReplSetReplication);
    restartReplSetReplication(st.configRS);
}

// Asserts that a writeConcernError was received.
function assertWriteConcernError(res) {
    assert(res.writeConcernError, "No writeConcernError received, got: " + tojson(res));
    assert(res.writeConcernError.code, "No writeConcernError code, got: " + tojson(res));
    assert(res.writeConcernError.errmsg, "No writeConcernError errmsg, got: " + tojson(res));
}

// Run the specified command, on the admin database if specified.
function runCommandCheckAdmin(db, cmd) {
    if (cmd.admin) {
        return db.adminCommand(cmd.req);
    } else {
        return db.runCommand(cmd.req);
    }
}

// Asserts that writeConcern timed out.
function checkWriteConcernTimedOut(res) {
    assertWriteConcernError(res);
    const errInfo = res.writeConcernError.errInfo;
    assert(errInfo, "No writeConcernError errInfo, got: " + tojson(res));
    assert(errInfo.wtimeout, "No errInfo wtimeout, got: " + tojson(res));
}

/**
 * Tests that a command properly waits for writeConcern on retry. Takes an optional
 * 'setupFunc' that sets up the database state. 'setupFunc' accepts a connection to the
 * primary.
 */
function runWriteConcernRetryabilityTest(priConn, secConn, cmd, kNodes, dbName, setupFunc) {
    dbName = dbName || "test";
    jsTestLog(`Testing ${tojson(cmd)} on ${dbName}.`);

    // Send a dummy write to this connection so it will have the Client object initialized.
    const secondPriConn = new Mongo(priConn.host);
    const testDB2 = secondPriConn.getDB(dbName);
    assert.commandWorked(testDB2.dummy.insert({x: 1}, {writeConcern: {w: kNodes}}));

    if (setupFunc) {
        setupFunc(priConn);
    }

    stopServerReplication(secConn);

    const testDB = priConn.getDB(dbName);
    checkWriteConcernTimedOut(testDB.runCommand(cmd));

    // Retry the command on the new connection whose lastOp will be less than the main connection.
    checkWriteConcernTimedOut(testDB2.runCommand(cmd));

    // Retry the command on the main connection whose lastOp will not have changed.
    checkWriteConcernTimedOut(testDB.runCommand(cmd));

    // Bump forward the client lastOp on both connections and try again on both.
    assert.commandWorked(testDB.dummy.insert({x: 2}));
    assert.commandWorked(testDB2.dummy.insert({x: 3}));
    checkWriteConcernTimedOut(testDB.runCommand(cmd));
    checkWriteConcernTimedOut(testDB2.runCommand(cmd));

    restartServerReplication(secConn);
}
