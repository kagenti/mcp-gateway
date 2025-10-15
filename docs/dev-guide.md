# Developer's Guide

## Working with Git

### Setting up your local repo

1. Create a [fork of mcp-gateway](https://github.com/kagenti/mcp-gateway/fork)

2. Clone your fork - either using HTTPS or SSH 

Via HTTPS:

```shell
git clone https://github.com/<your-username>/mcp-gateway.git
cd mcp-gateway
```

Via SSH:

```shell
git clone git@github.com:<your-username>/mcp-gateway.git
cd mcp-gateway
```

3. Add the upstream repository as a remote - either using HTTPS or SSH 


Via HTTPS:
```shell
git remote add upstream https://github.com/kagenti/mcp-gateway.git
```

Via SSH:
```shell
git remote add upstream git@github.com:kagenti/mcp-gateway.git
```

4. Fetch all tags from upstream

```shell
git fetch upstream --tags
```

### Making a PR

Work on your local repo cloned from your fork. Create a branch:

```shell
git checkout -b <name-of-your-branch>
```

When ready to make your PR, make sure first to rebase from upstream
(things may have changed while you have been working on the PR):

```shell
git checkout main; git fetch upstream; git merge --ff-only upstream/main
git checkout <name-of-your-branch>
git rebase main
```

Resolve any conflict if needed, then you can make your PR by doing:

```shell
git commit -am "<your commit message>" -s
```

Note that commits must be all signed off to pass DCO checks.
It is reccomended (but not enforced) to follow best practices
for commits comments such as [conventional commits](https://www.conventionalcommits.org/en/v1.0.0/).

Push the PR:

```shell
 git push --set-upstream origin <name-of-your-branch>
 ```

 Open the URL printed by the git push command for the PR and complete the PR by
 entering all the required info - pay attention to the type of PR indicator that goes
 at the start of the title, a meaningful description of what the PR does
 and possibly which issue is neing fixed.


### Tagging and triggering a build for new tag

Note - this is only enabled for maintainers for the project.

Checkout `main` and make sure it equals `main` in the upstream repo as follows:

if working on a fork and "upstream" is the name of the upstream remote (commmon convention)

```shell
git checkout main; git fetch upstream; git merge --ff-only upstream/main
```

if a maintainer using a branch upstream directly (not reccomended)

```shell
git checkout main; git pull
```

check existing tags e.g.,

```shell
git tag
v0.0.1-alpha.1
v0.0.2-alpha.1
...
v0.0.4-alpha.9
```

create a new tag e.g.

```shell
git tag v0.0.4-alpha.10
```

Push the tag upstream

```shell
git push upstream v0.0.4-alpha.10
```