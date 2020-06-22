(function() {
  if (typeof getToolTest === 'undefined') {
    load('jstests/configs/plain_28.config.js');
  }
  load("jstests/libs/output.js");
  var toolTest = getToolTest('stat_auth');
  var db = toolTest.db.getSiblingDB('admin');

  db.createUser({
    user: "foobar",
    pwd: "foobar",
    roles: jsTest.adminUserRoles
  });

  assert(db.auth("foobar", "foobar"), "auth failed");

  var args = ["mongostat",
    "--host", "127.0.0.1:" + toolTest.port,
    "--rowcount", "1",
    "--authenticationDatabase", "admin",
    "--username", "foobar",
    "--ssl",
    "--sslPEMKeyFile=jstests/libs/client.pem",
    "--sslCAFile=jstests/libs/ca.pem",
    "--sslAllowInvalidHostnames"];

  var x = runMongoProgram.apply(null, args.concat("--password", "foobar"));
  assert.eq(x, exitCodeSuccess, "mongostat should exit successfully with foobar:foobar");

  x = runMongoProgram.apply(null, args.concat("--password", "wrong"));
  assert.eq(x, exitCodeFailure, "mongostat should exit with an error exit code with foobar:wrong");
}());
