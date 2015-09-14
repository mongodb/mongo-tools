mongocaputils
=============

utilities for handling mongo pcap files

license: ISC


tools
-----

mongocapcat - turns mongo pcap files into bson/json

installation:

```sh
$ go get github.com/gabrielrussell/mongocaputils/cmd/mongocapcat
```

usage:

```sh
$ tcpdump -i lo0 -w some_mongo_cap.pcap 'dst port 27017'
$ mongocapcat -f=some_mongo_capture.pcap
```

