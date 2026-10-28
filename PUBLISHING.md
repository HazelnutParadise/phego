# Publishing the prepared documentation repository

Target: `HazelnutParadise/phego`. License: MIT. README: English. Default branch: `main`.

The prepared files and a local commit are not proof that a GitHub repository has been created. Use the delivery report to determine whether remote publication was actually verified.

## From the working-tree ZIP

Install Git, Python 3.10 or later, and the GitHub CLI. Authenticate the CLI with an account authorized to create repositories in HazelnutParadise. Configure a suitable Git author identity if the local repository has no commit yet.

From the extracted `phego` directory, choose visibility explicitly and run:

```sh
bash tools/publish-repository.sh --public
```

Use `--private` instead when a private repository is intended. The script validates documentation, checks authentication, creates the target only when necessary, refuses existing remote history, pushes without force, and verifies the remote commit. It never prints credentials. It does not change the visibility of an existing repository.

The script's GitHub operations require the user's local authorized account. The documentation validation is executable now; remote creation and push must be reported separately after they really succeed.

## From the Git bundle

The delivery package also includes a self-contained Git bundle. Restore the prepared commit using:

```sh
git clone phego-spec-v1.1.2.bundle phego
cd phego
git remote remove origin
bash tools/publish-repository.sh --public
```

The removed origin in this sequence is only the local bundle path created by `git clone`; it is not an existing GitHub repository.

## When the target already exists

Do not run a force push. Fetch and inspect the target history, read existing content, and reconcile changes through a normal commit or pull request. The helper deliberately stops for a nonempty remote, so it cannot overwrite someone else's initialization or later work.

The GitHub CLI's official repository-creation documentation is available at [gh repo create](https://cli.github.com/manual/gh_repo_create).
