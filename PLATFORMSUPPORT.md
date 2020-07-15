# Add New Platform support
In this tutorial, we will add a new platform support for Ubuntu2004-arm64.

1. check if the distro ubuntu2004-arm64-small exists in https://evergreen.mongodb.com/distros. If not please file ticket like https://jira.mongodb.org/browse/BUILD-9123 and ask build team to add the distro.
 Also be aware the distro with alias name might be created as well, for example ubuntu2004-arm64-test in this case. However that's not done automatically by the build team for now. You can try with the other if one is not available.

2. Since this is an ARM platform, add the following config in the `ARM Buildvariants` section in `common.yml`
```
- name: ubuntu2004-arm64
  display_name: ZAP ARM64 Ubuntu 20.04
  run_on:
    - ubuntu2004-arm64-small
  stepback: false
  batchtime: 10080 # weekly
  expansions:
    <<: *mongod_ssl_startup_args
    <<: *mongo_ssl_startup_args
    <<: *mongod_tls_startup_args
    <<: *mongo_tls_startup_args
    mongo_os: "ubuntu2004"
    mongo_edition: "targeted"
    mongo_arch: "aarch64"
    build_tags: "failpoints ssl"
    resmoke_use_tls: _tls
    excludes: requires_mmap_available,requires_large_ram,requires_mongo_24,requires_mongo_26,requires_mongo_30
    resmoke_args: -j 2
    edition: ssl
    USE_SSL: "true"
  tasks: *ubuntu2004_arm64_tasks
```

To set up enterprise version testing, add the extra fields to `expansions`
```
    mongo_edition: "enterprise"
    edition: enterprise
```

3. Add following in `release/platform.go` to support release script
```
	{
		Name:  "ubuntu2004",
		Arch:  "arm64",
		OS:    OSLinux,
		Pkg:   PkgDeb,
		Repos: []string{RepoEnterprise},
	},
```

4. Add support in `etc/repo-config.yml`
```
  - name: ubuntu2004
    type: deb
    code_name: "bionic"
    edition: org
    bucket: repo.mongodb.org
    component: multiverse
    architectures:
      - amd64
    repos:
      - apt/ubuntu/dists/bionic/mongodb-org
```
If enterprise version release is needed, add an enterprise version config in `Enterprise Repos:` section as well.

After all done, create an evergreen patch build to run through all the tasks on the new platform to make sure all the toolkits are available.

