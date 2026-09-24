# Releasing the MongoDB Database Tools

This document describes the version policy and release process for the MongoDB Database Tools.

## Versioning

The MongoDB Database Tools use [Semantic Versioning](https://semver.org/).

We will use the following guidelines to determine when each version component will be updated:

- **major**: backwards-breaking changes to the CLI API or other behaviors (e.g. exit codes) that
  could break scripts using the Tools
- **minor**: new features, including new server version support
- **patch**: bug fixes and cosmetic changes

The team recognizes that log messages, while intended to be informational, may cause breakage if
scripts attempt to parse output. While we will not commit to a formal versioning policy for log
message changes, we will always bump the minor version of the tools when there are logging changes.

At the moment, there are no pre-release (alpha, beta, rc, etc.) versions of the Tools. If there is a
need to support pre-release versions of the Tools, we will need to update our release infrastructure
to support them.

## Supporting New Server Versions

If adding support for a new server version, the Tools version should be able to be pushed to
server's linux repo. This can be done by adding the server linux repo to `linuxRepoVersionsStable`
in `release.go`.

## Releasing

This section describes the steps for releasing a new version of the Tools.

### Pre-Release Tasks

Complete these tasks before tagging a new release.

#### Make Sure the Jira "Release" Exists

Jira has a concept of "releases" separate from tickets. These releases are what appear in the drop
down for the "Fix Version/s" field for tickets. Tickets should be assigned to a release when they're
closed.

Before you start the release process, you need to
[make sure a release exists in the project](https://jira.mongodb.org/projects/TOOLS?selectedItem=com.atlassian.jira.jira-projects-plugin%3Arelease-page&status=unreleased).
Typically, the DB Tools project has a release called "patch-next" which people assign closed tickets
to. You can rename this to the version number of the next release and create a new "patch-next".

It's also possible that a release for the version you're going to release already exists, in which
case you can just use that.

#### Start Release Ticket

Sometimes this ticket already exists. Sometimes you need to create it. The release ticket is a
ticket in the TOOLS project with the "Issue Type" set to "Release". If it doesn't exist, create one
and give it a title like "Release DB Tools 100.19.0".

Now move the ticket for the release to the "In Progress" state. Ensure that its "Fix Version/s"
field matches the version being released.

#### Check for Outstanding Vulnerabilities in Dependencies

See [our internal wiki](https://wiki.corp.mongodb.com/pages/viewpage.action?pageId=276642867) for
more details on how we handle third-party vulnerabilities, in particular
[our vulnerability issue lifecycle](https://wiki.corp.mongodb.com/pages/viewpage.action?pageId=285096119).

We want to make sure that we have taken action on all reported vulnerabities in third-party
dependencies before release. To find these, we should look for
[TOOLS tickets linked to VULN tickets](https://jira.mongodb.org/issues/?filter=62497).

Ideally, all of these tickets should have the "Remediation Pending Release" status. However,
sometimes there may not be a version of the dependency available that addresses the vulnerability.
In that case, it's okay to do a release with the vulnerability still present in the dependency we
use.

We have an informal SLA for releasing an updated version of the Database Tools to address
_applicable_ vulnerabilities in dependencies, based on the issue's severity. It's possible that a
vulnerability is not applicable because the Database Tools code does not use the code path that
leads to the vulnerability. This timeline **starts when the upstream fix is available, not when the
issue is discovered**. The timeline for each severity level is as follows:

- Critical or High severity - 30 days
- Medium - 90 days
- Low or None - no SLA

If possible, we do not want to make a release with any known, applicable issues at the High or
Critical severity levels, even if this would not violate our SLA.

We would also like to avoid releasing with known, applicable issues at the Medium severity level,
but these can be deferred at the team's discretion.

#### Create the SBOM and SARIF Report Files for the Upcoming Release

Run `scripts/generate-release-sbom-snapshot.sh <tag>`, replacing `<tag>` with the tag for this
release (e.g. `100.12.2`). This writes `ssdlc/<tag>.bom.json` (a fresh SBOM) and
`ssdlc/<tag>.sarif.json` (a fresh gosec SARIF report). Neither the SBOM nor the SARIF report is
checked into the repo outside of these per-release snapshots — both are generated from scratch by
this script, not copied from some other checked-in file.

Generating the SBOM requires Podman and access to the DevProd Platforms ECR registry (which hosts
the Silkbomb tool used to enrich the SBOM's license data). You must have an AWS profile named
"ECRScopedAccess-901841024863" configured. The config in your `~/.aws/config` file should look like
this:

```toml
[profile ECRScopedAccess-901841024863]
region = us-east-1
sso_account_id = 901841024863
sso_role_name = ECRScopedAccess
sso_start_url = <same as other profiles or ask in Slack>
sso_region = us-east-1
```

In order to actually log in to this profile, run this command:

```
aws sso login --profile ECRScopedAccess-901841024863
```

In order for this to succeed, you must be a member of the `devprod-platforms-ecr-users` Mana group.

You must commit both resulting files before you can do a release.

#### Merge the SBOM and SARIF Files to `master`

You will need to create a branch with these file updates and merge this to `master` before
triggering the release. You can use the release ticket as the ticket for this branch.

#### Ensure All Static Analysis Checks Pass

The easiest way to do this is to run our linting, which includes `gosec`:

`go run build.go sa:lint`

If `gosec` reports any vulnerabilities, these must be addressed before release. See our
[documentation on contributing](./CONTRIBUTING.md) for more details on how we handle these
vulnerabilities.

#### Ensure Evergreen is Passing

Ensure that the build you are releasing is passing the tests on the evergreen waterfall. A
completely green build is not mandatory, since we do have flaky tests; however, failing tasks should
be manually investigated to ensure they are not actual test failures.

#### Check the Release in Jira for Incomplete Tickets and Update Ticket `"Fix Version/s"` Fields

Go to the
[Tools releases page](https://jira.mongodb.org/projects/TOOLS?selectedItem=com.atlassian.jira.jira-projects-plugin%3Arelease-page&status=unreleased),
and ensure that all the tickets in the "Fix Version/s" to be released are closed. Ensure that all
the tickets have the correct type. Take this opportunity to edit ticket titles if they can be made
more descriptive. The ticket titles will be published in the changelog.

If you are releasing a patch version but a ticket needs a minor bump, update the "Fix Version/s" to
be a minor version bump. If you are releasing a patch or minor version but a ticket needs a major
bump, stop the release process immediately.

The only uncompleted tickets in the release should be the release ticket and third-party
vulnerability tickets in the "Remediation Pending Release" status. If there are any remaining
tickets that will not be included in this release, remove the "Fix Version/s" and assign them a new
one if appropriate.

#### Update the Release Ticket

Mark the release ticket as "Docs Changes Needed". In "Docs Changes Summary", indicate that the
release notes will be found in CHANGELOG.md after the release ticket is closed.

### Triggering the Release

#### Major Release

There are some parts of the release infrastructure that will need to be changed in order to support
a major release. Those changes will need to be made before we can do a new major release. At the
time when the major release process is formalized, this section will be replaced with more specific
instructions.

#### Minor/Patch Release

##### Ensure `master` Is Up to Date

Ensure you have the `master` branch checked out, and that you have pulled the latest commit from
`mongodb/mongo-tools`.

##### Create the Tag and Push

Create an annotated tag and push it:

```
git tag -a -m vX.Y.Z X.Y.Z
git push --tags
```

It's important to use an _annotated_ tag and not a lightweight tag. A lightweight tag will not have
its own metadata and may break the release process. Also ensure you are pushing the tag to the
`mongodb/mongo-tools` repository and not to your fork. If necessary, you may find the correct remote
using `git remote -v` and specify it via `git push <remote> --tags`.

Pushing the tag should trigger an Evergreen version that can be viewed on the
[Database Tools Waterfall](https://evergreen.mongodb.com/waterfall/mongo-tools). If it doesn't, you
may have to ask a project manager/lead to give you the right permissions to do so. The permissions
needed are evergreen admin and github authorized user.

##### Set Evergreen Priorities

Some evergreen variants (particularly zSeries and PowerPC variants) may have a long schedule queue.
To speed up release tasks, you can set the task priority for any variant to 70 for release
candidates and 99 for actual releases.

#### Handling Release Task Failures

Sometimes you may start the release process only to discover that some tasks that are part of the
release, like `push`, fail. If the fix for these failures is to make changes in the repo, you need
to partially restart the release process. Here are the steps to follow:

1. Cancel the tasks still running for the release in Evergreen.
2. Fix the issue in the repo and merge the fix to master.
3. Delete the task from the `origin` remote (GitHub):
   ```
   $> git push origin --delete 100.5.4
   ```
4. Make a new tag and push it as you do for the normal release process.

Evergreen should kick off a new set of tasks for the release. Then you can continue the normal
release process from there.

### Post-Release Tasks

Complete these tasks after the release builds have completed on evergreen.

#### Verify Release Downloads

Go to the [Download Center](https://www.mongodb.com/try/download/database-tools) and verify that the
new release is available there. Download the package for your OS and confirm that
`mongodump --version` prints the correct version.

#### Update Homebrew Tap

In order to make the latest release available via our Homebrew tap, submit a pull request to
[mongodb/homebrew-brew](https://github.com/mongodb/homebrew-brew/blob/bb5b57095a892daeb2700f1a9440550f8e87505b/Formula/mongodb-database-tools.rb#L7-L13)
for both `x86` and `arm64`. You can get the sha256 sum locally using
`shasum -a 256 <tools zip file>`. Tag `@mongodb/devprod-test-infrastructure` in a comment on the PR
asking them to review this.

#### Update the changelog

- Checkout a new branch.
- Copy the following to the top of CHANGELOG.md under the title:

```
## X.Y.Z

_Released YYYY-MM-DD_

We are pleased to announce version X.Y.Z of the MongoDB Database Tools.

<INSERT-DESCRIPTION>

The Database Tools are available on the [MongoDB Download Center](https://www.mongodb.com/try/download/database-tools).
Installation instructions and documentation can be found on [docs.mongodb.com/database-tools](https://docs.mongodb.com/database-tools/).
Questions and inquiries can be asked on the [MongoDB Developer Community Forum](https://developer.mongodb.com/community/forums/tags/c/developer-tools/49/database-tools).
Please make sure to tag forum posts with `database-tools`.
Bugs and feature requests can be reported in the [Database Tools Jira](https://jira.mongodb.org/browse/TOOLS) where a list of current issues can be found.

<INSERT-LIST-OF-TICKETS>
```

- Update the release date to the date the release-json task finished on Evergreen in Eastern Time.
  You can set your timezone in "User Settings".

- Go to
  [Configure Release Notes](https://jira.mongodb.org/secure/ConfigureReleaseNote.jspa?projectId=12385)
  on Jira. Choose the version you are releasing and HTML as the style. This will show you the list
  of tickets tagged with the release version. (If the link doesn't work, you can access this through
  the release page for the version you are releasing.)
- Go through the list of tickets and check that each ticket is categorized correctly (as a task,
  bugfix etc.). Make sure that all tickets marked as `Mongo Internal` for their "Security Level"
  field are excluded from the release notes, _except_ for tickets created for third-party
  vulnerabilities. These vulnerability tickets will be linked to a corresponding ticket in the
  internal-only "VULN" Jira project.
- Make sure there is nothing in the list that might have been tagged with the wrong fix version.
- Copy the HTML list of tickets from Jira and paste it in CHANGELOG.md in place of
  `<INSERT-LIST-OF-TICKETS>`.
- Remove the top line of the list of tickets that says
  `Release Notes - MongoDB Database Tools - Version X.Y.Z`
- Change the ticket type titles from `<h2>`s to `<h3>`s. For example,

  ```
  <h2>        Build Failure
  </h2>
  ```

  Becomes:

  ```
  ### Build Failure
  ```

- Insert a brief description of the release in place of `<INSERT-DESCRIPTION>`. Don't go into too
  much unnecessary detail.
- Submit a PR with your changes under the release ticket number, request reviews from the TAR Team
  Leads and the DB Tools Product Manager. Merge once approved.

#### Mark the Jira Release as "released" and Close Release Ticket

Close the [release on Jira](https://jira.mongodb.org/projects/TOOLS/versions), adding the current
date (you may need to ask the TOOLS project manager to do this). Once this is done, move the Jira
ticket tracking this release to the "Closed" state.

#### Ensure Downstream Tickets Created

Ensure that downstream tickets have been created in the CLOUDP/DOCSP projects and linked to the
release ticket.

#### Mark the Release as Released in Jira

Go to the
[project's list of releases](https://jira.mongodb.org/projects/TOOLS?selectedItem=com.atlassian.jira.jira-projects-plugin%3Arelease-page&status=unreleased).
Click on the version you just released. Then click the "Release" button in the upper right and click
"Release" in the pop-up window.

#### Confirm that VULN-Linked Tickets Were Updated Properly

Any tickets that were in the release that were in the "Remediation Pending Release" state should be
automatically transitioned to "Remediation Complete" when the release is marked as done in Jira. You
can confirm this by searching for tickets in these two states and making sure that they have the
expected status.

VULN-linked tickets related to third party vulnerabilities should also have been updated
automatically to set the "Security Level" to None once the Fix Version is marked as Released.

#### Announce the release

Copy your entry from CHANGELOG.md and post it to [/r/mongodb](https://www.reddit.com/r/mongodb/).
Also post it in the #mongo-tools slack channel to announce it internally.
