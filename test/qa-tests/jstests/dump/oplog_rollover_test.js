(function() {
  if (typeof getToolTest === 'undefined') {
    load('jstests/configs/replset_28.config.js');
  }
  load('jstests/libs/extended_assert.js');
  var assert = extendedAssert;

  var targetPath = 'dump_oplog_rollover_test';
  resetDbpath(targetPath);
  var toolTest = getToolTest('oplogRolloverTest');
  var commonToolArgs = getCommonToolArguments();

  if (!toolTest.isReplicaSet) {
    print('Nothing to do for testing oplog rollover without a replica set!');
    return assert(true);
  }

  // IMPORTANT: make sure global `db` object is equal to this db, because
  // startParallelShell gives you no way of overriding db object.
  db = toolTest.db.getSiblingDB('foo'); // eslint-disable-line no-native-reassign

  assert.eq.soon(0, function() {
    db.dropDatabase();
    return db.bar.count();
  }, 'foo.bar should be empty');

  // Run parallel shell that inserts large documents as fast as possible. Each
  // document should be > 1MB, and thus every few writes should overflow a 5MB
  // oplog, which is the oplog size that these tests are designed for.
  startParallelShell(
    'print(\'starting insert\'); ' +
    (toolTest.authCommand || '') +
    'var longString = \'\'; ' +
    'while (longString.length < 1024 * 1024) { longString += \'bacon\'; } ' +
    'for (var i = 0; i < 1000; ++i) { ' +
    '  db.getSiblingDB(\'foo\').bar.insert({ x: longString }); ' +
    '}'
  );

  // Give some time for inserts to actually start before dumping. In order for
  // an oplog rollover to occur we need many documents to already be inserted. If the
  // delay below is not long enough to trigger an oplog rollover, then increase the
  // required document count.
  assert.lte.soon(175, db.bar.count.bind(db.bar), 'Took longer than 120 seconds'
      + ' to insert 175 documents before mongodump.', 120*1000, 1000);

  var dumpArgs = ['dump', '--oplog']
    .concat(getDumpTarget(targetPath))
    .concat(commonToolArgs);

  assert(toolTest.runTool.apply(toolTest, dumpArgs) !== 0,
    'mongodump --oplog should crash sensibly on oplog rollover');

  var expectedError = 'oplog overflow: mongodump was unable to capture all ' +
    'new oplog entries during execution';
  assert.strContains.soon(expectedError, rawMongoProgramOutput,
    'mongodump --oplog failure should output the correct error message');

  toolTest.stop();
}());
