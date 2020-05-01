// mongotop_stress.js; ensure that running mongotop, even when the server is
// under heavy load, works as expected
var testName = 'mongotop_stress';
load('jstests/top/util/mongotop_common.js');

(function() {
  jsTest.log('Testing mongotop\'s performance under load');

  var runTests = function(topology, passthrough) {
    jsTest.log('Using ' + passthrough.name + ' passthrough');
    var t = topology.init(passthrough);
    var conn = t.connection();
    db = conn.getDB('foo'); // eslint-disable-line no-native-reassign

    // concurrently insert documents into thousands of collections
    var stressShell = 'print(\'starting read/write stress test\');' +
    '   if (\'' + passthrough.name + '\' === \'auth\')' +
    '       db.getSiblingDB(\'admin\').auth(\'' + authUser + '\', \'' + authPassword + '\');' +
    '   var dbName = (Math.random() + 1).toString(36).substring(7); ' +
    '   var clName = (Math.random() + 1).toString(36).substring(7); ' +
    '   for (var i = 0; i < 10000; ++i) { ' +
    '       db.getSiblingDB(dbName).getCollection(clName).find({ x: i }).forEach();' +
    '       sleep(1);' +
    '       db.getSiblingDB(dbName).getCollection(clName).insert({ x: i });' +
    '       sleep(1);' +
    '   }';

    var shells = [];
    var shellArgs = [stressShell,
      undefined,
      undefined,
      '--tls',
      '--tlsCertificateKeyFile=jstests/libs/client.pem',
      '--tlsCAFile=jstests/libs/ca.pem',
      '--tlsAllowInvalidHostnames'];
    for (var i = 0; i < 10; ++i) {
      shells.push(startParallelShell.apply(null, shellArgs));
    }

    var sslOptions = ['--ssl', '--sslPEMKeyFile=jstests/libs/client.pem',
      '--sslCAFile=jstests/libs/ca.pem', '--sslAllowInvalidHostnames'];

    // wait a bit for the stress to kick in
    sleep(5000);
    jsTest.log('Current operation(s)');
    printjson(db.currentOp());

    // ensure tool runs without error
    clearRawMongoProgramOutput();
    assert.eq(runMongoProgram.apply(this, ['mongotop',
      '--port', conn.port, '--json', '--rowcount', 1]
      .concat(passthrough.args)
      .concat(sslOptions)), 0, 'failed 1');

    // Wait for all the shells to finish per SERVER-25777.
    shells.forEach(function(join) {
      join();
    });

    t.stop();
  };

  // run with plain and auth passthroughs
  passthroughs.forEach(function(passthrough) {
    runTests(standaloneTopology, passthrough);
    runTests(replicaSetTopology, passthrough);
  });
}());
