// mongotop_sharded.js; ensure that running mongotop against a sharded cluster
// fails with a useful error message
var testName = 'mongotop_sharded';
var expectedError = 'cannot run mongotop against a mongos';
load('jstests/top/util/mongotop_common.js');

(function() {
  jsTest.log('Testing mongotop against sharded cluster');

  var verifyOutput = function(shellOutput) {
    jsTest.log('shell output: ' + shellOutput);
    assert(shellOutput.match(expectedError), 'error message must appear at least once');
    shellOutput.split('\n').forEach(function(line) {
      // check the displayed error message
      assert.neq(line.match(shellOutputRegex), null, 'must only have shell output lines');
      assert.neq(line.match(expectedError), null, 'unexpeced error message');
    });
  };

  var executeProgram = function(args) {
    clearRawMongoProgramOutput();
    var pid = startMongoProgramNoConnect.apply(this, args);
    var exitCode = waitProgram(pid);
    var prefix = 'sh'+pid+'| ';
    var output = rawMongoProgramOutput();
    output = output.split('\n').filter(function(line) {
      return line.indexOf(prefix) === 0;
    }).join('\n');
    return {
      exitCode: exitCode,
      output: output,
    };
  };

  var runTests = function(topology, passthrough) {
    jsTest.log('Using ' + passthrough.name + ' passthrough');
    var t = topology.init(passthrough);
    var conn = t.connection();

    // getting the version should work without error
    assert.eq(runMongoProgram.apply(this, ['mongotop', '--port', conn.port, '--version'].concat(passthrough.args)), 0, 'failed 1');

    // getting the help text should work without error
    assert.eq(runMongoProgram.apply(this, ['mongotop', '--port', conn.port, '--help'].concat(passthrough.args)), 0, 'failed 2');

    // anything that runs against the mongos server should fail
    var result = executeProgram(['mongotop', '--port', conn.port].concat(passthrough.args));
    assert.neq(result.exitCode, 0, 'expected failure against a mongos');
    verifyOutput(result.output);

    result = executeProgram(['mongotop', '--port', conn.port, '2'].concat(passthrough.args));
    assert.neq(result.exitCode, 0, 'expected failure against a mongos');
    verifyOutput(result.output);

    t.stop();
  };

  // run with plain and auth passthroughs
  passthroughs.forEach(function(passthrough) {
    runTests(shardedClusterTopology, passthrough);
  });
}());
