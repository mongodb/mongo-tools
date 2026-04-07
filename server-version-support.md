# Add support for new server version

This document describes how to add support for a new version of the MongoDB server. In the best
case, the only thing you'll need to do is to add the new version to the test matrix. Depending on
how those tests look, though, you might need to make some other changes. This document describes the
process of adding support for MongoDB 8.3, but the process will be similar for other versions going
forward.

## Add to test matrix

The most important thing to do is to update [common.yml](./common.yml) to add the new version. You
can search for the previous version, and copy those blocks and change the server version. For
example:

```yml
- name: integration-8.3
  tags: ["8.3"]
  commands:
    - func: "fetch source"
    - command: expansions.update
    - func: "download mongod and shell"
      vars:
        mongo_version: "8.3.0-rc4"
    - func: "start mongod"
      vars:
        USE_TLS: "true"
    - func: "wait for mongod to be ready"
      vars:
        USE_TLS: "true"
    - func: "run make target"
      vars:
        target: build
    - func: "run make target"
      vars:
        target: test:integration -ssl=true ${testArgs}
```

You'll want to make sure to update the version in:

- the name of the task
- the task tags
- the `mongo_version` variable
- (if present) the `load_libs_version` variable

We need to release the tools before the relevant server version has actually happened, which means
that the `mongo_version` will likely be a pre-release version like `8.3.0-alpha6` or `8.3.0-rc0`.

There are also a few other things to do in the evergreen config:

- Update the `max_server_version` variable in the global variables section
- Add the new version tag to each of the build variants; for the `8.3` tag, each variant will need
  to be preceded with a dot, so you'll need `name: ".8.3"`

To make the tests work, you also need to update the `maxServerVersion` variable in
[release/download/download_server.go](./release/download/download_server.go), so that the tests can
actually download the versions they need. This is needed so that the `latest` tests use the newest
version we support.

### Adjust load libs

For the legacy JS tests running on server versions 8.1+, we need to shim some code that was removed
from the server repo. The shim code for each version lives in
[test/shell_common/libs/](./test/shell_common/libs/), in a filename called `load_libs-<VERSION>.js`
(where the version is something like `8.3`). You can start by copying the file for the previous
version.

You may need to add/adjust code in the load-libs files if the tests fail; that usually happens
because some test code has been removed from the server repo. For the 8.3 release, it was sufficient
to go back to the 8.2 server code and pull the relevant code and copy it into our shim.

(This process is admittedly not great; we're in the process of rewriting these JS tests into Go.
If/when we finish doing that, then we won't need any of this code at all, since they'll just be
normal integration tests.)

## Write code for new version

This will look different depending on what has changed in the server. In the best case, you won't
need to do anything at all and the tests will continue to work with the new server version. If they
don't, you may need to tweak some things in our code. Remember that we still need to support older
versions in the code though, so if behavior has changed, the library code will need to switch on the
version or some capability.

# Prepare for release

When the tests are all passing, the final and most important step is to update the
`linuxRepoVersionsStable` variable in [release/release.go](./release/release.go). Including the
version here will cause the release code to notify
[Barque](https://docs.devprod.prod.corp.mongodb.com/barque), which is the DevProd service
responsible for packaging the tools for various Linux repos (Debian, RHEL, etc.).

This **must** be done before releasing the tools. For historical reasons, the release of any
major/minor version of the server is blocked by releasing a version of the tools with that support.
Adding the version to the `linuxRepoVersionsStable` variable and releasing the tools ensures that
they are published to the appropriate Linux repos, and unblocks the server release process.
