load("jstests/configs/standard_dump_targets.config.js");

/* exported getToolTest */
var getToolTest;

(function() {
  getToolTest = function(name) {
    var TOOLS_TEST_CONFIG = {};
    if (TestData.useTLS) {
      TOOLS_TEST_CONFIG = {
        tlsMode: "requireTLS",
        tlsCertificateKeyFile: "jstests/libs/client.pem",
        tlsCAFile: "jstests/libs/ca.pem",
        tlsAllowInvalidHostnames: "",
      };
    }
    var toolTest = new ToolTest(name, TOOLS_TEST_CONFIG);

    var shardingTest = new ShardingTest({name: name,
      shards: 2,
      verbose: 0,
      mongos: 3,
      other: {
        chunksize: 1,
        enableBalancer: 0,
        configOptions: TOOLS_TEST_CONFIG,
        mongosOptions: TOOLS_TEST_CONFIG,
        shardOptions: TOOLS_TEST_CONFIG,
      },
      rs: TOOLS_TEST_CONFIG,
    });
    shardingTest.adminCommand({enablesharding: name});

    toolTest.m = shardingTest.s0;
    toolTest.db = shardingTest.getDB(name);
    toolTest.port = shardingTest.s0.port;

    var oldStop = toolTest.stop;
    toolTest.stop = function() {
      shardingTest.stop();
      oldStop.apply(toolTest, arguments);
    };

    toolTest.isSharded = true;

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
