// @tags: [requires_unstable]

(function() {
  if (typeof getToolTest === 'undefined') {
    load('jstests/configs/plain_28.config.js');
  }

  var toolTest = getToolTest('sni');
  if (!toolTest.useSSL) {
    return;
  }

  var port = allocatePort();
  var m = startMongod('-v', '--port', port, '--dbpath', MongoRunner.dataPath + 'sni_dump');

  var dumpTarget = 'sni_dump';
  resetDbpath(dumpTarget);

  var args = ["mongodump", "--host", "127.0.0.1:" + port];

  var x = runMongoProgram.apply(null, args);

  assert.eq(x, 0, "mongodump should exit successfully");

  var admin = m.getDB("admin");

  logs = admin.adminCommand({getLog: "global"});

  sniRegexp = /SNI server name \[(.*)\]/;

  var sni_line_count = 0;
  var missing_sni_host = false;
  logs.log.forEach(function(line) {
    print(line);
    var match = sniRegexp.exec(line);
    if (match !== null) {
      sni_line_count++;
      print("match: "+match);
      if (match[0]==="") {
        missing_sni_host = true;
      }
    }
  });

  assert.neq(sni_line_count, 0, "Server is producing SNI server name logs");
  assert.neq(missing_sni_host, true, "all SNI server name logs contain host names");

  resetDbpath(dumpTarget);

  toolTest.stop();
}());
