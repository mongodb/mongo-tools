To fill this in for a release, make a copy of this document named `ssdlc-compliance-report.$tag.md`
and replace all the templated bits (anything like `$tag`), with actual values. Then delete all the
text before the heading below.

# Mongosync SSDLC Compliance Report

## Release Creator

To determine who triggered the release, go to the releases's branch project at $release_project_url
in Evergreen and find the CI run for the release. This will show the name of the person who
triggered the release by tagging the repo.

## Process Document

See [to be written doc]().

## Tool Used to Track Third-Party Vulnerabilities

We use the [Silk Security platform](https://www.silk.security/) to track third-party-dependencies.

## Third-Party Dependency Information

See the SBOM for this release at
https://github.com/10gen/mongosync/blob/$tag/ssdlc/SBOM.$tag.bom.json.

## Static Analysis Findings

The SARIF report for this branch is visible in the Evergreen logs for the release as part of the
`gosec` task's output.

## Signature Information

Refer to data in the Papertrail service.

## Security Testing Report

Available as needed from the Tools and Replicator team.

## Security Assessment Report

Available as needed from the Tools and Replicator team.

## Known Vulnerabilities

All known vulnerabilities are called out in either the SBOM, the SARIF report, or are identified as
individual Jira tickets in the releases's changelog.
