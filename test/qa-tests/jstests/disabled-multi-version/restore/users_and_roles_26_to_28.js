// This test requires mongo 2.6.x, and mongo 3.0.0 releases
// @tags: [requires_mongo_26, requires_mongo_30]
(function() {

  load("jstests/configs/standard_dump_targets.config.js");
  // skip tests requiring wiredTiger storage engine on pre 2.8 mongod
  if (TestData && TestData.storageEngine === 'wiredTiger') {
    return;
  }

  // Tests using mongorestore with --restoreDbUsersAndRoles, using a dump from
  // a 2.6 mongod and restoring to a 2.8+ mongod

  jsTest.log('Testing running mongorestore with --restoreDbUsersAndRoles,'+
        ' restoring a 2.6 dump to a 2.8 mongod');

  var toolTest = new ToolTest('users_and_roles_26_to_28', {binVersion: '2.6'});
  resetDbpath(toolTest.dbpath);
  toolTest.startDB('foo');

  // where we'll put the dump
  var dumpTarget = 'users_and_roles_26_to_28_dump';
  resetDbpath(dumpTarget);

  // the db we'll be using
  var testDB = toolTest.db.getSiblingDB('test');

  // create some users and roles on the database
  testDB.createUser({
    user: 'userOne',
    pwd: 'pwdOne',
    roles: [{role: 'read', db: 'test'}],
  });
  testDB.createRole({
    role: 'roleOne',
    privileges: [{
      resource: {db: 'test', collection: ''},
      actions: ['find'],
    }],
    roles: [],
  });
  testDB.createUser({
    user: 'userTwo',
    pwd: 'pwdTwo',
    roles: [{role: 'roleOne', db: 'test'}],
  });

  // insert some data
  var data = [];
  for (var i = 0; i < 10; i++) {
    data.push({_id: i});
  }
  testDB.data.insertMany(data);
  // sanity check the insertion worked
  assert.eq(10, testDB.data.count());

  // dump the data
  var ret = toolTest.runTool.apply(toolTest, ['dump', '-vv', '--db', 'test', '--dumpDbUsersAndRoles']
    .concat(getDumpTarget(dumpTarget)));
  assert.eq(0, ret);

  // drop the database, users, and roles
  testDB.dropDatabase();
  testDB.dropAllUsers();
  testDB.dropAllRoles();

  // restart the mongod as latest
  stopMongod(toolTest.port);
  toolTest.m = null;
  toolTest.db = null;
  delete toolTest.options.binVersion;
  resetDbpath(toolTest.dbpath);
  toolTest.startDB('foo');

  // refresh the db reference
  testDB = toolTest.db.getSiblingDB('test');

  // restore the data, specifying --restoreDBUsersAndRoles
  ret = toolTest.runTool.apply(toolTest, ['restore', '-vv', '--db', 'test', '--restoreDbUsersAndRoles']
    .concat(getRestoreTarget(dumpTarget+'/test')));
  assert.eq(0, ret);

  // make sure the data was restored
  assert.eq(10, testDB.data.count());
  for (i = 0; i < 10; i++) {
    assert.eq(1, testDB.data.count({_id: i}));
  }

  // make sure the users were restored
  var users = testDB.getUsers();
  assert.eq(2, users.length);
  assert(users[0].user === 'userOne' || users[1].user === 'userOne');
  assert(users[0].user === 'userTwo' || users[1].user === 'userTwo');

  // make sure the role was restored
  var roles = testDB.getRoles();
  assert.eq(1, roles.length);
  assert.eq('roleOne', roles[0].role);

  // success
  toolTest.stop();

}());
