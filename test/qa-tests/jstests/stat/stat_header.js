(function() {
  if (typeof getToolTest === 'undefined') {
    load('jstests/configs/plain_28.config.js');
  }
  load('jstests/libs/output.js');
  load('jstests/libs/extended_assert.js');
  var assert = extendedAssert;

  var toolTest = getToolTest('stat_header');
  var commonToolArgs = getCommonToolArguments();

  function outputIncludesHeader() {
    return rawMongoProgramOutput()
      .split("\n").some(function(line) {
        return line.match(/^sh\d+\| insert/);
      });
  }

  clearRawMongoProgramOutput();
  x = runMongoProgram.apply(this, ["mongostat", "--port", toolTest.port, "--rowcount", 1]
    .concat(commonToolArgs));
  assert.soon(outputIncludesHeader, "normally a header appears");

  clearRawMongoProgramOutput();
  x = runMongoProgram.apply(this, ["mongostat", "--port", toolTest.port, "--rowcount", 1, "--noheaders"]
    .concat(commonToolArgs));
  assert.eq.soon(false, outputIncludesHeader, "--noheaders suppresses the header");

  toolTest.stop();
}());
