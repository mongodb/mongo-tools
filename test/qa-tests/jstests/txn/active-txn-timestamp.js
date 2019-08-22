// Verifies that mongodump --oplog will start the oplog dump from the active
// transaction timestamp instead of the last entry in the oplog, if a transaction
// is in progress.
//
// Specifically, this test covers only large transactions, but we believe it covers
// prepared transactions as well.
//
// @tags: [requires_min_mongo_42]
(function() {
  "use strict";
  load("jstests/libs/check_log.js");
  load('jstests/libs/extended_assert.js');
  var assert = extendedAssert;

  function getTxnTableEntry(db) {
    let txnTableEntries = db.getSiblingDB("config")["transactions"].find().toArray();
    assert.eq(txnTableEntries.length, 1);
    return txnTableEntries[0];
  }

  function getLatestOplogTimestamp(db) {
    let oplogEntries = db.getSiblingDB("local")["oplog.rs"].find().sort({"$natural": -1}).limit(1).toArray();
    assert.eq(oplogEntries.length, 1);
    return oplogEntries[0];
  }

  const replTest = new ReplSetTest({
    nodes: 3,
    nodeOptions: {
      setParameter: {
        maxNumberOfTransactionOperationsInSingleOplogEntry: 1,
        bgSyncOplogFetcherBatchSize: 1
      }
    },
  });

  replTest.startSet();
  let config = replTest.getReplSetConfig();
  config.members[2].priority = 0;
  // Disable primary catchup and chaining.
  config.settings = {catchUpTimeoutMillis: 0, chainingAllowed: false};
  replTest.initiate(config);

  const dbName = jsTest.name();
  const collName = "coll";

  const primary = replTest.nodes[0];
  const testDB = primary.getDB(dbName);
  const secondary = replTest.nodes[1];
  const newTestDB = secondary.getDB(dbName);

  testDB.dropDatabase();
  assert.commandWorked(testDB.runCommand({create: collName, writeConcern: {w: "majority"}}));

  // Prevent the priority: 0 node from fetching new ops so that commit requires other node
  assert.commandWorked(
    replTest.nodes[2].adminCommand({configureFailPoint: 'stopReplProducer', mode: 'alwaysOn'}));

  jsTest.log("Stop secondary oplog replication before the last operation in the transaction.");
  // The stopReplProducerOnDocument failpoint ensures that secondary stops replicating before
  // applying the last operation in the transaction. This depends on the oplog fetcher batch size
  // being 1.
  assert.commandWorked(secondary.adminCommand({
    configureFailPoint: "stopReplProducerOnDocument",
    mode: "alwaysOn",
    data: {document: {"applyOps.o._id": "last in txn"}}
  }));

  jsTestLog("Starting transaction");
  const session = primary.startSession({causalConsistency: false});
  const sessionDB = session.getDatabase(dbName);
  session.startTransaction({writeConcern: {w: "majority", wtimeout: 500}});

  const doc = {_id: "first in txn on primary " + primary};
  const doc2= {_id: "second in txn on primary " + primary};

  // Add two transaction oplog entries that will get replicated so the oldest active
  // is older than the last one in the oplog.
  assert.commandWorked(sessionDB.getCollection(collName).insert(doc));
  assert.commandWorked(sessionDB.getCollection(collName).insert(doc2));
  assert.commandWorked(sessionDB.getCollection(collName).insert({_id: "last in txn"}));

  jsTestLog("Committing transaction but fail on replication");
  let res = session.commitTransaction_forTesting();
  assert.commandFailedWithCode(res, ErrorCodes.WriteConcernFailed);

  jsTestLog("Wait for the secondary to block on fail point.");
  checkLog.contains(secondary, "stopReplProducerOnDocument fail point is enabled.");

  // Now the transaction should be in-progress on secondary.
  let txnTableEntry = getTxnTableEntry(newTestDB);
  assert.eq(txnTableEntry.state, "inProgress");

  // The latest oplog should be after the transaction startOptime.
  let latestOplog = getLatestOplogTimestamp(newTestDB);
  jsTestLog("latestOplog  : " + JSON.stringify(latestOplog));
  jsTestLog("txn timestamp: " + JSON.stringify(txnTableEntry));
  assert.eq(rs.compareOpTimes(txnTableEntry.startOpTime, latestOplog), -1);

  // Dump with oplog from the secondary.
  let targetPath = "activeTxnOplogTest";
  resetDbpath(targetPath);
  let rc = runMongoProgram("mongodump", '--host='+secondary.host, '--oplog', '--out='+targetPath);
  assert.eq(rc, 0, 'mongodump --oplog should succeed');

  // Check the first oplog.bson entry for the "first in txn" entry
  let tmpJSONFile = targetPath+"/oplog.bson.json";
  rc = _runMongoProgram("bsondump", "--outFile="+tmpJSONFile, targetPath+"/oplog.bson");
  assert.eq(rc, 0, "bsondump should succeed");
  let got = cat(tmpJSONFile).split("\n").filter(l => l.length);
  assert.strContains("first in txn on primary", got[0], "first oplog.bson entry has oldest active txn");

  // Shut down the cluster
  jsTestLog("Enable replication on the secondaries so that we can finish");
  assert.commandWorked(secondary.adminCommand({
    configureFailPoint: "stopReplProducerOnDocument",
    mode: "off",
  }));
  assert.commandWorked(
    replTest.nodes[2].adminCommand({configureFailPoint: 'stopReplProducer', mode: 'off'})
  );
  replTest.stopSet();
}());
