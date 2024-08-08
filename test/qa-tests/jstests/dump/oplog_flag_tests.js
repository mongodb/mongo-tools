(function() {
  if (typeof getToolTest === 'undefined') {
    load('jstests/configs/replset_28.config.js');
  }
  load('jstests/libs/extended_assert.js');
  var assert = extendedAssert;

  var targetPath = "oplogFlagDumpTest";
  resetDbpath(targetPath);
  var toolTest = getToolTest('oplogFlagTest');
  var commonToolArgs = getCommonToolArguments();

  if (!toolTest.isReplicaSet) {
    print('Nothing to do for testing oplogs without a replica set!');
    return assert(true);
  }

  // IMPORTANT: make sure global `db` object is equal to this db, because
  // startParallelShell gives you no way of overriding db object.
  db = toolTest.db.getSiblingDB('foo'); // eslint-disable-line no-native-reassign

  db.dropDatabase();
  assert.eq(0, db.bar.count());
  for (var i = 0; i < 1000; ++i) {
    db.bar.insert({x: i});
  }

  // Run parallel shell to rapidly insert documents
  var shellArgs = ['print(\'starting insert\'); ' +
    (toolTest.authCommand || '') +
    'for (var i = 1001; i < 10000; ++i) { ' +
    '  db.getSiblingDB(\'foo\').bar.insert({ x: i }); ' +
    '}',
  undefined,
  undefined,
  '--tls',
  '--tlsCertificateKeyFile=jstests/libs/client.pem',
  '--tlsCAFile=jstests/libs/ca.pem',
  '--tlsAllowInvalidHostnames'];
  var insertsShell = startParallelShell.apply(null, shellArgs);

  assert.lt.soon(1000, db.bar.count.bind(db.bar), 'should have more documents');
  var countBeforeMongodump = db.bar.count();

  var dumpArgs = ['dump', '--oplog']
    .concat(getDumpTarget(targetPath))
    .concat(commonToolArgs);
  var restoreArgs;

  if (toolTest.isReplicaSet) {
    // If we're running in a replica set, --oplog should give a snapshot by
    // applying ops from the oplog
    assert.eq(toolTest.runTool.apply(toolTest, dumpArgs), 0,
      'mongodump --oplog should succeed');

    // Wait for inserts to finish so we can then drop the database
    insertsShell();
    db.dropDatabase();
    assert.eq(0, db.bar.count());

    restoreArgs = ['restore']
      .concat(getRestoreTarget(targetPath))
      .concat(commonToolArgs);
    assert.eq(toolTest.runTool.apply(toolTest, restoreArgs), 0,
      'mongorestore should succeed');
    assert.gte(db.bar.count(), countBeforeMongodump);
    assert.lt(db.bar.count(), 10000);
  } else {
    // If we're running against a standalone or sharded cluster, mongodump
    // --oplog should fail immediately, without dumping any data
    assert(toolTest.runTool.apply(toolTest, dumpArgs) !== 0,
      'mongodump --oplog should fail fast on sharded and standalone');

    // Wait for inserts to finish so we can then drop the database
    insertsShell();
    db.dropDatabase();
    assert.eq(0, dbmongofiles_invalid.js.bar.count());

    restoreArgs = ['restore']
      .concat(getRestoreTarget(targetPath))
      .concat(commonToolArgs);
    assert.eq(toolTest.runTool.apply(toolTest, restoreArgs), 0,
      'mongorestore should succeed');
    // Shouldn't have dumped any documents
    assert.eq(db.bar.count(), 0);
  }

  toolTest.stop();
}());
