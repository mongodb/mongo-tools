// Wrap whole file in a function to avoid polluting the global namespace
(function () {
  let oldRunMongod = MongoRunner.runMongod;
  MongoRunner.runMongod = function(opts) {
    print("MongoRunner.runMongod overriden in mongo-tools");

    if (opts !== undefined && opts.journal !== undefined) {
      delete opts.journal;
    }

    return oldRunMongod(opts);
  };
}());
