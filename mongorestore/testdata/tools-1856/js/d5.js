// Dataset 8: Deeply nested arrays (100 layers of nesting)

load("dropall.js")
var db = connect('127.0.0.1:27017/memtest');

function nestarray (depth) {
    var arr = [];
    var cur = arr;
    for (let i = 0; i < depth; i++) {
        cur[0] = i;
        let tmp = [];
        cur[1] = {'a': tmp};
        cur = tmp;
    }
    cur[0] = depth;
    cur[1] = {'a': depth+1};
    return arr;
}

// 49 because each array is a field within a document, meaning the total amount
// of document nesting is 100. The 100-depth nesting limit appears to apply to
// arrays as well, and counts both layers of document and array nesting.
var array = nestarray(49)
for (let i = 0; i < 1000; i++) {
    let doc = {};
    doc[i] = array;
    db.test.insert(doc);
}
