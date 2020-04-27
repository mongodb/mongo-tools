# Releasing the MongoDB Database Tools

This document describes the version policy and release process for the MongoDB Database Tools.

## Versioning

The MongoDB Database Tools use [Semantic Versioning](https://semver.org/).

We will use the following guidelines to determine when each version component will be updated:
- **major**: backwards-breaking changes to the CLI API or other behaviors (e.g. exit codes) that could break scripts using the Tools
- **minor**: new features, including new server version support
- **patch**: bug fixes and cosmetic changes

The team recognizes that log messages, while intended to be informational, may cause breakage if scripts attempt to parse output.
While we will not commit to a formal versioning policy for log message changes, we will attempt to adhere to the following guidelines:
- **log.Always** level (the default): adding logging is a "minor" change; changing or removing logging is a "major" change.
- **log.Info** and higher levels (-v to -vvv): additions, modifications and removals are "patch" changes. We make no promises about message stability at these levels. 

At the moment, there are no pre-release (alpha, beta, rc, etc.) versions of the Tools.
If there is a need to support pre-release versions of the Tools, we will need to update our release infrastructure to support them.

## Releasing
This section describes the steps for releasing a new version of the Tools.

### Pre-Release Tasks
Complete these tasks before tagging a new release.

#### Ensure Evergreen Passing
Ensure that the build you are releasing is passing the tests on the evergreen waterfall.
A completely green build is not mandatory, since we do have flaky tests; however, failing tasks should be manually investigated to ensure they are not actual test failures.

#### Complete the Release in JIRA
Go to the [Tools releases page](https://jira.mongodb.org/projects/TOOLS?selectedItem=com.atlassian.jira.jira-projects-plugin%3Arelease-page&status=unreleased), and ensure that all the tickets in the fixVersion to be released are closed.
If there are any remaining tickets that will not be included in this release, remove the fixVersion and assign them a new one if appropriate.
Close the release on JIRA, adding the current date.

### Releasing

#### Major Release
There are some parts of the release infrastructure that will need to be changed in order to support a major release.
Those changes will need to be made before we can do a new major release.
At the time when the major release process is formalized, this section will be replaced with more specific instructions.

#### Minor/Patch Release
##### Determine the new version
First, determine the version number of the new release.
The new version should be the smallest possible minor or patch increment from the current version.

For example, consider the version `<x>.<y>.<z>`.
Bumping the patch version would give version `<x>.<y>.<z+1>`.
Bumping the minor version would give version `<x>.<y+1>.0`.

##### Create and push the tag
Ensure you have the `master` branch checked out.
This document assumes that you are tagging the latest commit on master for release.
You should ensure that this commit has been pushed, and that tests are passing on evergreen.

Next, create an annotated tag and push it:
```
git tag -a -m "vX.Y.Z" X.Y.Z
git push origin X.Y.Z
```

##### Restart evergreen tasks
Restart all `dist`, `sign`, and `push` tasks, as well as all tasks in the `Release Manager` buildvariant.
You may need to bump the priority of some of those tasks to get them to run in a timely manner.

### Post-Release Tasks
Complete these tasks after the release builds have completed on evergreen.

#### File a CLOUDP Ticket
File a CLOUDP ticket notifying the Automation team that the new release is available.

#### Update Homebrew Tap
In order to make the latest release available via our Homebrew tap, submit a pull request to [mongodb/homebrew-brew](https://github.com/mongodb/homebrew-brew), updating the [download link and sha256 sum](https://github.com/mongodb/homebrew-brew/blob/4ae91b18eebd313960de85c28d5592a3fa32110a/Formula/mongodb-database-tools.rb#L7-L8).
