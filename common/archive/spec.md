# The Mongodump archive format specification

The mongodump archive format contains metadata about collections and data dumped from those collections. Data from multiple collections can be interleaved so multiple threads can dump data from different collections into the archive concurrently.

Here is the definition in BNF-like syntax:

```ebnf
archive = magic-number , 
          header , 
          *collection-metadata , 
          terminator-bytes , 
          *(namespace-segment | namespace-eof) ;

magic-number = 0x6de29981 ; (* little-endian representation of 0x8199e26d *)

header = document ;       

collection-metadata = document ;  

terminator-bytes = 0xffffffff ;

namespace-segment = namespace-header , namespace-data , terminator-bytes ;

namespace-data = +document ;

namespace-eof = eof-header , terminator-bytes ;

namespace-header = document ;

eof-header = document ;
```

## Explanatory notes

`document` is a BSON document as defined by the [BSON Spec](https://bsonspec.org). The fields of each document are defined here:

- `header`:
    ```
    {
        int32 concurrent_collections, 
        string version, 
        string server_version, 
        string tool_version
    }
    ```
    - `concurrent_collections` - the number of collections dumped concurrently by mongodump as set by the `--numParallelCollections` options. Mongorestore will choose the larger of `concurrent_collections` and `--numParallelCollections` to set the number of collections to restore in parallel.
    - `version` - the archive format version. Currently there is only one version, `"0.1"`.
    - `server_version` - the MongoDB version of the source database.
    - `tool_version` - the version of mongodump that created the archive.

- `collection-metadata`:
    ```
    {
        string db, 
        string collection, 
        string metadata, 
        int32 size
    }
    ```
    - `db` - databse name.
    - `collection` - collection name.
    - `metadata` - the collection metadata (including options, index definitions, and collection type) encoded in canonical [Extended JSON v2](https://docs.mongodb.com/manual/reference/mongodb-extended-json/). This is the same data written to `metadata.json` files.
    - `size` - the total uncompressed size of the collection in bytes.
- `namespace-data`: One or more BSON documents from the collection. The collection's documents can be split across multiple segments.
- `namespace-header`:
    ```
    {
        string db,
        string collection,
        bool EOF,
        int64 CRC
    }
    ```
    - `db` - databse name.
    - `collection` - collection name.
    - `EOF` - always `false`.
    - `CRC` - always `0`.
- `eof-header`:
    ```
    {
        string db,
        string collection,
        bool EOF,
        int64 CRC
    }
    ```
    - `db` - databse name.
    - `collection` - collection name.
    - `EOF` - always `true`.
    - `CRC` - the CRC-64-ECMA of all documents in the namespace (across all `namespace-segment`s).
