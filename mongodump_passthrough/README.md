mongo-tools/mongodump_passthrough contains evergreen .yml files to
support resmoke passthrough testing of mongodump+mongorestore.

This is split into separate files to match the file structure of the mongosync repo
for this new code, but without refactoring the old top-level common.yml
file for other mongo-tools testing.

These files should be included from mongo-tools/common.yml

We have to be careful to avoid clashes with any task of function names used 
in common.yml
