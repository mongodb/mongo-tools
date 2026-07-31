/**
 * oplog_replay_local_rs.js
 *
 * This file tests mongorestore with --oplogReplay where the oplog file is in the 'oplog.rs'
 * collection of the 'local' database. This occurs when using a replica-set for replication.
 */
(function() {
  'use strict';
  if (typeof getToolTest === 'undefined') {
    load('jstests/configs/plain_28.config.js');
  }

  var commonToolArgs = getCommonToolArguments();
  var dumpTarget = 'oplog_replay_local_rs';

  var toolTest = getToolTest('oplog_replay_local_rs');

  // Set the test db to 'local' and collection to 'oplog.rs' to fake a replica set oplog
  var testDB = toolTest.db.getSiblingDB('local');
  var testColl = testDB['oplog.rs'];
  var testRestoreDB = toolTest.db.getSiblingDB('test');
  var testRestoreColl = testRestoreDB.op;
  resetDbpath(dumpTarget);

  var oplogSize = 100;
  testDB.createCollection('oplog.rs', {capped: true, size: 100000});

  // Create a fake oplog consisting of 100 inserts.
  for (var i = 0; i < oplogSize; i++) {
    var r = testColl.insert({
      ts: new Timestamp(0, i),
      op: "i",
      o: {_id: i, x: 'a' + i},
      ns: "test.op",
    });
    assert.eq(1, r.nInserted, "insert failed");
  }

  // The server sets the timestamp up to which the oplog is visible when it starts up,
  // and inserting into local.oplog.rs directly never advances it. As of MongoDB 9.0, a
  // forward scan of the oplog stops at that timestamp, so the entries we just wrote are
  // invisible to mongodump until the server restarts and sets the timestamp from the
  // last entry in the oplog. Restarting keeps the same port and dbpath, so the fake
  // oplog is still there afterwards.
  //
  // The suites that run this test get ToolTest from the mongo shell, not from
  // jstests/libs/servers_misc.js, so this cannot be moved into a ToolTest method there.
  MongoRunner.stopMongod(toolTest.m);
  toolTest.m = null;
  toolTest.db = null;
  toolTest.startDB();

  // Connections made before the restart are no longer usable.
  testRestoreDB = toolTest.db.getSiblingDB('test');
  testRestoreColl = testRestoreDB.op;

  // Dump the fake oplog.
  var ret = toolTest.runTool.apply(toolTest, ['dump',
    '--db', 'local',
    '-c', 'oplog.rs',
    '--out', dumpTarget]
    .concat(commonToolArgs));
  assert.eq(0, ret, "dump operation failed");

  // Create the test.op collection.
  testRestoreColl.drop();
  testRestoreDB.createCollection("op");
  assert.eq(0, testRestoreColl.count());

  // Replay the oplog from the provided oplog
  ret = toolTest.runTool.apply(toolTest, ['restore',
    '--oplogReplay',
    dumpTarget]
    .concat(commonToolArgs));
  assert.eq(0, ret, "restore operation failed");

  assert.eq(oplogSize, testRestoreColl.count(),
    "all oplog entries should be inserted");
  toolTest.stop();
}());
