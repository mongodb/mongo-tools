// oplog1.js

// very basic test for mongooplog
// need a lot more, but test that it functions at all

t = new ToolTest( "oplog1" );

db = t.startDB();

output = db.output

doc = { x : 17, _id: 5 };

db.oplog.insert( { ts : new Timestamp() , "op" : "i" , "ns" : output.getFullName() , "o" : doc } );

assert.eq( 0 , output.count() , "before" )

t.runTool( "oplog" , "--oplogns" , db.getName() + ".oplog" , "--from" , "127.0.0.1:" + t.port , "-vv" );

assert.eq( 1 , output.count() , "after" );

assert.eq( doc , output.findOne() , "after check" );

t.stop();


