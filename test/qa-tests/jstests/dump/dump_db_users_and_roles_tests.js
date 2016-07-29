if (typeof getToolTest === 'undefined') {
  load('jstests/configs/plain_28.config.js');
}

(function() {
  var targetPath = 'dump_db_users_and_roles_test';
  resetDbpath(targetPath);
  var toolTest = getToolTest('dumpDbUsersAndRolesTest');
  var commonToolArgs = getCommonToolArguments();
  var db = toolTest.db.getSiblingDB('foo');

  // Dump with no users
  var dumpArgs = ['dump', '--db', 'foo', '--dumpDbUsersAndRoles']
    .concat(getDumpTarget(targetPath))
    .concat(commonToolArgs);
  assert.eq(toolTest.runTool.apply(toolTest, dumpArgs), 1,
    'mongodump should fail with --dumpDbUsersAndRoles and no users');

  db.dropDatabase();
  assert.eq(0, db.bar.count());

  db.getSiblingDB('baz').dropDatabase();
  assert.eq(0, db.getSiblingDB('baz').bar.count());

  // Create roles
  db.createRole({
    role: 'taco',
    privileges: [
      {resource: {db: 'foo', collection: ''}, actions: ['find']},
    ],
    roles: [],
  });
  db.getSiblingDB('baz').createRole({
    role: 'bacon',
    privileges: [
      {resource: {db: 'baz', collection: ''}, actions: ['find']},
    ],
    roles: [],
  });

  // And users with those roles
  db.createUser({
    user: 'baconator',
    pwd: 'bacon',
    roles: [{role: 'taco', db: 'foo'}],
  });
  db.getSiblingDB('baz').createUser({
    user: 'eggs',
    pwd: 'bacon',
    roles: [{role: 'bacon', db: 'baz'}],
  });

  // mongodump should fail when --dumpDbUsersAndRoles is specified but
  // --db isn't
  dumpArgs = ['dump', '--dumpDbUsersAndRoles']
    .concat(getDumpTarget(targetPath))
    .concat(commonToolArgs);
  assert(toolTest.runTool.apply(toolTest, dumpArgs) !== 0,
    'mongodump should fail when --dumpDbUsersAndRoles is specified without ' +
    '--db');

  // Running mongodump with `--db foo --dumpDbUsersAndRoles` should dump the
  // associated users
  resetDbpath(targetPath);
  dumpArgs = ['dump', '--db', 'foo', '--dumpDbUsersAndRoles']
    .concat(getDumpTarget(targetPath))
    .concat(commonToolArgs);
  assert.eq(toolTest.runTool.apply(toolTest, dumpArgs), 0,
    'mongodump should succeed with `--db foo --dumpDbUsersAndRoles`');
  db.dropDatabase();
  db.getSiblingDB('baz').dropDatabase();
  db.getSiblingDB('admin').system.users.remove({user: 'baconator'});
  db.getSiblingDB('admin').system.users.remove({user: 'eggs'});
  db.getSiblingDB('admin').system.roles.remove({role: 'taco'});
  db.getSiblingDB('admin').system.roles.remove({role: 'bacon'});

  restoreArgs = ['restore', "--db", "foo", '--restoreDbUsersAndRoles']
    .concat(getRestoreTarget(targetPath + "/foo"))
    .concat(commonToolArgs);
  assert.eq(toolTest.runTool.apply(toolTest, restoreArgs), 0,
    'mongorestore should succeed');
  var c = db.getSiblingDB('admin').system.roles.find();
  assert(!!c);

  // Should have restored only the user that was in the 'foo' db
  assert(!!db.getUser('baconator'));
  assert(!db.getSiblingDB('baz').getUser('eggs'));
  // And only the role that was defined on the 'foo' db
  assert(!!db.getRole('taco'));
  assert(!db.getSiblingDB('baz').getRole('bacon'));

  toolTest.stop();
}());
