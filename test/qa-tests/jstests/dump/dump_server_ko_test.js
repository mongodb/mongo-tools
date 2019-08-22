if (typeof getToolTest === 'undefined') {
  load('jstests/configs/plain_28.config.js');
}

(function() {
  var targetPath = "dump_server_ko_test";
  resetDbpath(targetPath);
  var toolTest = getToolTest('ServerKOTest');
  var commonToolArgs = getCommonToolArguments();

  // IMPORTANT: make sure global `db` object is equal to this db, because
  // startParallelShell gives you no way of overriding db object.
  db = toolTest.db.getSiblingDB('foo'); // eslint-disable-line no-native-reassign

  db.dropDatabase();
  assert.eq(0, db.bar.count());

  var data = [];
  for (var i = 0; i < 1000; ++i) {
    data.push({x: i});
  }
  db.bar.insertMany(data);

  // Run parallel shell that waits for mongodump to start and then
  // brings the server down.
  if (toolTest.isReplicaSet && !toolTest.authCommand) {
    // shutdownServer() is flakey on replica sets because of localhost
    // exception, so do a stepdown instead
    return assert(false, 'Can\'t run shutdownServer() on replica set ' +
      'without auth!');
  }
  // On sharded and standalone, kill the server
  var koShell = startParallelShell(
    'sleep(1000); ' +
      (toolTest.authCommand || '') +
      'db.getSiblingDB(\'admin\').shutdownServer({ force: true });');

  var dumpArgs = ['dump',
    '--db', 'foo',
    '--collection', 'bar',
    '--query', '{ "$where": "sleep(25); return true;" }']
    .concat(getDumpTarget(targetPath))
    .concat(commonToolArgs);

  assert(toolTest.runTool.apply(toolTest, dumpArgs) !== 0,
    'mongodump should crash gracefully when remote server dies');

  var possibleErrors = [
    'error reading from db',
    'error reading collection',
    'connection closed',
    'Interrupted',
    'interrupted',
  ];
  assert.soon(function() {
    var output = rawMongoProgramOutput();
    return possibleErrors
      .map(output.indexOf, output)
      .some(function(index) {
        return index !== -1;
      });
  }, 'mongodump crash should output one of the correct error messages');

  // Swallow the exit code for the shell per SERVER-25777.
  koShell();

  toolTest.stop();
}());
