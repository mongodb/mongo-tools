module github.com/mongodb/mongo-tools

go 1.14

require (
	github.com/3rf/mongo-lint v0.0.0-20140604191638-3550fdcf1f43
	github.com/aws/aws-sdk-go v1.34.28
	github.com/craiggwilson/goke v0.0.0-20200309222237-69a77cdfe646
	github.com/google/go-cmp v0.5.2
	github.com/gopherjs/gopherjs v0.0.0-20190430165422-3e4dfb77656c // indirect
	github.com/jessevdk/go-flags v1.4.0 // indirect
	github.com/jtolds/gls v4.20.0+incompatible // indirect
	github.com/klauspost/compress v1.10.1 // indirect
	github.com/mattn/go-colorable v0.1.7 // indirect
	github.com/mattn/go-runewidth v0.0.4 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/mongodb/mongo-tools-common v4.0.18+incompatible
	github.com/nsf/termbox-go v0.0.0-20160718140619-0723e7c3d0a3
	github.com/smartystreets/assertions v0.0.0-20160205033931-287b4346dc4e // indirect
	github.com/smartystreets/goconvey v1.6.1-0.20160205033552-bf58a9a12912
	github.com/xdg/stringprep v1.0.1-0.20180714160509-73f8eece6fdc // indirect
	github.com/youmark/pkcs8 v0.0.0-20181117223130-1be2e3e5546d // indirect
	go.mongodb.org/mongo-driver v1.4.2
	golang.org/x/sys v0.0.0-20200302150141-5c8b2ff67527 // indirect
	gopkg.in/mgo.v2 v2.0.0-00010101000000-000000000000
	gopkg.in/tomb.v2 v2.0.0-20140626144623-14b3d72120e8
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

replace gopkg.in/mgo.v2 => github.com/10gen/mgo v0.0.0-20181212170345-8c133fd1d0fc
