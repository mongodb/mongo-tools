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

We currently build the tools with Go version 1.15. Other Go versions may work but they are untested.

Using `go get` to directly build the tools will not work. To build them, it's recommended to first clone this repository:

```
git clone https://github.com/mongodb/mongo-tools
cd mongo-tools
```

Then run `./make build` to build all the tools, placing them in the `bin` directory inside the repository.

You can also build a subset of the tools using the `-pkgs` option. For example, `./make build -pkgs=mongodump,mongorestore` builds only `mongodump` and `mongorestore`.

To use the build/test scripts in this repository, you **_must_** set GOROOT to your Go root directory. This may depend on how you installed Go.

```
export GOROOT=/usr/local/go
```

Updating Dependencies
---------------
Starting with version 100.3.1, the tools use `go mod` to manage dependencies. All dependencies are listed in the `go.mod` file and are directly vendored in the `vendor` directory.

In order to make changes to dependencies, you first need to change the `go.mod` file. You can manually edit that file to add/update/remove entries, or you can run the following in the repository directory:

```
go mod edit -require=<package>@<version>  # for adding or updating a dependency
go mod edit -droprequire=<package>        # for removing a dependency
```

Then run `go mod vendor -v` to reconstruct the `vendor` directory to match the changed `go.mod` file.

Optionally, run `go mod tidy -v` to ensure that the `go.mod` file matches the `mongo-tools` source code.

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

Vendoring the Change into Server Repo
---------------
See our [Vendor the Change into Server Repo](SERVERVENDORING.md).
