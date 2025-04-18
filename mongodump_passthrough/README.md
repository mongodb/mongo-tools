mongo-tools/mongodump_passthrough contains evergreen .yml files to support resmoke passthrough
testing of mongodump+mongorestore.

This started out as a copy of the files from mongosync/evergreen, but has minor changes to cope with
running from mongo-tools, and to avoid conflicts with the mongo-tools/common.yml

These files should be included from mongo-tools/common.yml

We have to be careful to avoid clashes with any task of function names used in common.yml
