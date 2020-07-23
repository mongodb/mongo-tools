load("jstests/configs/standard_dump_targets.config.js");

/* exported getToolTest */
var getToolTest;
var TOOLS_TEST_CONFIG = {
  binVersion: '',
};

(function() {
  if (TestData.useTLS) {
    TOOLS_TEST_CONFIG = {
      binVersion: '',
      tlsMode: "requireTLS",
      tlsCertificateKeyFile: "jstests/libs/server.pem",
      tlsCAFile: "jstests/libs/ca.pem",
      tlsAllowInvalidHostnames: "",
    };
  }

  getToolTest = function(name) {
    var toolTest = new ToolTest(name, TOOLS_TEST_CONFIG);
    toolTest.startDB();
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
