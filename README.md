# belt

A personal CLI utility belt — a single binary with a growing suite of everyday developer tools.

## Installation

### Download a release

**macOS / Linux:**

```sh
curl -fsSL https://raw.githubusercontent.com/mateusdeitos/cli-utility-belt/main/install.sh | sh
```

**Windows (PowerShell):**

```powershell
irm https://raw.githubusercontent.com/mateusdeitos/cli-utility-belt/main/install.ps1 | iex
```

### Build from source

Requires Go 1.22+.

```sh
git clone https://github.com/mateusdeitos/cli-utility-belt.git
cd cli-utility-belt
go build -o belt .
```

---

## Commands

### `belt git`

Git-related utilities.

---

#### `belt git sync-child-branches`

Merges a set of **base branches** into each **child branch**, then pushes. Useful when multiple feature branches all need to stay in sync with a common set of upstream branches.

All configs are stored in `~/.belt/git-sync-child-branches.json`, keyed by directory path and config name. Multiple named configs can coexist in the same directory.

**First run**

On the first run in a directory, you will be prompted for a name, base branches, and child branches:

```
$ belt git sync-child-branches
No config found for this directory. Let's set it up.
Config name (a short name to identify this config): my-config
Base branches (comma-separated): main, feature-base
Child branches (comma-separated): feature-a, feature-b, feature-c
Config "my-config" saved.
```

After that, `belt git sync-child-branches` will run the full merge+push cycle without prompting.

**Flags**

| Flag | Description |
|---|---|
| _(none)_ | Run the merge+push cycle using the stored config |
| `--name <name>` | Target a specific config by name |
| `--list` | List the configured base and child branches |
| `--add-branch <name>` | Add a branch to the child branches list |
| `--add-current-branch` | Add the currently checked-out branch to the child branches list |
| `--remove <name>` | Remove a branch from the child branches list |
| `--update` | Re-prompt for base and child branches and overwrite the stored config |

**Examples**

```sh
# Run the sync
belt git sync-child-branches

# Target a specific config by name
belt git sync-child-branches --name my-config

# See what's configured
belt git sync-child-branches --list

# Add a new child branch
belt git sync-child-branches --add-branch my-new-feature

# Add whatever branch you're on right now
belt git sync-child-branches --add-current-branch

# Remove a branch that no longer needs syncing
belt git sync-child-branches --remove old-feature

# Change the configuration
belt git sync-child-branches --update
```

---

## CI/CD

Pushing to `main` automatically creates a **draft release** on GitHub. When you publish the draft, binaries for Linux (amd64), macOS (amd64 + arm64), and Windows (amd64) are built and attached to the release.
