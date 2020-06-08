/**
 * Verify the AWS IAM Auth works with temporary credentials from sts:AssumeRole
 */

load("lib/aws_e2e_lib.js");

(function() {
  "use strict";

  const ASSUMED_ROLE = "arn:aws:sts::557821124784:assumed-role/authtest_user_assume_role/*";

  function getAssumeCredentials() {
    const config = readSetupJson();

    const env = {
      AWS_ACCESS_KEY_ID: config["iam_auth_assume_aws_account"],
      AWS_SECRET_ACCESS_KEY: config["iam_auth_assume_aws_secret_access_key"],
    };

    const role_name = config["iam_auth_assume_role_name"];

    const python_command = getPython3Binary() +
      ` -u lib/aws_assume_role.py --role_name=${role_name} > creds.json`;

    const ret = runShellCmdWithEnv(python_command, env);
    assert.eq(ret, 0, "Failed to assume role on the current machine");

    const result = cat("creds.json");
    jsTestLog("result: " + result);
    try {
      return JSON.parse(result);
    } catch (e) {
      jsTestLog("Failed to parse: " + result);
      throw e;
    }
  }

  const credentials = getAssumeCredentials();
  var TOOLS_TEST_CONFIG = {
    auth: '',
    setParameter: {
      authenticationMechanisms: 'MONGODB-AWS,SCRAM-SHA-256'
    },
  };

  mkdir("/data/db/jstests_tool_assume_role/");
  var toolTest = new ToolTest("assume_role", TOOLS_TEST_CONFIG);
  toolTest.port = 33333;
  toolTest.startDB();

  var adminDB = toolTest.db.getSiblingDB('admin');
  adminDB.createUser({
    user: "bob",
    pwd: "pwd123",
    roles: ['__system'],
  });
  assert(adminDB.auth("bob", "pwd123"));
  jsTest.log("after bob auth");

  const externalDB = toolTest.db.getSiblingDB('$external');
  assert.commandWorked(externalDB.runCommand({createUser: ASSUMED_ROLE, roles:[{role: 'read', db: "aws_test_db"}]}));
  assert.commandWorked(externalDB.runCommand({grantRolesToUser: ASSUMED_ROLE, roles: [{role: 'readWrite', db: "mongodump_test_db"}]}));
  assert(externalDB.auth({
    user: credentials["AccessKeyId"],
    pwd: credentials["SecretAccessKey"],
    awsIamSessionToken: credentials["SessionToken"],
    mechanism: 'MONGODB-AWS'
  }));
}());
