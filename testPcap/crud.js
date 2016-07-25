// simple crud test
// CREATE
for (var i=0; i<20000; i++) {
  db.bench.insert({a:i});
}
// READ
db.bench.find({a:{$mod:[4, 0]}}).count();
// UPDATE
db.bench.updateMany({a:{$mod:[4,0]}}, {$set:{b:true}});
// DELETE
db.bench.deleteMany({b:true});

// must end with 20000*3/4 = 15000 documents
