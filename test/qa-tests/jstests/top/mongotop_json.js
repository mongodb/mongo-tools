// mongotop_json.js; ensure that running mongotop using the --json flag works as
// expected
var testName = 'mongotop_json';
(function() {
  jsTest.log('Testing mongotop --json option');
  load('jstests/top/util/mongotop_common.js');
  load('jstests/libs/extended_assert.js');
  var assert = extendedAssert;

  var runTests = function(topology, passthrough) {
    jsTest.log('Using ' + passthrough.name + ' passthrough');
    var t = topology.init(passthrough);
    var conn = t.connection();

    // clear the output buffer
    clearRawMongoProgramOutput();

    // ensure tool runs without error with --rowcount = 1
    assert.eq(runMongoProgram.apply(this, ['mongotop', '--port', conn.port, '--json', '--rowcount', 1].concat(passthrough.args)), 0, 'failed 1');
    assert(typeof JSON.parse(extractJSON(rawMongoProgramOutput())) === 'object', 'invalid JSON 1');

    // ensure tool runs without error with --rowcount > 1
    var rowcount = 5;
    clearRawMongoProgramOutput();
    var pid = startMongoProgramNoConnect.apply(this, ['mongotop', '--port', conn.port, '--json', '--rowcount', rowcount].concat(passthrough.args));
    assert.eq(waitProgram(pid), 0, 'failed 2');
    var outputLines;
    assert.eq.soon(rowcount, function() {
      outputLines = rawMongoProgramOutput().split('\n').filter(function(line) {
        return line.indexOf('sh'+pid+'| ') !== -1;
      });
      return outputLines.length;
    }, "expected " + rowcount + " top results");

    outputLines.forEach(function(line) {
      assert(typeof JSON.parse(extractJSON(line)) === 'object', 'invalid JSON 2');
    });

    t.stop();
  };

  // run with plain and auth passthroughs
  passthroughs.forEach(function(passthrough) {
    runTests(standaloneTopology, passthrough);
    runTests(replicaSetTopology, passthrough);
  });
}());
