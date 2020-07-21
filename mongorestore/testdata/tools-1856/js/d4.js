// Dataset 4: Many (1,000) arrays

load("dropall.js")
var db = connect('127.0.0.1:27017/memtest');

var array = [1]
for (let i = 0; i < 1000; i++) {
    let doc = {};
    // each document will have 10 arrays
    for (let j = 0; j < 10; j++){
        let name = i + "_" + j;
        doc[name] = array;
    }
    db.test.insert(doc);
}
