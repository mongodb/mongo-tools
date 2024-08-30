# Tools Tickets Security Level

This document describes the security level for Jira tickets under the [TOOLS project](https://jira.mongodb.org/projects/TOOLS/issues). 
Every TOOLS ticket is by default viewable by public users, unless `Security Level` is set to `Mongo Internal`. 
Once a ticket is marked as `Mongo Internal`, it cannot be viewed by external users and should be excluded from the 
[changelog](CHANGELOG.md) after every release. This document outlines what tickets should be internal vs external.

## Investigation of HELP Tickets

TOOLS investigation ticket of HELP tickets are created with full ticket description from the HELP ticket, 
which might contain links or logs specific to customers or internal teams of MongoDB. 
These tickets are **strictly prohibited** from being accessed publicly.

## Internal Processes

Tickets are created for an internal process changes that is not tied with the code base should in general be marked as 
internal. Examples include:
- [TOOLS-3642](https://jira.mongodb.org/browse/TOOLS-3642) 
- [TOOLS-3644](https://jira.mongodb.org/browse/TOOLS-3644)

## Vulnerabilities

Vulnerabilities are created as internal by default. But once they're resolved, either being mitigated or 
being marked as false positive, they should be publicly viewable following a new release. 
Our [release process](RELEASE.md) requires the release manager to mark any resolved vulnerability tickets as public 
and include them in the [changelog](CHANGELOG.md). Examples include:
- [TOOLS-3615](https://jira.mongodb.org/browse/TOOLS-3615)

