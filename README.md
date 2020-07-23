MongoDB Tools
===================================

 - **bsondump** - _display BSON files in a human-readable format_
 - **mongoimport** - _Convert data from JSON, TSV or CSV and insert them into a collection_
 - **mongoexport** - _Write an existing collection to CSV or JSON format_
 - **mongodump/mongorestore** - _Dump MongoDB backups to disk in .BSON format, or restore them to a live database_
 - **mongostat** - _Monitor live MongoDB servers, replica sets, or sharded clusters_
 - **mongofiles** - _Read, write, delete, or update files in [GridFS](http://docs.mongodb.org/manual/core/gridfs/)_
 - **mongotop** - _Monitor read/write activity on a mongo server_


Report any bugs, improvements, or new feature requests at https://jira.mongodb.org/browse/TOOLS

Building Tools
---------------

We currently build the tools with Go version 1.12, other Go versions may work but they are untested. `go get` will not 
work; you need to clone the repository to build it. Be sure to clone the repository into your Go workspace inside your 
$GOPATH.

You can find additional information on GOPATHs here:
https://github.com/golang/go/wiki/GOPATH
https://github.com/golang/go/wiki/SettingGOPATH

```
mkdir -p $GOPATH/src/github.com/mongodb
cd $GOPATH/src/github.com/mongodb
git clone https://github.com/mongodb/mongo-tools
cd mongo-tools
```

To use build/test scripts in the repo, you *MUST* set GOROOT to your Go root directory.

```
export GOROOT=/usr/local/go
```

### Quick build

The `build.sh` script builds all the tools, placing them in the `bin`
directory.  Pass any build tags (like `ssl` or `sasl`) as additional command
line arguments.

```
./build.sh
./build.sh ssl
./build.sh ssl sasl
```

### Manual build

Source `set_goenv.sh` and run the `set_goenv` function to setup your GOPATH and
architecture-specific configuration flags:

```
. ./set_goenv.sh
set_goenv
```

Pass tags to the `go build` command as needed in order to build the tools with
support for SSL and/or SASL. For example:

```
mkdir bin
go build -o bin/mongoimport mongoimport/main/mongoimport.go
go build -o bin/mongoimport -tags ssl mongoimport/main/mongoimport.go
go build -o bin/mongoimport -tags "ssl sasl" mongoimport/main/mongoimport.go
```

Contributing
---------------
See our [Contributor's Guide](CONTRIBUTING.md).

Documentation
---------------
See the MongoDB packages [documentation](https://docs.mongodb.org/database-tools/).

For documentation on older versions of the MongoDB, reference that version of the [MongoDB Server Manual](docs.mongodb.com/manual):

- [MongoDB 4.2 Tools](https://docs.mongodb.org/v4.2/reference/program)
- [MongoDB 4.0 Tools](https://docs.mongodb.org/v4.0/reference/program)
- [MongoDB 3.6 Tools](https://docs.mongodb.org/v3.6/reference/program)

Adding New Platforms Support
---------------
See our [Adding New Platform Support Guide](PLATFORMSUPPORT.md).