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
| _(none)_ | Show the current config and hint |
| `--run` | Run the merge+push cycle |
| `--new` | Create a new config for this directory |
| `--del <name>` | Delete a config by name |
| `--update` | Re-select branches for the current config |
| `--name <name>` | Target a specific config by name |
| `--list` | List the configured base and child branches (prints store path) |
| `--add-branch <name>` | Add a branch to the child branches list |
| `--add-current-branch` | Add the currently checked-out branch to the child branches list |
| `--remove <name>` | Remove a branch from the child branches list |

**Examples**

```sh
# Run the sync
belt git sync-child-branches --run

# Target a specific config by name and run
belt git sync-child-branches --name my-config --run

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

### `belt ecs`

AWS ECS utilities.

---

#### `belt ecs env-export`

Interactively selects an ECS cluster, service, and container, then exports its environment variables — including resolved SSM Parameter Store and Secrets Manager references — into a `.env` file.

**Flags**

| Flag | Description |
|---|---|
| `-o, --output <path>` | Output file path (default: `.env.<service-name>`) |
| `--profile <name>` | AWS profile to use (overrides `AWS_PROFILE`) |

**Examples**

```sh
# Interactive export using the default AWS profile
belt ecs env-export

# Use a specific AWS profile
belt ecs env-export --profile staging

# Write to a custom file
belt ecs env-export -o .env.local
```

The command resolves SSM and Secrets Manager ARNs concurrently (up to 5 at a time) and prints a success/failure line per secret.

---

## CI/CD

Pushing to `main` automatically creates a **draft release** on GitHub. When you publish the draft, binaries for Linux (amd64), macOS (amd64 + arm64), and Windows (amd64) are built and attached to the release.
