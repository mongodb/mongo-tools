var getToolTest;

(function() {
  var TOOLS_TEST_CONFIG = {
    binVersion: '2.6',
  };

  getToolTest = function(name) {
    var toolTest = new ToolTest(name, TOOLS_TEST_CONFIG);
    toolTest.startDB();
    return toolTest;
  };
})();

var getCommonToolArguments = function() {
  return [];
};
