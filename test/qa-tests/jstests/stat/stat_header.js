(function() {
  if (typeof getToolTest === 'undefined') {
    load('jstests/configs/plain_28.config.js');
  }
  load("jstests/libs/mongostat.js");

  var toolTest = getToolTest('stat_header');

  clearRawMongoProgramOutput();
  x = runMongoProgram("mongostat", "--port", toolTest.port, "--rowcount", 1);
  var match = rawMongoProgramOutput().split("\n").some(function(i) {
    return i.match(/^sh\d+\| insert/);
  });
  assert(match, "normally a header appears");

  clearRawMongoProgramOutput();
  x = runMongoProgram("mongostat", "--port", toolTest.port, "--rowcount", 1, "--noheaders");
  match = rawMongoProgramOutput().split("\n").some(function(i) {
    return i.match(/^sh\d+\| insert/);
  });
  assert(!match, "--noheaders suppress the header");

  toolTest.stop();
}());
