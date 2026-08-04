# Bakery CLI - Architectural Design Document

## 1. Overview
This document outlines the software design for the Bakery CLI. The application will be built using a strictly modular architecture where each package encapsulates a single domain of responsibility. This ensures that the codebase remains highly testable, easily debuggable, and extensible.

Every module must accept its dependencies (including its Logger) during initialization (Dependency Injection) rather than relying on global state.

## 2. Directory & Path Specifications (XDG)
To store remote registries, caches, and global configurations, the CLI must adhere to standard OS-specific directories. Windows is explicitly unsupported at this time. We will use a library like `github.com/adrg/xdg` to seamlessly map XDG Base Directory specs on Linux to their standard macOS equivalents.

* **Configuration Directory** (for global overrides, default registries):
    * *Linux:* `~/.config/bakery` (`$XDG_CONFIG_HOME/bakery`)
    * *macOS:* `~/Library/Application Support/bakery`
* **Data Directory** (for cloning and storing remote git registries/pieces):
    * *Linux:* `~/.local/share/bakery` (`$XDG_DATA_HOME/bakery`)
    * *macOS:* `~/Library/Application Support/bakery`
* **Cache Directory** (for temporary downloads/processing):
    * *Linux:* `~/.cache/bakery` (`$XDG_CACHE_HOME/bakery`)
    * *macOS:* `~/Library/Caches/bakery`

## 3. High-Level Modular Architecture

| Module | Driving Requirement | Responsibility |
| :--- | :--- | :--- |
| **`cmd`** | CLI Interface | Cobra entrypoint, flag parsing, subcommands. |
| **`config`** | Config Management | Koanf integration, reading XDG configs and env vars. |
| **`logger`** | Structured Logging | Slog setup with human-readable output, leveled logging. |
| **`registry`** | Remote Repositories | Cloning/updating git repos, parsing `pies/` and `pieces/`. |
| **`resolver`** | Piece Dependency Graph | Resolving capabilities, verifying constraints, building prompt tree. |
| **`prompt`** | Interactive Prompts | Dynamically rendering `huh` forms from resolved `piece.yml` files. |
| **`engine`** | Overlay Templating | Executing `text/template`, copying overlays, writing state files. |
| **`executor`** | Tool Execution | Running native commands (`go mod tidy`, `pnpm install`) post-scaffold. |

---

## 4. Module Specifications

### 4.1. Module: `logger`
* **Purpose:** Initializes the global `slog` instance using a human-readable text handler (e.g., `slog.NewTextHandler` or a custom colored console handler, NOT JSON).
* **Requirements:**
    * Expose standard log levels: `DEBUG`, `INFO`, `WARN`, `ERROR`.
    * Default log level is `INFO`.
    * Must provide a helper function to spawn sub-loggers with a module name (e.g., `logger.With("module", "registry")`).
    * Every other module in the system must be instantiated with a named sub-logger.

### 4.2. Module: `config`
* **Purpose:** Manages global application settings using `koanf`.
* **Requirements:**
    * Reads from CLI flags, environment variables (e.g., `BAKERY_LOG_LEVEL`), and an optional global config file in the XDG Config directory.
    * Parses the `--log-level` flag and passes it back to the `logger` module.
    * Stores global user preferences, such as default fallback registries or a predefined list of allowed files for non-empty directories.

### 4.3. Module: `cmd`
* **Purpose:** The Cobra CLI implementation.
* **Requirements:**
    * Defines the `init <pie> <project-name>` command.
    * Defines `repo add <alias> <url>` and `repo remove <alias>` commands for managing registry sources.
    * Defines `update [alias]` to manually trigger git fetches and populate caches for all or a specific repository.
    * Defines global flags like `--log-level`, `--defaults`, and `--piefile`.
    * Wires all modules together (the Composition Root). It instantiates the logger, config, registry manager, etc., passing dependencies explicitly.

