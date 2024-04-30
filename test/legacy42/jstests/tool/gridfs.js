// tests gridfs with a sharded fs.chunks collection.
// @tags: [requires_sharding]

var test = new ShardingTest({shards: 3, mongos: 1, config: 1, verbose: 2, other: {chunkSize: 1}});

var mongos = test.s0;

var filename = "mongod";  // A large file we are guaranteed to have
if (_isWindows())
    filename += ".exe";

function testGridFS(name) {
    var d = mongos.getDB(name);

    // this function should be called on a clean db
    assert.eq(d.name.files.count(), 0);
    assert.eq(d.fs.chunks.count(), 0);

    var exitCode = MongoRunner.runMongoTool("mongofiles",
                                            {
                                                port: mongos.port,
                                                db: name,
                                            },
                                            "put",
                                            filename);
    assert.eq(0, exitCode, "mongofiles failed to upload '" + filename + "' into a sharded cluster");

    assert.eq(d.fs.files.count(), 1);
    var fileObj = d.fs.files.findOne();
    print("fileObj: " + tojson(fileObj));

    var dlFilename = filename + ".dl";
    var exitCode = MongoRunner.runMongoTool("mongofiles",
                                            {
                                                port: mongos.port,
                                                db: name,
                                            },
                                            "get",
                                            filename,
                                            "--local",
                                            dlFilename);
    assert.eq(0, exitCode, "mongofiles failed to download '" + filename);

    assert.eq(md5sumFile(filename), md5sumFile(dlFilename), "the downloaded file has same content as the original");
}

print('\n\n\t**** unsharded ****\n\n');
name = 'unsharded';
testGridFS(name);

print('\n\n\t**** sharded db, unsharded collection ****\n\n');
name = 'sharded_db';
test.adminCommand({enablesharding: name});
testGridFS(name);

print('\n\n\t**** sharded collection on files_id ****\n\n');
name = 'sharded_files_id';
test.adminCommand({enablesharding: name});
test.adminCommand({shardcollection: name + '.fs.chunks', key: {files_id: 1}});
testGridFS(name);

print('\n\n\t**** sharded collection on files_id,n ****\n\n');
name = 'sharded_files_id_n';
test.adminCommand({enablesharding: name});
test.adminCommand({shardcollection: name + '.fs.chunks', key: {files_id: 1, n: 1}, unique: true});
testGridFS(name);

test.stop();
