load("jstests/configs/standard_dump_targets.config.js");

/* exported getToolTest */
var getToolTest;

(function() {
  var TOOLS_TEST_CONFIG = {
    tlsMode: "requireTLS",
    tlsCertificateKeyFile: "jstests/libs/client.pem",
    tlsCAFile: "jstests/libs/ca.pem",
    tlsAllowInvalidHostnames: "",
  };

  getToolTest = function(name) {
    var toolTest = new ToolTest(name, TOOLS_TEST_CONFIG);

    var replTest = new ReplSetTest({
      name: 'tool_replset',
      nodes: 3,
      oplogSize: 5,
      nodeOptions: TOOLS_TEST_CONFIG
    });

    replTest.startSet();
    replTest.initiate();
    var master = replTest.getPrimary();

    toolTest.m = master;
    toolTest.db = master.getDB(name);
    toolTest.port = replTest.getPort(master);

    var oldStop = toolTest.stop;
    toolTest.stop = function() {
      replTest.stopSet();
      oldStop.apply(toolTest, arguments);
    };

    toolTest.isReplicaSet = true;

    return toolTest;
  };
}());

/* exported getCommonToolArguments */
var getCommonToolArguments = function() {
  return ['--ssl',
    '--sslPEMKeyFile=jstests/libs/client.pem',
    '--sslCAFile=jstests/libs/ca.pem',
    '--sslAllowInvalidHostnames'];
};
