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

#### File CLOUDP Tickets
File the following CLOUDP tickets for deploying the new release:
- "Release Database Tools X.Y.Z to CM/OM" with a component of "Automation Agent" and assigned team of "Automation"
- "Release Database Tools X.Y.Z to Atlas" with a component of "Automation Agent" and assigned team of "Atlas Triage"

#### Update Homebrew Tap
In order to make the latest release available via our Homebrew tap, submit a pull request to [mongodb/homebrew-brew](https://github.com/mongodb/homebrew-brew), updating the [download link and sha256 sum](https://github.com/mongodb/homebrew-brew/blob/4ae91b18eebd313960de85c28d5592a3fa32110a/Formula/mongodb-database-tools.rb#L7-L8).

#### Update the changelog

- Checkout a new branch.
- Copy the following to the top of CHANGELOG.md under the title:

```
## X.Y.Z

_Released YYYY-MM-DD_

We are pleased to announce version X.Y.Z of the MongoDB Database Tools.

<INSERT-DESCRIPTION>

The Database Tools are available on the [MongoDB Download Center](https://www.mongodb.com/try/download/database-tools). Installation instructions and documentation can be found on [docs.mongodb.com/database-tools](https://docs.mongodb.com/database-tools/). Questions and inquiries can be asked on the [MongoDB Developer Community Forum](https://developer.mongodb.com/community/forums/tags/c/developer-tools/49/database-tools). Please make sure to tag forum posts with `database-tools`. Bugs and feature requests can be reported in the [Database Tools Jira](https://jira.mongodb.org/browse/TOOLS) where a list of current issues can be found.

<INSERT-LIST-OF-TICKETS>
```

- Update the release date to the date the release-json task finished on Evergreen in Eastern Time. You can set your timezone in "User Settings". 
- Go to [Configure Release Notes](https://jira.mongodb.org/secure/ConfigureReleaseNote.jspa?projectId=12385) on JIRA. Choose the version you are releasing and HTML as the style. This will show you the list of tickets tagged with the release version. (If the link doesn't work, you can access this through the release page for the version you are releasing.)
- Go through the list of tickets and check that each ticket is categorized correctly (as a task, bugfix etc.). Also make sure there is nothing in the list that might have been tagged with the wrong fix version.
- Copy the HTML list of tickets from JIRA and paste it in CHANGELOG.md in place of `<INSERT-LIST-OF-TICKETS>`.
- Remove the top line of the list of tickets that says `Release Notes - MongoDB Database Tools - Version X.Y.Z`
- Change the ticket type titles from `<h2>`s to `<h3>`s. For example,

    ```
    <h2>        Build Failure
    </h2>
    ```

    Becomes:

    ```
    ### Build Failure
    ```
- Insert a brief description of the release in place of `<INSERT-DESCRIPTION>`. Don't go into too much unnecessary detail. 
- Submit a PR with your changes.

#### Update the changelog in the docs

Once the PR has been approved and merged, open a DOCSP ticket and ask the docs team to update the changelog in the docs with the new entry in CHANGELOG.md.

#### Announce the release

Copy your entry from CHANGELOG.md and post it to the [MongoDB Community Forums](https://developer.mongodb.com/community/forums/tags/c/developer-tools/49/database-tools) in the "Developer Tools" section with the tag `database-tools`. Also post it in the #mongo-tools slack channel to announce it internally.
