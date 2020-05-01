(function() {
  jsTest.log('Testing running import with upserts');

  var toolTest = new ToolTest('import_repl');

  var TOOLS_TEST_CONFIG = {
    tlsMode: "requireTLS",
    tlsCertificateKeyFile: "jstests/libs/client.pem",
    tlsCAFile: "jstests/libs/ca.pem",
    tlsAllowInvalidHostnames: "",
  };
  var replset1 = new ReplSetTest({
    nodes: 3,
    name: 'importtest',
    nodeOptions: TOOLS_TEST_CONFIG
  });
  replset1.startSet();
  replset1.initiate();

  var primary = replset1.getPrimary();
  var secondary = replset1.getSecondary();

  var db = primary.getDB('import_repl_test');

  var sslOptions = ['--ssl', '--sslPEMKeyFile=jstests/libs/client.pem',
    '--sslCAFile=jstests/libs/ca.pem', '--sslAllowInvalidHostnames'];

  // trying to write to the secondary should fail
  assert.neq(runMongoProgram.apply(this, ['mongoimport',
    '--file', 'jstests/import/testdata/basic.json',
    '--db', db.getName(),
    '--collection', db.c.getName(),
    '--host', secondary.host, "--legacy"]
    .concat(sslOptions)), 0,
  "writing to secondary should fail");

  assert.eq(db.c.count(), 0, 'database not empty');

  // now import using the primary
  assert.eq(runMongoProgram.apply(this, ['mongoimport',
    '--file', 'jstests/import/testdata/basic.json',
    '--db', db.getName(),
    '--collection', db.c.getName(),
    '--host', primary.host, "--legacy"]
    .concat(sslOptions)), 0,
  "writing to primary should succeed");

  assert.neq(db.c.count(), 0, 'database unexpectedly empty on primary');

  db.dropDatabase();

  // import using the secondary but include replset name, should succeed
  assert.eq(runMongoProgram.apply(this, ['mongoimport',
    '--file', 'jstests/import/testdata/basic.json',
    '--db', db.getName(),
    '--collection', db.c.getName(),
    '--host', replset1.name + "/" + secondary.host, "--legacy"]
    .concat(sslOptions)), 0,
  "writing to secondary with replset name should succeed");

  assert.neq(db.c.count(), 0, 'database unexpectedly empty on secondary');

  toolTest.stop();
}());