### 4.4. Module: `registry`
* **Purpose:** Handles fetching, caching, and reading Remote Registries.
* **Requirements:**
    * Accepts a logger (`module=registry`).
    * Parses Git URLs (e.g., `user/repo/tag_or_commit`).
    * **Lazy Caching & Updates:** The registry never updates caches automatically. Caches are only populated on the very first use, or when explicitly requested via the `update` command.
    * **Local Registry Support:** The registry must gracefully support local directories (e.g., `file:///path/to/repo` or `/path/to/repo`). For local registries, the module skips caching, fetching, and `pieces.lock` validation entirely, reading directly from the source filesystem to facilitate rapid development and testing. The generated `pielock` will record `local` instead of a Git commit hash.
    * **Commit-based Resolution:** When fetching or updating a remote git repository, the module queries it for the exact commit hash matching the requested ref. It only pulls/clones data if that exact commit hash is missing from the local XDG Data directory.
    * **Cache Integrity & Transport Security:** Generating a hash locally is insufficient to prevent transport corruption or man-in-the-middle attacks. The registry module must rely on authoritative hashes provided by the remote repository.
        * Remote registries are required to have a `pieces.lock` file at their root containing checksums for every piece.
        * During a fetch/update, the registry module must download and read this `pieces.lock` file.
        * It must calculate the hashes of the downloaded template files and strictly compare them against the lockfile.
        * **Fatal Error Condition:** If `pieces.lock` is missing from the remote repository, or if any piece fails the hash validation, the registry module MUST throw a fatal error, delete the corrupted cache, and absolutely refuse to continue scaffolding.
        * Once verified during download, it stores the valid checksum locally to protect against future local disk corruption.
    * Reads and deserializes `piece.yml` files from the `pieces/` directories.
    * Reads and deserializes `*.yml` files from the `pies/` directories.
    * Exposes an API to query available pieces, their metadata, and their local filesystem paths.

### 4.5. Module: `resolver`
* **Purpose:** Evaluates the Piece Dependency Graph.
* **Requirements:**
    * Accepts a logger (`module=resolver`).
    * Takes the requested Pie (which defines a list of mandatory/starting pieces).
    * Traverses the dependencies (`depends_on`) and matches them against pieces or capabilities (`provides`) found in the `registry` module.
    * Detects capability collisions (mutually exclusive dependencies).
    * **Output:** Produces a deterministic, directed acyclic graph (DAG) representing the valid pieces to install, and the dynamic decision tree of questions that need answering.

### 4.6. Module: `prompt`
* **Purpose:** Renders dynamic CLI forms using Charmbracelet's `huh`.
* **Requirements:**
    * Accepts a logger (`module=prompt`).
    * Consumes the decision tree produced by the `resolver`.
    * If a `piefile.yaml` was provided with pre-filled answers (or if `--defaults` is active), it silently skips the corresponding questions.
    * Dynamically shows/hides subsequent questions based on answers provided in real-time.
    * **Output:** Returns a fully answered mapping of pieces that are enabled.

### 4.7. Module: `engine` (Scaffolder)
* **Purpose:** Copies files, renders templates, and writes generated state files.
* **Requirements:**
    * Accepts a logger (`module=engine`).
    * Takes the final list of enabled pieces, their answers, and their exact Git commit hashes (resolved by the registry).
    * **Owns the template context.** The engine is the single source of truth for every variable exposed to templates; it assembles the context (`buildContext`) from three categories:
        * **Well-known variables** — generic, language-agnostic values derived by the engine itself from raw inputs supplied by the `cmd` module (e.g. the target project directory). These are deliberately *not* injected by `cmd`; the command layer passes raw inputs only. The currently supported well-known variable is:
            * `ProjectName` (`string`) — base name of the target project directory.
        * **Piece answer variables** — one entry per piece that was interactively prompted at runtime, keyed by its full piece key (`alias:name`). Values are typed by prompt type (`bool` for confirm, `string` for select/input, `[]string` for multi-select). Because keys contain a colon, they must be referenced with the `index` function, e.g. `{{ index . "core:cobra" }}`. Pieces whose answer is pre-filled in the piefile are currently *not* injected into the context — only pieces actually prompted appear.
        * **`Capabilities`** — a `map[string]bool` that is the union of every active piece's `provides:` list, enabling conditional rendering such as `{{ if index .Capabilities "cli" }}`.
    * **Content rendering:** For each piece, iterates through its `template/` directory in the XDG registry cache and passes every file's *contents* through Go's `text/template` using the context above.
    * **Path rendering (token substitution):** In addition to contents, the *path* of each file — both directory names and the file name — is rendered via simple **variable replacement** of `__Name__` tokens. This is deliberately *not* `text/template`: no functions, pipelines, conditionals, or `{{ }}` syntax are supported in paths, only literal `__Variable__` substitution.
        * Only well-known variables are valid path tokens. The sole supported token today is `__ProjectName__` (e.g. `template/cmd/__ProjectName__/root.go` renders to `cmd/<project-name>/root.go`).
        * **Unknown tokens are a fatal error.** A path containing `__Foo__` where `Foo` is not a known variable aborts scaffolding, which catches typos such as `__ProjecName__`.
        * **An empty rendered segment is a fatal error.** A token that resolves to an empty string, or any segment that is empty, `.`, or `..` after rendering, aborts scaffolding.
        * Substitution is performed per path segment, and a rendered value may never introduce additional path separators (see §5). Piece answer variables are intentionally excluded from path tokens for now (see §6.4).
    * Writes the rendered files into the target `<project-name>` directory.
    * At the end, writes the generated `piefile.yaml` and `pielock` (containing the exact commit hashes) into the target directory root.

