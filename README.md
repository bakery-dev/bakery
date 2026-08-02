# Bakery

> Project scaffolding powered by composable pieces.

Bakery assembles new projects from modular **pieces** (templated overlays)
bundled into **pies** (curated bundles), fetched from git-based
**registries**. The CLI fetches pieces resolved to exact commits, prompts you
interactively (or runs fully non-interactively), renders every file through
Go's `text/template`, and runs native tooling (`go mod tidy`, …) at the end.

```sh
bakery init core:cli my-app      # scaffold the `cli` pie into ./my-app
bakery repo add core github:bakery-dev/bakery   # register a registry
bakery update                    # refresh cached registries
```

---

## Installation (NixOS / Home Manager)

Bakery ships a Nix flake with a **default overlay** that exposes the `bakery`
package. The standard flow: add the flake to your inputs, apply the overlay,
then add `bakery` to your packages.

### 1. Add the flake input

In your `flake.nix`:

```nix
{
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";

    bakery = {
      url = "github:bakery-dev/bakery";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };
}
```

### 2. Apply the overlay

Pass the overlay into your `nixpkgs` instantiation so `pkgs.bakery` exists:

```nix
nixpkgs.overlays = [ inputs.bakery.overlays.default ];
```

### 3a. System packages (NixOS)

```nix
environment.systemPackages = [ pkgs.bakery ];
```

### 3b. Home Manager

```nix
home.packages = [ pkgs.bakery ];
```

That's it — `bakery` is now on your `PATH`.

> **Tip:** Using `inputs.nixpkgs.follows = "nixpkgs"` makes bakery build
> against the same nixpkgs revision as the rest of your system, avoiding a
> duplicate nixpkgs download and extra builds.

---

## Quick start (ad-hoc)

No need to wire it into your flake just to try it:

```sh
# Run once, no install
nix run github:bakery-dev/bakery -- --help

# Install into your user profile
nix profile install github:bakery-dev/bakery
```

---

## Usage

Bakery follows a registry → pie → piece model.

| Command | Description |
| --- | --- |
| `bakery init [<pie>] <project>` | Scaffold a new project from a pie. |
| `bakery repo add <alias> <url>` | Register a piece registry. |
| `bakery repo remove <alias>` | Remove a registered registry. |
| `bakery update [alias]` | Fetch latest commits for one or all registries. |

### Flags

| Flag | Description |
| --- | --- |
| `--log-level <DEBUG\|INFO\|WARN\|ERROR>` | Structured log level (default `INFO`). |
| `--defaults` | Skip interactive prompts, use default answers. |
| `--piefile <path>` | Scaffold non-interactively from an existing `piefile.yaml`. |
| `--registry alias=path` | Override a registry alias with a local path or URL (repeatable). |

### Scaffolding

```sh
# Interactive: resolve pie, answer questions, render, run actions
bakery init core:cli my-app

# Non-interactive, all defaults
bakery init core:cli my-app --defaults

# From a saved piefile (fully reproducible)
bakery init my-app --piefile ./piefile.yaml
```

### Registries

Registries are git repos (or local directories) exposing `pieces/`, `pies/`,
and a mandatory `pieces.lock` checksum file. Local dirs are read directly with
no caching — handy for developing pieces.

```sh
bakery repo add core   github:bakery-dev/bakery
bakery repo add local  file:///home/me/pieces-repo
bakery update core
```

Piece references embed their source: `user/repo:piece`,
`user/repo/ref:piece`, or `alias:piece` for a configured alias.

### Configuration

Bakery stores global config and caches under XDG directories:

| | Linux | macOS |
| --- | --- | --- |
| Config | `$XDG_CONFIG_HOME/bakery/config.yaml` | `~/Library/Application Support/bakery/config.yaml` |
| Data (registry clones) | `$XDG_DATA_HOME/bakery` | `~/Library/Application Support/bakery` |
| Cache | `$XDG_CACHE_HOME/bakery` | `~/Library/Caches/bakery` |

Windows is not supported.

---

## Development

A pre-configured dev shell is available via the flake:

```sh
nix develop     # or: direnv allow   (see .envrc)
```

It provides `go`, `gopls`, `gotools`, `git`, and a `golangci-lint` pinned to
the version recorded in `.golangci-version`.

Build and test:

```sh
go build ./...
go test ./...
golangci-lint run
```

See [`docs/design.md`](docs/design.md) for the architecture and
[`docs/requirements.md`](docs/requirements.md) for the full feature spec.

---

## License

[MIT](LICENSE) © bakery-dev contributors.
