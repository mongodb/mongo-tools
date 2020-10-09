# How to vendor Tools changes into Server Repo
### N.B.  We are no longer vendoring tools to server master.  Only vendor to v4.2, v4.0, and v3.6.  Vendoring to v4.0 and v3.6 should only be done for critical/security issues.

Vendoring Tools into Server is relatively straightforward:

- Cherry-pick the required commits into the "vX.Y-master" branch. Run Evergreen patch build and make sure all the tests passed. Then create a PR and paste the evergreen link in the description.
- After the PR is approved and merged, for any Tools branch to vendor to server, merge the "vX.Y-master" branch into "vX.Y", or some intermediate point.  (E.g. v4.2-master is the tip of v4.2 backports, but v4.2 trails behind and is the "stable" v4.2 branch that will be merged to server v4.2). 
  > You should create another Pull request for the merging. But remember since Github doesn't support fast forward option in merge, after the PR is approved, please close the PR and manually merge by running following command in vX.Y branch. Then check these two branches are identical before pushing.
  >>  git merge --ff-only vX.Y-master 
- Push “vX.Y” branch 
- Update (git pull) the kernel tools repository (these instructions assume it's located at `~/git/mongo/kernel-tools`
- Navigate to the mongodb server repo
- Checkout the branch you want to import into and make sure it's pulled to the latest head (examples, "v4.2", "v4.0", "v3.6", etc.)
- Execute `~/git/mongo/kernel-tools/vendoring/import_vendor_branch.py tools` to import tools; this will leave you in an editor for a commit message  
Note: if you have your origin remote set to git@github.com:mongodb/mongo, this will not work. It must be set to git@github.com:mongodb/mongo.git because the script checks that string for direct equality.
- Make sure to git fetch the tools remote.
- Tidy up the commit message if you want  
IMPORTANT: Remove any commits that aren't actually resolved in the TOOLS -- e.g. turning off tests pending a later fix
- Also remove any commits for tickets that were already "released" (i.e. tagged to a previous server version)
- Check whether etc/evergreen.yml needs any changes due to changes in the build system
- Generally this is no longer needed now that everything uses set_gopath.sh 
- Patch build the "bang" (!) build variants for the "tool" tasks. E.g.:
  > `evergreen patch -p mongodb-mongo-v4.2`  
                                                                          
  Note: you will likely need to set upstream: git branch --set-upstream-to origin/v4.2
- If the patch build works, rebase and queue the commit
  > `git pull --rebase`  
  `evergreen commit-queue merge --project mongodb-mongo-v4.2`
- After the commit is accepted, update the fix versions on the tools tickets.
