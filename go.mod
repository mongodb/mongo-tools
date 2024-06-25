module github.com/mongodb/mongo-tools

go 1.21

require (
	github.com/aws/aws-sdk-go v1.53.11
	github.com/craiggwilson/goke v0.0.0-20240206162536-b1c58122d943
	github.com/google/go-cmp v0.6.0
	github.com/jessevdk/go-flags v1.5.0
	github.com/mitchellh/go-wordwrap v1.0.1
	github.com/nsf/termbox-go v1.1.1
	github.com/pkg/errors v0.9.1
	github.com/smartystreets/goconvey v1.8.1
	github.com/stretchr/testify v1.9.0
	github.com/urfave/cli/v2 v2.27.2
	github.com/youmark/pkcs8 v0.0.0-20201027041543-1326539a0a0a
	// Later versions remove a package the tools use, so we're sticking with
	// this older version for now.
	go.mongodb.org/mongo-driver v1.11.9
	golang.org/x/crypto v0.23.0
	golang.org/x/exp v0.0.0-20240529005216-23cca8864a10
	golang.org/x/mod v0.17.0
	gopkg.in/tomb.v2 v2.0.0-20161208151619-d5d1b5820637
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/cpuguy83/go-md2man/v2 v2.0.4 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/gopherjs/gopherjs v1.17.2 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/jtolds/gls v4.20.0+incompatible // indirect
	github.com/klauspost/compress v1.17.8 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/montanaflynn/stats v0.7.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/smarty/assertions v1.15.0 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.1.2 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/xrash/smetrics v0.0.0-20240521201337-686a1a2994c1 // indirect
	golang.org/x/net v0.25.0 // indirect
	golang.org/x/sync v0.7.0 // indirect
	golang.org/x/sys v0.20.0 // indirect
	golang.org/x/term v0.20.0 // indirect
	golang.org/x/text v0.15.0 // indirect
)
