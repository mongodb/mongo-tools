/**
 * no_primary_error_code.js
 *
 * This file tests TOOLS-690 where mongoimport returned exit code 0 when it should have returned
 * exit code 1 on error. The error stems from when mongos cannot find a primary. This file checks
 * that errors of type 'not master', 'unable to target', and 'Connection refused' yield error
 * code 1.
 */
(function() {
  'use strict';
  jsTest.log('Testing mongoimport when a sharded cluster has no primaries');

  var TOOLS_TEST_CONFIG = {};
  if (TestData.useTLS) {
    TOOLS_TEST_CONFIG = {
      tlsMode: "requireTLS",
      tlsCertificateKeyFile: "jstests/libs/client.pem",
      tlsCAFile: "jstests/libs/ca.pem",
      tlsAllowInvalidHostnames: "",
    };
  }
  var sh = new ShardingTest({
    name: 'no_primary_error_code',
    shards: {
      rs0: {
        nodes: 1,
        settings: {chainingAllowed: false},
      },
    },
    verbose: 0,
    mongos: 1,
    config: 1,
    configReplSetTestOptions: {
      settings: {chainingAllowed: false},
    },
    other: {
      configOptions: TOOLS_TEST_CONFIG,
      mongosOptions: TOOLS_TEST_CONFIG,
      shardOptions: TOOLS_TEST_CONFIG,
      nodeOptions: TOOLS_TEST_CONFIG,
    },
    rs: TOOLS_TEST_CONFIG,
  });

  var sslOptions = ['--ssl', '--sslPEMKeyFile=jstests/libs/client.pem',
    '--sslCAFile=jstests/libs/ca.pem', '--sslAllowInvalidHostnames'];

  // If we can't find a primary in 20 seconds than assume there are no more.
  var primary = sh.rs0.getPrimary(20000);

  jsTest.log('Stepping down ' + primary.host);

  try {
    primary.adminCommand({replSetStepDown: 300, force: true});
  } catch (e) {
    // Ignore any errors that occur when stepping down the primary.
    print('Error Stepping Down Primary: ' + e);
  }

  // Check that we catch 'not master'
  jsTest.log('All primaries stepped down, trying to import.');


  var ret = runMongoProgram.apply(null, ['mongoimport',
    '--file', 'jstests/import/testdata/basic.json',
    '--db', 'test',
    '--collection', 'noPrimaryErrorCode',
    '--host', sh.s0.host]
    .concat(sslOptions));
  assert.eq(ret, 1, 'mongoimport should fail with no primary');

  sh.getDB('test').dropDatabase();

  // Kill the replica set.
  sh.rs0.stopSet(15);

  // Check that we catch 'Connection refused'
  jsTest.log('All primaries died, trying to import.');

  ret = runMongoProgram.apply(null, ['mongoimport',
    '--file', 'jstests/import/testdata/basic.json',
    '--db', 'test',
    '--collection', 'noPrimaryErrorCode',
    '--host', sh.s0.host]
    .concat(sslOptions));
  assert.eq(ret, 1, 'mongoimport should fail with no primary');

  sh.stop();
}());