### 4.8. Module: `executor`
* **Purpose:** Runs native post-scaffolding shell commands securely.
* **Requirements:**
    * Accepts a logger (`module=executor`).
    * Implements secure, hardcoded "Ecosystem Actions" (e.g., ecosystem `go` supports `tidy`; ecosystem `pnpm` supports `install`).
    * Reads the `actions` requested by the active pieces in their `piece.yml` files.
    * Maps requested actions to their hardcoded implementations and executes them securely in the generated directory. Arbitrary command execution is strictly forbidden.
    * Should stream the output of these commands to the user (or log them as `DEBUG` while showing a spinner, depending on verbosity).

---

## 5. Security Constraints

Given that the CLI fetches templates and configurations from remote repositories, strict security boundaries must be enforced by the underlying modules:

1. **Path Traversal Protection:** Under no circumstances is the `engine` module allowed to write, modify, or delete files outside of the resolved `<project-name>` target directory. Because template paths may now contain rendered variable tokens (`__Name__`), sanitization must be performed **after** rendering: each path segment is validated individually (it must be non-empty, must not be `.` or `..`, and must not contain `/`, `\`, or a NUL byte, so a variable value can never smuggle extra directory depth), and the final resolved destination is confirmed to remain within the target root via `filepath.Clean` plus a robust relativity/prefix check (e.g., blocking `../`).
2. **Symlink Prohibition:** Symlinks are explicitly forbidden inside piece `template/` directories. This prevents malicious pieces from orchestrating symlink-based path traversal attacks against the user's filesystem or the local registry cache.
    * *Validation:* The CLI must proactively scan files for the `os.ModeSymlink` bit before reading or copying templates. If a symlink is detected, the CLI must immediately halt and abort the process with a fatal error.
3. **Ecosystem Action Sandboxing:** As noted in the `executor` module, pieces cannot declare arbitrary bash scripts. They can only request predefined actions which are securely implemented by the CLI.

---

## 6. Open Questions & Considerations

1. **Registry Authentication:** How do we handle fetching registries from private Git repositories? Should we rely on the host's existing `~/.ssh/config` and local Git CLI credentials, or build native Git auth handling via a Go-Git library?
2. **Partial Template Overwrites:** If multiple pieces contain a file with the exact same path (e.g., `main.go`), how does the `engine` handle the collision? Does it overwrite, error out, or do we rely on the `resolver` to prevent capability collisions from creating overlapping files?
3. **macOS XDG Enforcement:** Do we want to strictly use `~/Library/...` on macOS (which is the Apple standard), or force standard Linux `~/.config` across all Unix-like platforms for consistency?
4. **Template Piece State Lookups:** The templating engine will receive the resolved piece state, but how exactly should templates query this state safely? Since there can be multiple pieces with the same name from different repositories, writing full repo URLs inside template `{{ if }}` blocks might be brittle and unergonomic. We need a clean abstraction for this.
    * *Update:* The variable source is now centralized in the engine's `buildContext` (see §4.7). Piece answers are keyed by full `alias:name` and referenced via `{{ index . "alias:name" }}`; the `Capabilities` map covers the common "is this capability present?" case. **Path templating** is intentionally restricted to a small set of well-known, generic variables (`ProjectName` today) via `__Name__` tokens. Piece-declared variables (which would let, e.g., a Go piece expose a `ModuleName`) are a deliberate future extension and remain out of scope.
