(function() {
  if (typeof getToolTest === 'undefined') {
    load('jstests/configs/plain_28.config.js');
  }
  load('jstests/libs/extended_assert.js');
  var assert = extendedAssert;

  var targetPath = "forceTableScanDumpTest";
  resetDbpath(targetPath);
  var toolTest = getToolTest('forceTableScanTest');
  var commonToolArgs = getCommonToolArguments();

  // IMPORTANT: make sure global `db` object is equal to this db, because
  // startParallelShell gives you no way of overriding db object.
  db = toolTest.db.getSiblingDB('foo'); // eslint-disable-line no-native-reassign

  db.dropDatabase();
  assert.eq(0, db.bar.count());

  // Run parallel shell to rapidly insert documents
  var shellArgs = ['print(\'starting insert\'); ' +
    (toolTest.authCommand || '') +
    'for (var i = 0; i < 10000; ++i) { ' +
    '  db.getSiblingDB(\'foo\').bar.insert({ x: i }); ' +
    '}',
  undefined,
  undefined,
  '--tls',
  '--tlsCertificateKeyFile=jstests/libs/client.pem',
  '--tlsCAFile=jstests/libs/ca.pem',
  '--tlsAllowInvalidHostnames'];
  var insertsShell = startParallelShell.apply(null, shellArgs);

  assert.lt.soon(250, db.bar.count.bind(db.bar), 'should have some documents');
  var countBeforeMongodump = db.bar.count();

  var dumpArgs = ['dump', '--forceTableScan']
    .concat(getDumpTarget(targetPath))
    .concat(commonToolArgs);
  assert.eq(toolTest.runTool.apply(toolTest, dumpArgs), 0,
    'mongodump --forceTableScan should succeed');

  // Wait for inserts to finish so we can then drop the database
  insertsShell();
  db.dropDatabase();
  assert.eq(0, db.bar.count());

  // --batchSize is necessary because config servers don't allow
  // batch writes, so if you've dumped the config DB you should
  // be careful to set this.
  var restoreArgs = ['restore',
    '--batchSize', '1',
    '--drop']
    .concat(getRestoreTarget(targetPath))
    .concat(commonToolArgs);
  assert.eq(toolTest.runTool.apply(toolTest, restoreArgs), 0,
    'mongorestore should succeed');
  assert.gte(db.bar.count(), countBeforeMongodump);
  assert.lt(db.bar.count(), 10000);

  toolTest.stop();
}());
