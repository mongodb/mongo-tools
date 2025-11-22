module github.com/mongodb/mongo-tools

go 1.25

require (
	github.com/craiggwilson/goke v0.0.0-20240206162536-b1c58122d943
	github.com/deckarep/golang-set/v2 v2.6.0
	github.com/google/go-cmp v0.6.0
	github.com/jessevdk/go-flags v1.5.0
	github.com/mitchellh/go-wordwrap v1.0.1
	github.com/nsf/termbox-go v1.1.1
	github.com/pkg/errors v0.9.1
	github.com/smartystreets/goconvey v1.8.1
	github.com/stretchr/testify v1.10.0
	github.com/urfave/cli/v2 v2.27.2
	github.com/youmark/pkcs8 v0.0.0-20240726163527-a2c0da244d78
	// Later versions remove a package the tools use, so we're sticking with
	// this older version for now.
	go.mongodb.org/mongo-driver v1.17.3
	golang.org/x/crypto v0.45.0 // indirect
	golang.org/x/exp v0.0.0-20250128182459-e0ece0dbea4c
	golang.org/x/mod v0.29.0
	golang.org/x/term v0.37.0
	gopkg.in/tomb.v2 v2.0.0-20161208151619-d5d1b5820637
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.17.0
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.8.0
	github.com/aws/aws-sdk-go-v2 v1.36.5
	github.com/aws/aws-sdk-go-v2/config v1.29.17
	github.com/aws/aws-sdk-go-v2/feature/s3/manager v1.17.81
	github.com/aws/aws-sdk-go-v2/service/s3 v1.81.0
	github.com/ccoveille/go-safecast/v2 v2.0.0
	github.com/google/uuid v1.6.0
	github.com/samber/lo v1.49.1
	golang.org/x/sync v0.18.0
)

require (
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.10.0 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.3.2 // indirect
	github.com/aws/aws-sdk-go v1.53.11 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.6.11 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.17.70 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.16.32 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.3.36 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.6.36 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.3 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.3.36 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.12.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.7.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.12.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.18.17 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.25.5 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.30.3 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.34.0 // indirect
	github.com/aws/smithy-go v1.22.4 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.4 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.2 // indirect
	github.com/golang/snappy v0.0.4 // indirect
	github.com/gopherjs/gopherjs v1.17.2 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/jtolds/gls v4.20.0+incompatible // indirect
	github.com/klauspost/compress v1.17.8 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/montanaflynn/stats v0.7.1 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/smarty/assertions v1.15.0 // indirect
	github.com/xdg-go/pbkdf2 v1.0.0 // indirect
	github.com/xdg-go/scram v1.1.2 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	github.com/xrash/smetrics v0.0.0-20240521201337-686a1a2994c1 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
)
