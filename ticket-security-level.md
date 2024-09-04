# Tools Tickets Security Level

This document describes the security level for Jira tickets under the
[TOOLS project](https://jira.mongodb.org/projects/TOOLS/issues). Every TOOLS ticket is by default
viewable by public users, unless the `Security Level` field on the ticket is set to
`Mongo Internal`. Once a ticket is marked as `Mongo Internal`, it cannot be viewed by external users
and should be excluded from the [changelog](CHANGELOG.md) when it's included in a release. This
document describes which tickets should be internal vs external.

## Investigation of HELP Tickets

TOOLS investigation ticket of HELP tickets are created with a copy of the full ticket description
from the HELP ticket, which might contain links or logs specific to customers or internal teams of
MongoDB. These tickets **must always be marked as `Mongo internal`**.

## Internal Processes

Tickets created for an internal process change, for example a change in our release process
involving internal tools, should be marked as `Mongo internal`. Some past examples include:

- [TOOLS-3642](https://jira.mongodb.org/browse/TOOLS-3642)
- [TOOLS-3644](https://jira.mongodb.org/browse/TOOLS-3644)

## Vulnerabilities

Vulnerabilities are created as internal by default. However, once they're resolved, whether they've
been fixed or marked as a false positive, they should be publicly viewable **after** the fix is
released. Our [release process](RELEASE.md) requires the release manager to include them in the
[changelog](CHANGELOG.md) and to update their `Security Level` to make them public after they are
included in a release. Examples include:

- [TOOLS-3615](https://jira.mongodb.org/browse/TOOLS-3615)
