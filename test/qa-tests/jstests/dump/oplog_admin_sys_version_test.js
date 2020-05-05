(function() {
  if (typeof getToolTest === 'undefined') {
    load('jstests/configs/replset_28.config.js');
  }
  load('jstests/libs/extended_assert.js');
  var assert = extendedAssert;

  var targetPath = 'dump_oplog_uuid_test';
  resetDbpath(targetPath);

  var TOOLS_TEST_CONFIG = {};
  if (TestData.useTLS) {
    TOOLS_TEST_CONFIG = {
      tlsMode: "requireTLS",
      tlsCertificateKeyFile: "jstests/libs/client.pem",
      tlsCAFile: "jstests/libs/ca.pem",
      tlsAllowInvalidHostnames: "",
    };
  }
  var toolTest = getToolTest('oplogUUIDTest', TOOLS_TEST_CONFIG);
  var commonToolArgs = getCommonToolArguments();

  if (!toolTest.isReplicaSet) {
    print('Nothing to do for testing oplog rollover without a replica set!');
    return assert(true);
  }

  var db = toolTest.db.getSiblingDB('foo');

  var bigObj = {x: ''};
  while (bigObj.x.length < 1024) {
    bigObj.x += 'bacon';
  }

  var dumpArgs = ['mongodump',
    '--oplog',
    '--failpoints', 'PauseBeforeDumping',
    '--host', toolTest.m.host]
    .concat(getDumpTarget(targetPath))
    .concat(commonToolArgs);

  db.bar1.insert(bigObj);

  for (var i = 0; i < 2000; ++i) {
    db.bar0.insert(bigObj);
  }

  var pid = startMongoProgramNoConnect.apply(null, dumpArgs);

  var adminDB = toolTest.db.getSiblingDB('admin');
  r = adminDB.getCollection("system.version").insert({'dummy': true});
  assert.eq(r.nInserted, 1, 'insert into system.version: ' + r);

  assert(waitProgram(pid) !== 0,
    'mongodump --oplog during admin.system.version change should fail');

  toolTest.stop();
}());
