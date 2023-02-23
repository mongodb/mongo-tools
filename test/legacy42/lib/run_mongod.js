/**
 * This file has to be loaded to mongo shell before loading js tests.
 *
 * This file overrides MongoRunner.runMongod function in mongo shell:
 *  - Removes "journal" option before starting mongod to make the deprecated mongo shell compatible with MongoDB 6.1+
 *
 * MongoRunner.runMongod Starts a mongod instance.
 */
var oldRunMongod = MongoRunner.runMongod;

MongoRunner.runMongod = function(opts) {
    print("Running MongoRunner.runMongod overriden in mongo-tools");

    if (opts != undefined && opts.journal != undefined) {
        delete opts.journal;
    }

    return oldRunMongod(opts);
};
