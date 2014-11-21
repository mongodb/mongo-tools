var getToolTest;

(function() {
  getToolTest = function(name) {
    var toolTest = new ToolTest(name, null);

    var shardingTest = new ShardingTest(name, 2, 0, 3, { chunksize: 1, enableBalancer: 0 });
    shardingTest.adminCommand({ enablesharding: name });

    toolTest.m = shardingTest.s0;
    toolTest.db = shardingTest.getDB(name);
    toolTest.port = shardingTest.s0.port;

    var oldStop = toolTest.stop;
    toolTest.stop = function() {
      shardingTest.stop();
      oldStop.apply(toolTest, arguments);
    };

    return toolTest;
  };
})();

var getCommonToolArguments = function() {
  return [];
};
