(function() {
  load('jstests/libs/extended_assert.js');
  var assert = extendedAssert;

  if (typeof getToolTest === 'undefined') {
    load('jstests/configs/plain_28.config.js');
  }

  if (dump_targets === "archive") {
    print('skipping test incompatable with archiving');
    return assert(true);
  }

  // Tests running mongorestore with --restoreDbUsersAndRoles, with
  // no users or roles in the dump.

  jsTest.log('Testing running mongorestore with --restoreDbUsersAndRoles with'+
            ' no users or roles in the dump');

  var toolTest = getToolTest('empty_users_and_roles');
  var commonToolArgs = getCommonToolArguments();

  // run the restore with no users or roles. it should succeed, but create no
  // users or roles
  toolTest.runTool.apply(toolTest, ['restore',
    '--db', 'test',
    '--restoreDbUsersAndRoles']
    .concat(getRestoreTarget('jstests/restore/testdata/blankdb'))
    .concat(commonToolArgs));

  assert.strContains.soon("cannot find users or roles to restore with --restoreDbUsersAndRoles", rawMongoProgramOutput,
    "restore without users should not succeed");

  // success
  toolTest.stop();

}());
