# How to run resmoke tests

* Build all tools first; use the 'failpoints' tag and optionally ssl/sasl tags

* From this directory, run `prep-for-resmoke.sh` to copy binaries to this directory.
  Optional arguments are (1) directory to find tools binaries (defaults to `bin`
  directory in repo root), (2) directory to find mongodb binaries (defaults to
  wherever `mongod` is found in your PATH.

* To list test suites: `python buildscripts/resmoke.py -l`

* To run a suite: `python buildscripts/resmoke.py --suites=<suite>`
  * Consider adding `--continueOnFailure` or `--dryRun=tests` as desired

* To run a particular test: `python buildscripts/resmoke.py --executor=core <path/to/test.js>`

# Tips

* `resmoke.py --help` to see options
