# Database Tools Changelog

## 100.7.2

_Released 2023-05-30_

We are pleased to announce version 100.7.2 of the MongoDB Database Tools.

This release fixes an issue with installing Database Tools on RHEL aarch64 architecture. 

The Database Tools are available on the [MongoDB Download Center](https://www.mongodb.com/try/download/database-tools).
Installation instructions and documentation can be found on [docs.mongodb.com/database-tools](https://docs.mongodb.com/database-tools/).
Questions and inquiries can be asked on the [MongoDB Developer Community Forum](https://developer.mongodb.com/community/forums/tags/c/developer-tools/49/database-tools).
Please make sure to tag forum posts with `database-tools`.
Bugs and feature requests can be reported in the [Database Tools Jira](https://jira.mongodb.org/browse/TOOLS) where a list of current issues can be found.

### Bug

<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-3309'>TOOLS-3309</a>] -         Fix release issue on RHEL ARM architecture
</li>
</ul>

## 100.7.1

_Released 2023-05-24_

We are pleased to announce version 100.7.1 of the MongoDB Database Tools.

This release fixes a few bugs and adds downloads for macOS 11 on ARM
as well as RedHat Enterprise Linux 9 (x86 and ARM) and Amazon Linux
2023 (x86 and ARM).

Downloads were compiled with Go 1.19.9.

The Database Tools are available on the [MongoDB Download Center](https://www.mongodb.com/try/download/database-tools).
Installation instructions and documentation can be found on [docs.mongodb.com/database-tools](https://docs.mongodb.com/database-tools/).
Questions and inquiries can be asked on the [MongoDB Developer Community Forum](https://developer.mongodb.com/community/forums/tags/c/developer-tools/49/database-tools).
Please make sure to tag forum posts with `database-tools`.
Bugs and feature requests can be reported in the [Database Tools Jira](https://jira.mongodb.org/browse/TOOLS) where a list of current issues can be found.

### Bug

<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2598'>TOOLS-2598</a>] -         Tools improperly parse multi-certs inside client certificate file
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-3203'>TOOLS-3203</a>] -         mongodump fails because it can’t query system.sharding_ddl_coordinators collection
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-3273'>TOOLS-3273</a>] -         Validation added in 100.7.0 prevents Atlas proxy from running &quot;mongodump&quot;
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-3279'>TOOLS-3279</a>] -         Test suite segfaults in some failure cases
</li>
</ul>

### Task

<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2722'>TOOLS-2722</a>] -         Add MacOS 11.0 ARM to Tools
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-3052'>TOOLS-3052</a>] -         Add Amazon Linux 2023 ARM to Tools
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-3054'>TOOLS-3054</a>] -         Add RHEL9 ARM to Tools
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-3062'>TOOLS-3062</a>] -         Add Amazon Linux 2023 to Tools
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-3276'>TOOLS-3276</a>] -         Skip columnstore indexes tests in mongodump and mongorestore if error is NotImplemented
</li>
</ul>

## 100.7.0

_Released 2023-03-01_

We are pleased to announce version 100.7.0 of the MongoDB Database Tools.

This release adds tests against MongoDB 6.3. Highlights include new tests for [Column Store Indexes](https://www.mongodb.com/products/column-store-indexes), updating the minimum Go version to 1.19, fixing a bug that caused the Tools to ignore a password supplied via a prompt. Several build failures are also fixed in this version.

The Database Tools are available on the [MongoDB Download Center](https://www.mongodb.com/try/download/database-tools).
Installation instructions and documentation can be found on [docs.mongodb.com/database-tools](https://docs.mongodb.com/database-tools/).
Questions and inquiries can be asked on the [MongoDB Developer Community Forum](https://developer.mongodb.com/community/forums/tags/c/developer-tools/49/database-tools).
Please make sure to tag forum posts with `database-tools`.
Bugs and feature requests can be reported in the [Database Tools Jira](https://jira.mongodb.org/browse/TOOLS) where a list of current issues can be found.

### Bug

<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-3243'>TOOLS-3243</a>] -         Tools produce error about missing password after prompting for a password
</li>
</ul>

### Epic
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-3235'>TOOLS-3235</a>] -         Tools 6.3 Support
</li>
</ul>

### Task

<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-3169'>TOOLS-3169</a>] -         Upgrade Go to 1.19
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-3202'>TOOLS-3202</a>] -         Fix legacy-jstests failure with latest Server (6.1)
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-3208'>TOOLS-3208</a>] -         Investigate test failures in HEAD and make more tickets as needed
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-3229'>TOOLS-3229</a>] -         Ignore admin database in dump/restore for atlasProxy
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-3241'>TOOLS-3241</a>] -         Fix flaky TestFailDuringResharding test
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-3247'>TOOLS-3247</a>] -         Remove mongo-tools support for ZAP PPC64LE Ubuntu 16.04 
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-3248'>TOOLS-3248</a>] -         Fix TestRestoreTimeseriesCollections for server 6.3+
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-3249'>TOOLS-3249</a>] -         Remove mongo-tools support for server version 3.4 on MacOS
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-3250'>TOOLS-3250</a>] -         Fix aws-auth task failures
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-3251'>TOOLS-3251</a>] -         Update common.yml to run tests with 6.3
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-3252'>TOOLS-3252</a>] -         Test support for Columnstore Indexes
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-3255'>TOOLS-3255</a>] -         Fix qa-tests-3.4
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-3256'>TOOLS-3256</a>] -         Make the push tasks only run on git tags
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-3257'>TOOLS-3257</a>] -         Override deprecated mongo shell functions to fix qa-tests-latest
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-3259'>TOOLS-3259</a>] -         Remove 6.3 tests on `ZAP s390x RHEL 7.2` and `ZAP PPC64LE RHEL 8.1`
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-3260'>TOOLS-3260</a>] -         Fix qa-tests on Debian 11
</li>
</ul>

## 100.6.1

_Released 2022-11-03_

We are pleased to announce version 100.6.1 of the MongoDB Database Tools.

This release fixes an issue where users without permissions to read `config.system.preimages` could not run mongodump. This release also fixes issues with installing the Database Tools on Amazon Linux 2 aarch64 and RHEL 8.3 s390x. The Database Tools are now available on RHEL 9.0 x86. We also made several improvements to our testing and release infrastructure.

The Database Tools are available on the [MongoDB Download Center](https://www.mongodb.com/try/download/database-tools).
Installation instructions and documentation can be found on [docs.mongodb.com/database-tools](https://docs.mongodb.com/database-tools/).
Questions and inquiries can be asked on the [MongoDB Developer Community Forum](https://developer.mongodb.com/community/forums/tags/c/developer-tools/49/database-tools).
Please make sure to tag forum posts with `database-tools`.
Bugs and feature requests can be reported in the [Database Tools Jira](https://jira.mongodb.org/browse/TOOLS) where a list of current issues can be found.

### Bug

- [TOOLS-3176 - Ignore config.system.preimages namespace](https://jira.mongodb.org/browse/TOOLS-3176)
- [TOOLS-3179 - Mongo Tools Enterprise Z series package is being published to Community repo](https://jira.mongodb.org/browse/TOOLS-3179)
- [TOOLS-3204 - Tools should not be packaged as arm64 on aarch64 platforms](https://jira.mongodb.org/browse/TOOLS-3176)

### Task
- [TOOLS-2956 - Use the new notary service to notarize MacOS builds](https://jira.mongodb.org/browse/TOOLS-2956)
- [TOOLS-3056 - Add RHEL9 x86 to Tools](https://jira.mongodb.org/browse/TOOLS-3056)
- [TOOLS-3185 - Fix TestMongorestoreTxns failure on all platforms when run against latest Server](https://jira.mongodb.org/browse/TOOLS-3185)
- [TOOLS-3201 - Add a .snyk file to exclude tests from snyk code analysis](https://jira.mongodb.org/browse/TOOLS-3201)

## 100.6.0

_Released 2022-08-19_

We are pleased to announce version 100.6.0 of the MongoDB Database Tools.

This release introduces a security enhancement. The tools will interactively prompt for a SSL key password if the corresponding SSL key is encrypted and no password is provided on the command line.

The Database Tools are available on the [MongoDB Download Center](https://www.mongodb.com/try/download/database-tools).
Installation instructions and documentation can be found on [docs.mongodb.com/database-tools](https://docs.mongodb.com/database-tools/).
Questions and inquiries can be asked on the [MongoDB Developer Community Forum](https://developer.mongodb.com/community/forums/tags/c/developer-tools/49/database-tools).
Please make sure to tag forum posts with `database-tools`.
Bugs and feature requests can be reported in the [Database Tools Jira](https://jira.mongodb.org/browse/TOOLS) where a list of current issues can be found.

### New Feature

- [TOOLS-2913 - Prompt for SSL key password when key is encrypted](https://jira.mongodb.org/browse/TOOLS-2913)

## 100.5.4

_Released 2022-07-19_

We are pleased to announce version 100.5.4 of the MongoDB Database Tools.

This release mostly consists of build failure fixes, support for new platforms, and tests against server version 6.0. The new platforms are Debian 11 on x86, Ubuntu 22.04 on x86 and ARM, and RHEL 8.3 on s390x. The version of Go driver used by the tools has been updated to 1.10.0.

The Database Tools are available on the [MongoDB Download Center](https://www.mongodb.com/try/download/database-tools).
Installation instructions and documentation can be found on [docs.mongodb.com/database-tools](https://docs.mongodb.com/database-tools/).
Questions and inquiries can be asked on the [MongoDB Developer Community Forum](https://developer.mongodb.com/community/forums/tags/c/developer-tools/49/database-tools).
Please make sure to tag forum posts with `database-tools`.
Bugs and feature requests can be reported in the [Database Tools Jira](https://jira.mongodb.org/browse/TOOLS) where a list of current issues can be found.

### Build Failure

- [TOOLS-3100 - Fix native-cert-ssl-4.4 task failure in all build variants](https://jira.mongodb.org/browse/TOOLS-3100)
- [TOOLS-3101 - Fix failing aws-auth-6.0 and aws-auth-latest tasks](https://jira.mongodb.org/browse/TOOLS-3101)
- [TOOLS-3102 - Fix intermittent failures of qa-tests-{5.3, 6.0, latest} tasks](https://jira.mongodb.org/browse/TOOLS-3102)
- [TOOLS-3110 - Fix integration test failures with server 6.0+](https://jira.mongodb.org/browse/TOOLS-3110)
- [TOOLS-3111 - Fix intermittent legacy JS test task failure](https://jira.mongodb.org/browse/TOOLS-3111)
- [TOOLS-3122 - Fix SSL cert test(s) on RHEL 6.2](https://jira.mongodb.org/browse/TOOLS-3122)
- [TOOLS-3156 - Unable to publish to Ubuntu 22.04 repos](https://jira.mongodb.org/browse/TOOLS-3156)

### Task

- [TOOLS-3045 - Add tests for latest server release](https://jira.mongodb.org/browse/TOOLS-3045)
- [TOOLS-3051 - Release Tools with Debian 11](https://jira.mongodb.org/browse/TOOLS-3051)
- [TOOLS-3058 - Add Ubuntu 22.04 ARM to Tools](https://jira.mongodb.org/browse/TOOLS-3058)
- [TOOLS-3059 - Release Tools with Ubuntu 22.04 ARM](https://jira.mongodb.org/browse/TOOLS-3059)
- [TOOLS-3060 - Add Ubuntu 22.04 x86 to Tools](https://jira.mongodb.org/browse/TOOLS-3060)
- [TOOLS-3061 - Release Tools with Ubuntu 22.04 x86](https://jira.mongodb.org/browse/TOOLS-3061)
- [TOOLS-3103 - Add tests for 6.0 to evergreen](https://jira.mongodb.org/browse/TOOLS-3103)
- [TOOLS-3113 - Test secondary indexes on timeseries collections](https://jira.mongodb.org/browse/TOOLS-3113)
- [TOOLS-3130 - Add 6.0 to list of linux repos we release to](https://jira.mongodb.org/browse/TOOLS-3130)
- [TOOLS-3149 - Update the Go Driver to 1.10.0](https://jira.mongodb.org/browse/TOOLS-3149)
- [TOOLS-3155 - Repo config for RHEL 8.3 on s390x is incorrect](https://jira.mongodb.org/browse/TOOLS-3155)
- [TOOLS-2939 - Add Enterprise RHEL 8 zSeries](https://jira.mongodb.org/browse/TOOLS-2939)


## 100.5.3

_Released 2022-06-14_

We are pleased to announce version 100.5.3 of the MongoDB Database Tools.

This release contains a number of bug fixes and changes. Highlights include support for clustered collections in mongorestore, updating our Go version from 1.16.7 to 1.17.8 to address CVEs, and supported platform updates.

The Database Tools are available on the [MongoDB Download Center](https://www.mongodb.com/try/download/database-tools).
Installation instructions and documentation can be found on [docs.mongodb.com/database-tools](https://docs.mongodb.com/database-tools/).
Questions and inquiries can be asked on the [MongoDB Developer Community Forum](https://developer.mongodb.com/community/forums/tags/c/developer-tools/49/database-tools).
Please make sure to tag forum posts with `database-tools`.
Bugs and feature requests can be reported in the [Database Tools Jira](https://jira.mongodb.org/browse/TOOLS) where a list of current issues can be found.


### Build Failure

* [TOOLS-3119 - All builds are failing on RHEL6.2](https://jira.mongodb.org/browse/TOOLS-3119)
* [TOOLS-3126 - The unit tests for options processing segfault on macOS](https://jira.mongodb.org/browse/TOOLS-3126)
* [TOOLS-3127 - The dist CI task is failing on Windows](https://jira.mongodb.org/browse/TOOLS-3127)

### Bug

* [TOOLS-2958 - An index deletion or collMod in the oplog can be applied to the wrong index](https://jira.mongodb.org/browse/TOOLS-2958)
* [TOOLS-2961 - The RHEL82 ARM release does not use the correct architecture](https://jira.mongodb.org/browse/TOOLS-2961)
* [TOOLS-2963 - Tools are not prompting for a password in many cases where they should](https://jira.mongodb.org/browse/TOOLS-2963)
* [TOOLS-3044 - The zip file for tools on Windows contains invalid paths](https://jira.mongodb.org/browse/TOOLS-3044)
* [TOOLS-3071 - Tools installed by RPM packages to /usr/bin are owned by mongod:mongod instead of root:root](https://jira.mongodb.org/browse/TOOLS-3071)

### Task

* [TOOLS-2906 - Update Evergreen config to use new merge key format](https://jira.mongodb.org/browse/TOOLS-2906)
* [TOOLS-3001 - bsondump should allow documents up to the internal max bson size (16mb + 16kb)](https://jira.mongodb.org/browse/TOOLS-3001)
* [TOOLS-3028 - Remove evergreen batchtimes from ZAP](https://jira.mongodb.org/browse/TOOLS-3028)
* [TOOLS-3049 - Update the Go version used to build mongo-tools to address several critical and high CVEs](https://jira.mongodb.org/browse/TOOLS-3049)
* [TOOLS-3050 - Add Debian 11 to platforms we publish tools packages for](https://jira.mongodb.org/browse/TOOLS-3050)
* [TOOLS-3095 - Remove Ubuntu 14.04 from CI and release platforms](https://jira.mongodb.org/browse/TOOLS-3095)
* [TOOLS-3104 - Add tests for 5.3 to evergreen](https://jira.mongodb.org/browse/TOOLS-3104)
* [TOOLS-3105 - Pin Go driver to version 1.9.1](https://jira.mongodb.org/browse/TOOLS-3105)
* [TOOLS-3106 - Remove tests for 5.1 and 5.2 for most platforms](https://jira.mongodb.org/browse/TOOLS-3106)
* [TOOLS-3108 - Update mongorestore to support clustered indexes](https://jira.mongodb.org/browse/TOOLS-3108)
* [TOOLS-3116 - Change Windows build to run on windows-vsCurrent-large](https://jira.mongodb.org/browse/TOOLS-3116)

## 100.5.2

_Released 2022-02-01_

We are pleased to announce version 100.5.2 of the MongoDB Database Tools.

This release fixes an issue where inserting large documents with mongorestore or mongoimport could cause extremely high memory usage (<a href='https://jira.mongodb.org/browse/TOOLS-2875'>TOOLS-2875</a>). It also fixes a few minor bugs.

The Database Tools are available on the [MongoDB Download Center](https://www.mongodb.com/try/download/database-tools).
Installation instructions and documentation can be found on [docs.mongodb.com/database-tools](https://docs.mongodb.com/database-tools/).
Questions and inquiries can be asked on the [MongoDB Developer Community Forum](https://developer.mongodb.com/community/forums/tags/c/developer-tools/49/database-tools).
Please make sure to tag forum posts with `database-tools`.
Bugs and feature requests can be reported in the [Database Tools Jira](https://jira.mongodb.org/browse/TOOLS) where a list of current issues can be found.

### Bug
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2745'>TOOLS-2745</a>] -         Tools don't support setting retryWrites=false in URI parameter
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2983'>TOOLS-2983</a>] -         Some error messages for conflicting URI/CLI arguments are misleading
</li>
</ul>

### Task
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2875'>TOOLS-2875</a>] -         Limit the BufferedBulkInserter's batch size by bytes
</li>
</ul>

## 100.5.1

_Released 2021-10-12_

We are pleased to announce version 100.5.1 of the MongoDB Database Tools.

This release fixes an issue where certain config collections which should generally be ignored were included by mongodump/mongorestore. This release also ensures that any operations on these collections will not be applied during the oplog replay phase of mongorestore. 

The Database Tools are available on the [MongoDB Download Center](https://www.mongodb.com/try/download/database-tools).
Installation instructions and documentation can be found on [docs.mongodb.com/database-tools](https://docs.mongodb.com/database-tools/).
Questions and inquiries can be asked on the [MongoDB Developer Community Forum](https://developer.mongodb.com/community/forums/tags/c/developer-tools/49/database-tools).
Please make sure to tag forum posts with `database-tools`.
Bugs and feature requests can be reported in the [Database Tools Jira](https://jira.mongodb.org/browse/TOOLS) where a list of current issues can be found.

### Bug
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2952'>TOOLS-2952</a>] -         Filter config collections in dump/restore
</li>
</ul>

## 100.5.0

_Released 2021-08-10_

We are pleased to announce version 100.5.0 of the MongoDB Database Tools.

This release includes support for the loadbalanced URI option, which provides compatibility with [MongoDB Atlas Serverless](https://www.mongodb.com/developer/how-to/atlas-serverless-quick-start/). 

The Database Tools are available on the [MongoDB Download Center](https://www.mongodb.com/try/download/database-tools).
Installation instructions and documentation can be found on [docs.mongodb.com/database-tools](https://docs.mongodb.com/database-tools/).
Questions and inquiries can be asked on the [MongoDB Developer Community Forum](https://developer.mongodb.com/community/forums/tags/c/developer-tools/49/database-tools).
Please make sure to tag forum posts with `database-tools`.
Bugs and feature requests can be reported in the [Database Tools Jira](https://jira.mongodb.org/browse/TOOLS) where a list of current issues can be found.

### Build Failure
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2938'>TOOLS-2938</a>] -         Re-add Ubuntu 16.04 PowerPC platform
</li>
</ul>
                                                                        
### Release
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2880'>TOOLS-2880</a>] -         Release Database Tools 100.5.0
</li>
</ul>

### Bug
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2863'>TOOLS-2863</a>] -         cs.AuthMechanismProperties is not initialized when mechanism set by --authenticationMechanism
</li>
</ul>
        
### New Feature
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2937'>TOOLS-2937</a>] -         Set loadbalanced option in db.configureClient()
</li>
</ul>
        
### Task
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2932'>TOOLS-2932</a>] -         Upgrade to Go Driver 1.7.1
</li>
</ul>

## 100.4.1

_Released 2021-07-23_

We are pleased to announce version 100.4.1 of the MongoDB Database Tools.

This patch fixes a bug ([TOOLS-2931](https://jira.mongodb.org/browse/TOOLS-2931)) that was introduced in version 100.4.0 which causes mongodump to skip any document that contains an empty field name (e.g. `{ "": "foo" }`). Documents with empty field names were not skipped by default if the `--query` or `--queryFile` options were specified. No tools other than mongodump were affected. It is highly recommended to upgrade to 100.4.1 if it is possible that your database contains documents with empty field names.

The Database Tools are available on the [MongoDB Download Center](https://www.mongodb.com/try/download/database-tools).
Installation instructions and documentation can be found on [docs.mongodb.com/database-tools](https://docs.mongodb.com/database-tools/).
Questions and inquiries can be asked on the [MongoDB Developer Community Forum](https://developer.mongodb.com/community/forums/tags/c/developer-tools/49/database-tools).
Please make sure to tag forum posts with `database-tools`.
Bugs and feature requests can be reported in the [Database Tools Jira](https://jira.mongodb.org/browse/TOOLS) where a list of current issues can be found.

### Build Failure
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2927'>TOOLS-2927</a>] -         Clean up the platforms list inside platform.go
</li>
</ul>
                                                                        
### Release
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2929'>TOOLS-2929</a>] -         Release Database Tools 100.4.1
</li>
</ul>
                                                      
### Bug
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2931'>TOOLS-2931</a>] -         mongodump skips documents with empty field names
</li>
</ul>
                
### Task
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2926'>TOOLS-2926</a>] -         Run release on &#39;test&#39; and &#39;development&#39; linux repo separately. 
</li>
</ul>
                      
## 100.4.0

_Released 2021-07-19_

We are pleased to announce version 100.4.0 of the MongoDB Database Tools.

This release includes server 5.0 support, including dump/restoring timeseries collections.  

The Database Tools are available on the [MongoDB Download Center](https://www.mongodb.com/try/download/database-tools).
Installation instructions and documentation can be found on [docs.mongodb.com/database-tools](https://docs.mongodb.com/database-tools/).
Questions and inquiries can be asked on the [MongoDB Developer Community Forum](https://developer.mongodb.com/community/forums/tags/c/developer-tools/49/database-tools).
Please make sure to tag forum posts with `database-tools`.
Bugs and feature requests can be reported in the [Database Tools Jira](https://jira.mongodb.org/browse/TOOLS) where a list of current issues can be found.

<h3>        Build Failure
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2892'>TOOLS-2892</a>] -         aws-auth tests failing on all variants
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2893'>TOOLS-2893</a>] -         legacy-js-tests 4.4 and 5.0 failing on all variants
</li>
</ul>
                                                                        
<h3>        Release
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2845'>TOOLS-2845</a>] -         Release Database Tools 100.4.0
</li>
</ul>
                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        
<h3>        Bug
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2041'>TOOLS-2041</a>] -         Mongorestore should handle duplicate key errors during oplog replay
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2833'>TOOLS-2833</a>] -         Creating an index with partialFilterExpression during oplogReplay fails
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2925'>TOOLS-2925</a>] -         RPM packages are only signed with the 4.4 auth token
</li>
</ul>
        
<h3>        New Feature
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2857'>TOOLS-2857</a>] -         Dump timeseries collections
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2858'>TOOLS-2858</a>] -         Mongodump can query timeseries collections by metadata
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2859'>TOOLS-2859</a>] -         Restore timeseries collections
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2860'>TOOLS-2860</a>] -         Include/Exclude/Rename timeseries collections in mongorestore
</li>
</ul>
        
<h3>        Task
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2719'>TOOLS-2719</a>] -         Add Enterprise RHEL 8 zSeries to Tools
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2721'>TOOLS-2721</a>] -         Add RHEL8 ARM to Tools
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2777'>TOOLS-2777</a>] -         Generate Full JSON variant should not be running on every commit
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2823'>TOOLS-2823</a>] -         Build with go 1.16
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2824'>TOOLS-2824</a>] -         Add static analysis task that runs &quot;evergreen validate&quot;
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2849'>TOOLS-2849</a>] -         Mongodump should fail during resharding
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2850'>TOOLS-2850</a>] -         Mongorestore should fail when restoring geoHaystack indexes to 4.9.0
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2851'>TOOLS-2851</a>] -         importCollection command should cause mongodump to fail
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2853'>TOOLS-2853</a>] -         Hide deprecated --slaveOk option
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2866'>TOOLS-2866</a>] -         Drop support for zSeries platforms
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2873'>TOOLS-2873</a>] -         Run full test suite on all supported distros in evergreen
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2881'>TOOLS-2881</a>] -         Push tools releases to 4.9+ linux repos
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2921'>TOOLS-2921</a>] -         Upgrade to Go Driver 1.6
</li>
</ul>
                                                
## 100.3.1

_Released 2021-03-17_

We are pleased to announce version 100.3.1 of the MongoDB Database Tools.

This release includes various bug fixes.
Particularly notable is TOOLS-2783, where we reverted a change from 100.2.1 (TOOLS-1856: use a memory pool in mongorestore) after discovering that it was causing memory usage issues.

The Database Tools are available on the [MongoDB Download Center](https://www.mongodb.com/try/download/database-tools).
Installation instructions and documentation can be found on [docs.mongodb.com/database-tools](https://docs.mongodb.com/database-tools/).
Questions and inquiries can be asked on the [MongoDB Developer Community Forum](https://developer.mongodb.com/community/forums/tags/c/developer-tools/49/database-tools).
Please make sure to tag forum posts with `database-tools`.
Bugs and feature requests can be reported in the [Database Tools Jira](https://jira.mongodb.org/browse/TOOLS) where a list of current issues can be found.

<h3>        Build Failure
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2796'>TOOLS-2796</a>] -         mongotop_sharded.js failing on all versions of the qa-tests
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2815'>TOOLS-2815</a>] -         Development build artifacts accidentally uploaded for versioned release
</li>
</ul>
                                                                        
<h3>        Release
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2791'>TOOLS-2791</a>] -         Release Database Tools 100.3.1
</li>
</ul>
                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        
<h3>        Bug
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2584'>TOOLS-2584</a>] -         Restoring single BSON file should use db set in URI
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2783'>TOOLS-2783</a>] -         Mongorestore uses huge amount of RAM
</li>
</ul>
                
<h3>        Task
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-704'>TOOLS-704</a>] -         Remove system.indexes collection dumping from mongodump
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2801'>TOOLS-2801</a>] -         Migrate from dep to Go modules and update README
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2802'>TOOLS-2802</a>] -         Make mongo-tools-common a subpackage of mongo-tools
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2805'>TOOLS-2805</a>] -         Add mod tidy static analysis check for Go modules
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2806'>TOOLS-2806</a>] -         Migrate mongo-tools-common unit tests to mongo-tools
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2807'>TOOLS-2807</a>] -         Migrate mongo-tools-common integration tests to mongo-tools
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2808'>TOOLS-2808</a>] -         Migrate mongo-tools-common IAM auth tests to mongo-tools
</li>
</ul>
                                                

## 100.3.0

_Released 2021-02-04_

We are pleased to announce version 100.3.0 of the MongoDB Database Tools.

This release includes support for PKCS8-encrypted client private keys, support for providing secrets in a config file instead of on the command line, and a few small bug fixes.

The Database Tools are available on the [MongoDB Download Center](https://www.mongodb.com/try/download/database-tools).
Installation instructions and documentation can be found on [docs.mongodb.com/database-tools](https://docs.mongodb.com/database-tools/).
Questions and inquiries can be asked on the [MongoDB Developer Community Forum](https://developer.mongodb.com/community/forums/tags/c/developer-tools/49/database-tools).
Please make sure to tag forum posts with `database-tools`.
Bugs and feature requests can be reported in the [Database Tools Jira](https://jira.mongodb.org/browse/TOOLS) where a list of current issues can be found.


<h3>        Build Failure
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2795'>TOOLS-2795</a>] -         Tools failing to build on SUSE15-sp2
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2800'>TOOLS-2800</a>] -         RPM creation failing on amazon linux 1
</li>
</ul>
                                                                        
<h3>        Release
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2790'>TOOLS-2790</a>] -         Release Database Tools 100.3.0
</li>
</ul>
                                                            
<h3>        Investigation
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2771'>TOOLS-2771</a>] -         SSL connection problems mongodump
</li>
</ul>
                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                
<h3>        Bug
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2751'>TOOLS-2751</a>] -         Deferred query EstimatedDocumentCount helper incorrect with filter
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2760'>TOOLS-2760</a>] -         rpm package should not obsolete itself
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2775'>TOOLS-2775</a>] -         --local does not work with multi-file get or get_regex
</li>
</ul>
        
<h3>        New Feature
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2779'>TOOLS-2779</a>] -         Add --config option for password values
</li>
</ul>
        
<h3>        Task
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2013'>TOOLS-2013</a>] -         Support PKCS8 encrypted client private keys
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2707'>TOOLS-2707</a>] -         Build mongo-tools and mongo-tools-common with go 1.15
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2780'>TOOLS-2780</a>] -         Add warning when password value appears on command line
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2798'>TOOLS-2798</a>] -         Add Amazon Linux 2 Arm64 to Tools
</li>
</ul>
                                                

## 100.2.1

_Released 2020-11-13_

We are pleased to announce version 100.2.1 of the MongoDB Database Tools.

This release includes a `mongorestore` performance improvement, a fix for a bug affecting highly parallel `mongorestore`s, and an observability improvement to `mongodump` and `mongoexport`, in addition to a number of internal build/release changes.

The Database Tools are available on the [MongoDB Download Center](https://www.mongodb.com/try/download/database-tools).
Installation instructions and documentation can be found on [docs.mongodb.com/database-tools](https://docs.mongodb.com/database-tools/).
Questions and inquiries can be asked on the [MongoDB Developer Community Forum](https://developer.mongodb.com/community/forums/tags/c/developer-tools/49/database-tools).
Please make sure to tag forum posts with `database-tools`.
Bugs and feature requests can be reported in the [Database Tools Jira](https://jira.mongodb.org/browse/TOOLS) where a list of current issues can be found.


<h3>        Build Failure
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2767'>TOOLS-2767</a>] -         Windows 64 dist task fails
</li>
</ul>
                                                                        
<h3>        Release
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2741'>TOOLS-2741</a>] -         Release Database Tools 100.2.1
</li>
</ul>
                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    
<h3>        Bug
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2744'>TOOLS-2744</a>] -         mongorestore not scaling due to unnecessary incremental sleep time
</li>
</ul>
        
<h3>        New Feature
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2750'>TOOLS-2750</a>] -         Log before getting collection counts
</li>
</ul>
        
<h3>        Task
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-1856'>TOOLS-1856</a>] -         use a memory pool in mongorestore 
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2651'>TOOLS-2651</a>] -         Simplify build scripts
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2687'>TOOLS-2687</a>] -         Add archived releases JSON feed for Database Tools
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2735'>TOOLS-2735</a>] -         Move server vendoring instructions to a README in the repo
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2748'>TOOLS-2748</a>] -         Add a String() to OpTime
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2758'>TOOLS-2758</a>] -         Bump Go driver to 1.4.2
</li>
</ul>
                                            

## 100.2.0

_Released 2020-10-15_

We are pleased to announce version 100.2.0 of the MongoDB Database Tools.

This release deprecates the `--sslAllowInvalidHostnames` and `--sslAllowInvalidCertificates` flags in favor of a new `--tlsInsecure` flag. The `mongofiles put` and `mongofiles get` commands can now accept a list of file names. There is a new `mongofiles get_regex` command to retrieve all files matching a regex pattern. The 100.2.0 release also contains fixes for several bugs. It fixes a bug introduced in version 100.1.0 that made it impossible to connect to clusters with an SRV connection string (<a href='https://jira.mongodb.org/browse/TOOLS-2711'>TOOLS-2711</a>).

The Database Tools are available on the [MongoDB Download Center](https://www.mongodb.com/try/download/database-tools).
Installation instructions and documentation can be found on [docs.mongodb.com/database-tools](https://docs.mongodb.com/database-tools/).
Questions and inquiries can be asked on the [MongoDB Developer Community Forum](https://developer.mongodb.com/community/forums/tags/c/developer-tools/49/database-tools).
Please make sure to tag forum posts with `database-tools`.
Bugs and feature requests can be reported in the [Database Tools Jira](https://jira.mongodb.org/browse/TOOLS) where a list of current issues can be found.

                                                
<h3>        Build Failure
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2693'>TOOLS-2693</a>] -         Most tasks failing on race detector variant
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2737'>TOOLS-2737</a>] -         Fix TLS tests on Mac and Windows
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2747'>TOOLS-2747</a>] -         Git tag release process does not work
</li>
</ul>
                                                                        
<h3>        Release
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2704'>TOOLS-2704</a>] -         Release Database Tools 100.2.0
</li>
</ul>
                                                       
<h3>        Bug
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2587'>TOOLS-2587</a>] -         sslAllowInvalidHostnames bypass ssl/tls server certification validation entirely
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2688'>TOOLS-2688</a>] -         mongodump does not handle EOF when passing in the password as STDIN
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2706'>TOOLS-2706</a>] -         tar: implausibly old time stamp error on Amazon Linux/RHEL
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2708'>TOOLS-2708</a>] -         Atlas recommended connection string for mongostat doesn&#39;t work
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2710'>TOOLS-2710</a>] -         Non-zero index key values are not preserved in ConvertLegacyIndexes
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2711'>TOOLS-2711</a>] -         Tools fail with &quot;a direct connection cannot be made if multiple hosts are specified&quot; if mongodb+srv URI or a legacy uri containing multiple mongos is specified
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2716'>TOOLS-2716</a>] -         mongodb-database-tools package should break older versions of mongodb-*-tools
</li>
</ul>
        
<h3>        New Feature
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2667'>TOOLS-2667</a>] -         Support list of files for put and get subcommands in mongofiles
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2668'>TOOLS-2668</a>] -         Create regex interface for getting files from remote FS in mongofiles
</li>
</ul>
        
<h3>        Task
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2674'>TOOLS-2674</a>] -         Clarify contribution guidelines
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2700'>TOOLS-2700</a>] -         Use git tags for triggering release versions
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2701'>TOOLS-2701</a>] -         Log target linux repo in push task
</li>
</ul>
                                           

## 100.1.1

_Released 2020-07-31_

We are pleased to announce version 100.1.1 of the MongoDB Database Tools.

This release fixes contains a fix for a linux packaging bug and a mongorestore bug related to the convertLegacyIndexes flag.

The Database Tools are available on the [MongoDB Download Center](https://www.mongodb.com/try/download/database-tools).
Installation instructions and documentation can be found on [docs.mongodb.com/database-tools](https://docs.mongodb.com/database-tools/).
Questions and inquiries can be asked on the [MongoDB Developer Community Forum](https://developer.mongodb.com/community/forums/tags/c/developer-tools/49/database-tools).
Please make sure to tag forum posts with `database-tools`.
Bugs and feature requests can be reported in the [Database Tools Jira](https://jira.mongodb.org/browse/TOOLS) where a list of current issues can be found.

<h3>        Release
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2685'>TOOLS-2685</a>] -         Release Database Tools 100.1.1
</li>
</ul>
                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        
<h3>        Bug
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2645'>TOOLS-2645</a>] -         Check for duplicate index keys after converting legacy index definitions
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2683'>TOOLS-2683</a>] -         Ubuntu 16.04 DB Tools 100.1.0 DEB depends on libcom-err2, should be libcomerr2
</li>
</ul>

## 100.1.0

_Released 2020-07-24_

We are pleased to announce version 100.1.0 of the MongoDB Database Tools.

This release officially adds support for MongoDB 4.4.
In addition to various bug fixes, it adds support for MongoDB 4.4's new MONGODB-AWS authentication mechanism.

The Database Tools are available on the [MongoDB Download Center](https://www.mongodb.com/try/download/database-tools).
Installation instructions and documentation can be found on [docs.mongodb.com/database-tools](https://docs.mongodb.com/database-tools/).
Questions and inquiries can be asked on the [MongoDB Developer Community Forum](https://developer.mongodb.com/community/forums/tags/c/developer-tools/49/database-tools).
Please make sure to tag forum posts with `database-tools`.
Bugs and feature requests can be reported in the [Database Tools Jira](https://jira.mongodb.org/browse/TOOLS) where a list of current issues can be found.

<h3>        Build Failure
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2604'>TOOLS-2604</a>] -         integration-4.4-cluster is failing on master
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2638'>TOOLS-2638</a>] -         Test-case failure for mongorestore
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2643'>TOOLS-2643</a>] -         New linux distros missing from repo-config.yaml
</li>
</ul>
                                                                        
<h3>        Release
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2630'>TOOLS-2630</a>] -         Release Database Tools 100.1.0
</li>
</ul>
                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        
<h3>        Bug
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2287'>TOOLS-2287</a>] -         URI parser incorrectly prints unsupported parameter warnings
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2337'>TOOLS-2337</a>] -         nsInclude does not work with percent encoded namespaces
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2366'>TOOLS-2366</a>] -         ^C isn&#39;t handled by mongodump
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2494'>TOOLS-2494</a>] -         mongorestore thorw error &quot;panic: close of closed channel&quot;
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2531'>TOOLS-2531</a>] -         mongorestore hung if restoring views with --preserveUUID --drop options
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2596'>TOOLS-2596</a>] -         DBTools --help links to old Manual doc pages
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2597'>TOOLS-2597</a>] -           swallows errors from URI parsing
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2609'>TOOLS-2609</a>] -         Detached signatures incorrectly appearing in download JSON
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2622'>TOOLS-2622</a>] -         Tools do not build following README instructions
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2669'>TOOLS-2669</a>] -         macOS zip archive structure incorrect
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2670'>TOOLS-2670</a>] -         Troubleshoot IAM auth options errors
</li>
</ul>
        
<h3>        New Feature
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2369'>TOOLS-2369</a>] -         IAM Role-based authentication
</li>
</ul>
        
<h3>        Task
</h3>
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2363'>TOOLS-2363</a>] -         Update warning message for &quot;mongorestore&quot;
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2476'>TOOLS-2476</a>] -         Notarize builds for macOS catalina
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2505'>TOOLS-2505</a>] -         Add missing 4.4 Platforms
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2534'>TOOLS-2534</a>] -         Ignore startIndexBuild and abortIndexBuild oplog entries in oplog replay
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2535'>TOOLS-2535</a>] -         commitIndexBuild and createIndexes oplog entries should build indexes with the createIndexes command during oplog replay
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2554'>TOOLS-2554</a>] -         Remove ReplSetTest file dependencies from repo
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2569'>TOOLS-2569</a>] -         Update tools to go driver 1.4.0
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2618'>TOOLS-2618</a>] -         Refactor AWS IAM auth testing code
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2628'>TOOLS-2628</a>] -         Add 3.4 tests to evg
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2644'>TOOLS-2644</a>] -         Update barque authentication
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2650'>TOOLS-2650</a>] -         Create changelog for tools releases
</li>
</ul>

## 100.0.2

_Released 2020-06-04_

We are pleased to announce version 100.0.2 of the MongoDB Database Tools.

This release contains several bugfixes. It also adds support for dumping and restoring collections with long names since the 120 byte name limit will be raised to 255 bytes in MongoDB version 4.4.

The Database Tools are available on the [MongoDB Download Center](https://www.mongodb.com/try/download/database-tools). Installation instructions and documentation can be found on [docs.mongodb.com/database-tools](https://docs.mongodb.com/database-tools/). Questions and inquiries can be asked on the [MongoDB Developer Community Forum](https://developer.mongodb.com/community/forums/tags/c/developer-tools/49/database-tools). Please make sure to tag forum posts with `database-tools`. Bugs and feature requests can be reported in the [Database Tools Jira](https://jira.mongodb.org/browse/TOOLS) where a list of current issues can be found.
                                                             
### Bug
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-1785'>TOOLS-1785</a>] -         Typo in mongodump help
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2495'>TOOLS-2495</a>] -         Oplog replay can&#39;t handle entries &gt; 16 MB
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2498'>TOOLS-2498</a>] -         Nil pointer error mongodump
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2559'>TOOLS-2559</a>] -         Error on uninstalling database-tools 99.0.1-1 RPM
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2575'>TOOLS-2575</a>] -         mongorestore panic during convertLegacyIndexes from 4.4 mongodump
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2593'>TOOLS-2593</a>] -         Fix special handling of $admin filenames
</li>
</ul>
                
### Task
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2446'>TOOLS-2446</a>] -         Add MMAPV1 testing to Tools tests
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2469'>TOOLS-2469</a>] -         Accept multiple certs in CA 
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2530'>TOOLS-2530</a>] -         Mongorestore can restore from new mongodump format
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2537'>TOOLS-2537</a>] -         Ignore config.system.indexBuilds namespace
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2544'>TOOLS-2544</a>] -         Add 4.4 tests to Evergreen
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2551'>TOOLS-2551</a>] -         Split release uploading into per-distro tasks
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2555'>TOOLS-2555</a>] -         Support directConnection option
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2561'>TOOLS-2561</a>] -         Sign mongodb-tools tarballs
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2605'>TOOLS-2605</a>] -         Cut 100.0.2 release
</li>
</ul>
                                            

## 100.0.1

_Released 2020-04-28_

We are pleased to announce version 100.0.1 of the MongoDB Database Tools.

This release was a test of our new release infrastructure and contains no changes from 100.0.0.

The Database Tools are available on the [MongoDB Download Center](https://www.mongodb.com/try/download/database-tools). Installation instructions and documentation can be found on [docs.mongodb.com/database-tools](https://docs.mongodb.com/database-tools/). Questions and inquiries can be asked on the [MongoDB Developer Community Forum](https://developer.mongodb.com/community/forums/tags/c/developer-tools/49/database-tools). Please make sure to tag forum posts with `database-tools`. Bugs and feature requests can be reported in the [Database Tools Jira](https://jira.mongodb.org/browse/TOOLS) where a list of current issues can be found.
                                                                                        
### Task
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2493'>TOOLS-2493</a>] -         Cut tools 100.0.0 and 100.0.1 GA releases
</li>
</ul>

## 100.0.0

_Released 2020-04-28_

We are pleased to announce version 100.0.0 of the MongoDB Database Tools.

This is the first separate release of the Database Tools from the Server. We decided to move to a separate release so we can ship new features and bugfixes more frequently. The new separate release version starts from 100.0.0 to make it clear the versioning is separate from the Server. You can read more about this on the [MongoDB blog](alendar.google.com/calendar/render?tab=mc#main_7).

This release contains bugfixes, some new command-line options, and quality of life improvements. A full list can be found below, but here are some highlights: 

- There are no longer restrictions on using `--uri` with other connection options such as `--port` and `--password` as long as the URI and the explicit option don't provide conflicting information. Connection strings can now be specified as a positional argument without the `--uri` option.
- The new [`--useArrayIndexFields`](https://docs.mongodb.com/database-tools/mongoimport/#cmdoption-mongoimport-usearrayindexfields) flag for mongoimport interprets natural numbers in fields as array indexes when importing csv or tsv files.
- The new [`--convertLegacyIndexes`](https://docs.mongodb.com/database-tools/mongorestore/#cmdoption-mongorestore-convertlegacyindexes) flag for mongorestore removes any invalid index options specified in the corresponding mongodump output, and rewrites any legacy index key values to use valid values.
- A new [`delete` mode](https://docs.mongodb.com/database-tools/mongoimport/#ex-mongoimport-delete) for mongoimport. With `--mode delete`, mongoimport deletes existing documents in the database that match a document in the import file.

The Database Tools are available on the [MongoDB Download Center](https://www.mongodb.com/try/download/database-tools). Installation instructions and documentation can be found on [docs.mongodb.com/database-tools](https://docs.mongodb.com/database-tools/). Questions and inquiries can be asked on the [MongoDB Developer Community Forum](https://developer.mongodb.com/community/forums/tags/c/developer-tools/49/database-tools). Please make sure to tag forum posts with `database-tools`. Bugs and feature requests can be reported in the [Database Tools Jira](https://jira.mongodb.org/browse/TOOLS) where a list of current issues can be found.
                                                
### Build Failure
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2489'>TOOLS-2489</a>] -         format-go task failing on master
</li>
</ul>
                                                                                                           
### Bug
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-1493'>TOOLS-1493</a>] -         Tools crash running help when terminal width is low
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-1786'>TOOLS-1786</a>] -         mongodump does not create metadata.json file for views dumped as collections
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-1826'>TOOLS-1826</a>] -         mongorestore panic in archive mode when replay oplog failed
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-1909'>TOOLS-1909</a>] -         mongoimport does not report that it supports the decimal type
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2275'>TOOLS-2275</a>] -         autoIndexId:false is not supported in 4.0
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2334'>TOOLS-2334</a>] -         Skip system collections during oplog replay
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2336'>TOOLS-2336</a>] -         Wrong deprecation error message printed when restoring from stdin
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2346'>TOOLS-2346</a>] -         mongodump --archive to stdout corrupts archive when prompting for password
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2379'>TOOLS-2379</a>] -         mongodump/mongorestore error if source database has an invalid index option
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2380'>TOOLS-2380</a>] -         mongodump fails against hidden node with authentication enabled
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2381'>TOOLS-2381</a>] -         Restore no socket timeout behavior
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2395'>TOOLS-2395</a>] -         Incorrect message for oplog overflow
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2403'>TOOLS-2403</a>] -         mongorestore hang while replaying last oplog failed in archive mode
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2422'>TOOLS-2422</a>] -         admin.tempusers is not dropped by mongorestore
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2423'>TOOLS-2423</a>] -         mongorestore does not drop admin.tempusers if it exists in the dump
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2455'>TOOLS-2455</a>] -         mongorestore hangs on invalid archive
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2462'>TOOLS-2462</a>] -         Password prompt does not work on windows
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2497'>TOOLS-2497</a>] -         mongorestore may incorrectly validate index name length before calling createIndexes
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2513'>TOOLS-2513</a>] -         Creating client options results in connection string validation error
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2520'>TOOLS-2520</a>] -         Fix options parsing for SSL options
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2547'>TOOLS-2547</a>] -         Installing database tools fails on rhel 7.0
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2548'>TOOLS-2548</a>] -         Installing database tools fails on SLES 15
</li>
</ul>
        
### New Feature
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-1954'>TOOLS-1954</a>] -         Support roundtrip of mongoexport array notation in mongoimport
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2268'>TOOLS-2268</a>] -         Add remove mode to mongoimport
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2412'>TOOLS-2412</a>] -         Strip unsupported legacy index options
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2430'>TOOLS-2430</a>] -         mongorestore: in dotted index keys, replace &quot;hashed&quot; with &quot;1&quot; 
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2459'>TOOLS-2459</a>] -         Allow --uri to be used with other connection string options
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2460'>TOOLS-2460</a>] -         A connection string can be set as a positional argument
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2521'>TOOLS-2521</a>] -         Add support for the tlsDisableOCSPEndpointCheck URI option
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2529'>TOOLS-2529</a>] -         Mongodump outputs new file format for long collection names
</li>
</ul>
        
### Task
<ul>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2418'>TOOLS-2418</a>] -         Remove mongoreplay from mongo-tools
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2421'>TOOLS-2421</a>] -         Maintain test coverage after moving tools tests from server
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2438'>TOOLS-2438</a>] -         Create MSI installer in dist task
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2439'>TOOLS-2439</a>] -         Tools formula included in homebrew tap
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2440'>TOOLS-2440</a>] -         Sign MSI installer
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2441'>TOOLS-2441</a>] -         Update release process documentation
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2442'>TOOLS-2442</a>] -         Automate release uploads
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2443'>TOOLS-2443</a>] -         Generate tarball archive in dist task
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2444'>TOOLS-2444</a>] -         Generate deb packages in dist task
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2449'>TOOLS-2449</a>] -         Create sign task
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2464'>TOOLS-2464</a>] -         Update platform support
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2470'>TOOLS-2470</a>] -         Sign linux packages
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2471'>TOOLS-2471</a>] -         Automate JSON download feed generation
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2472'>TOOLS-2472</a>] -         Automate linux package publishing
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2473'>TOOLS-2473</a>] -         Consolidate community and enterprise buildvariants
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2475'>TOOLS-2475</a>] -         Manually verify tools release
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2480'>TOOLS-2480</a>] -         Generate rpm packages in dist task
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2488'>TOOLS-2488</a>] -         Update package naming and versioning
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2493'>TOOLS-2493</a>] -         Cut tools 100.0.0 and 100.0.1 GA releases
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2506'>TOOLS-2506</a>] -         Update maintainer in linux packages
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2523'>TOOLS-2523</a>] -         Remove Ubuntu 12.04 and Debian 7.1 variants
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2536'>TOOLS-2536</a>] -         ignoreUnknownIndexOptions option in the createIndexes command for servers &gt;4.1.9
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2538'>TOOLS-2538</a>] -         Move convertLegacyIndexKeys() from mongorestore to mongo-tools-common
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2539'>TOOLS-2539</a>] -         Publish linux packages to curator with correct names
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2549'>TOOLS-2549</a>] -         Push GA releases to server testing repo
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2550'>TOOLS-2550</a>] -         Push GA releases to the 4.4 repo
</li>
<li>[<a href='https://jira.mongodb.org/browse/TOOLS-2551'>TOOLS-2551</a>] -         Split release uploading into per-distro tasks
</li>
</ul>
                                                                            
