// Dataset 0: Control

load("dropall.js")
var db = connect('127.0.0.1:27017/memtest');

for (let i = 0; i < 1000; i++){
    let doc = {};
    doc[i] = i
    db.test.insert(doc);
}
