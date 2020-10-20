# Database Tools Changelog

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
                                                                            
