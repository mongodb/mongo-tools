(function() {

  load("jstests/configs/standard_dump_targets.config.js");
  // Tests running mongorestore and restoring users with a nonempty temp
  // users collection.

  jsTest.log('Testing restoring users with a nonempty temp users collection.'+
        ' The restore should fail');

  var TOOLS_TEST_CONFIG = {};
  if (TestData.useTLS) {
    TOOLS_TEST_CONFIG = {
      tlsMode: "requireTLS",
      tlsCertificateKeyFile: "jstests/libs/client.pem",
      tlsCAFile: "jstests/libs/ca.pem",
      tlsAllowInvalidHostnames: "",
    };
  }
  var toolTest = new ToolTest('nonempty_temp_users', TOOLS_TEST_CONFIG);
  var commonToolArgs = getCommonToolArguments();
  toolTest.startDB('foo');

  // where we'll put the dump
  var dumpTarget = 'nonempty_temp_users_dump';
  resetDbpath(dumpTarget);

  // the admin db
  var adminDB = toolTest.db.getSiblingDB('admin');

  // create a user on the admin database
  adminDB.createUser({
    user: 'adminUser',
    pwd: 'password',
    roles: [{role: 'read', db: 'admin'}],
  });

  // dump the data
  var ret = toolTest.runTool.apply(toolTest, ['dump']
    .concat(getDumpTarget(dumpTarget))
    .concat(commonToolArgs));
  assert.neq(1, ret);

  // clear out the user
  adminDB.dropAllUsers();

  // insert into the tempusers collection
  adminDB.tempusers.insert({_id: 'corruption'});

  // restore the data. It should succeed
  ret = toolTest.runTool.apply(toolTest, ['restore']
    .concat(getRestoreTarget(dumpTarget))
    .concat(commonToolArgs));
  assert.neq(1, ret);

  // success
  toolTest.stop();

}());
