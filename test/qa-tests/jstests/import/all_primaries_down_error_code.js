/**
 * all_primaries_down_error_code.js
 *
 * This file tests TOOLS-690 where mongoimport returned exit code 0 when it should have returned
 * exit code 1 on error. The error stems from when mongos cannot find a primary.
 * This file tests that errors of type 'could not contact primary for replica set' return exit
 * code 1.
 */
(function() {
  'use strict';
  jsTest.log('Testing mongoimport when a sharded cluster has no primaries');

  var TOOLS_TEST_CONFIG = {
    tlsMode: "requireTLS",
    tlsCertificateKeyFile: "jstests/libs/client.pem",
    tlsCAFile: "jstests/libs/ca.pem",
    tlsAllowInvalidHostnames: "",
  };

  var sh = new ShardingTest({
    name: 'all_primaries_down_error_code',
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

  // Make sure there is no primary in any replica set.
  for (var rs of sh._rs) {
    var ranOutOfPrimaries = false;
    for (var i = 0; i < rs.nodes.length + 1; i++) {
      var primary;
      try {
        // If we can't find a primary in 20 seconds than assume there are no more.
        primary = rs.test.getPrimary(20000);
      } catch (e) {
        print('Error Finding Primary: ' + e);
        ranOutOfPrimaries = true;
        break;
      }

      jsTest.log('Stepping down ' + primary.host);

      try {
        primary.adminCommand({replSetStepDown: 300, force: true});
      } catch (e) {
        // Ignore any errors that occur when stepping down the primary.
        print('Error Stepping Down Primary: ' + e);
      }
    }
    // Assert that we left due to running out of primaries and not due to the loop ending.
    assert(ranOutOfPrimaries,
      'Had to kill primary more times than number of nodes in the replset.');
  }

  // Check that we catch 'could not contact primary for replica set'
  jsTest.log('All primaries stepped down, trying to import.');

  var ret = runMongoProgram('mongoimport',
    '--file', 'jstests/import/testdata/basic.json',
    '--db', 'test',
    '--collection', 'noPrimaryErrorCode',
    '--host', sh.s0.host,
    '--ssl',
    '--sslPEMKeyFile=jstests/libs/client.pem',
    '--sslCAFile=jstests/libs/ca.pem', '--sslAllowInvalidHostnames');
  assert.eq(ret, 1, 'mongoimport should fail with no primary');

  sh.stop();
}());
