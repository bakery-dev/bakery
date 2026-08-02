# Bakery CLI Schemas

This document defines the schemas for the configuration and state files used by the Bakery CLI's registry-backed scaffolding system.

## 1. piece.yml

The `piece.yml` file must be located in the root of a piece's directory (e.g., `pieces/react/piece.yml`). It serves as the source of truth for the piece's metadata, dependencies, and interactive prompt configuration.

```yaml
# piece.yml
name: "react"
description: "React frontend foundation using Vite"
author: "bakery-dev team"

# (Optional) Capabilities this piece provides to satisfy dependencies of other pieces.
# This is crucial for mutually exclusive dependencies (e.g., React vs Angular).
provides:
  - "frontend-framework"

# (Optional) Pieces or capabilities this piece requires to function.
# Uses the Git ref syntax: [user]/[repo]/[tag_or_commit]:[piece]
depends_on:
  - "bakery/core/v2:vite"

# (Optional) Predefined ecosystem actions to execute post-scaffold.
# Arbitrary bash commands are explicitly forbidden for security. Instead, request
# predefined actions handled securely by the CLI executor (e.g., go:tidy, pnpm:install).
actions:
  - "pnpm:install"

# (Optional) The configuration for the interactive `huh` prompt.
# If omitted, the piece is considered a mandatory/silent inclusion if its dependencies are met or if it's explicitly listed in a piefile.
prompt:
  type: "confirm" # Supported types: confirm, select, multi_select, input
  title: "Do you want to initialize a React frontend?"
  default: true
  # For 'select' or 'multi_select' types:
  # options:
  #   - label: "Yes, include React"
  #     value: true
```

## 2. piefile.yaml

A Piefile defines a bundle of pieces (a "Pie") and optionally pre-fills the answers to their prompts. 
When placed in a registry's `pies/` directory, it acts as a scaffolding template (e.g., `pies/web.yml`). When a project is generated, a finalized `piefile.yaml` containing the user's actual answers is written to the project root.

```yaml
# piefile.yaml
name: "web"
description: "Standard Web Application (Go backend + React frontend)"

# The list of pieces included in this Pie and their corresponding answers.
pieces:
  "bakery/core:cobra":
    answer: true # Mandatory pieces can simply have their answer pre-filled
  
  "bakery/core:slog":
    answer: true
  
  "bakery/frontend:react":
    # If no answer is provided here, the CLI will prompt the user (unless --defaults is passed)
    answer: null 
  
  "bakery/database:sqlc":
    answer: true
```

## 3. pielock

The `pielock` file is generated automatically by the CLI in the scaffolded project's root. It stores the exact resolved Git commit hashes of all fetched pieces to guarantee reproducible builds and enable future `bakery upgrade` operations without breaking when remote branches move forward.

```yaml
# pielock
resolved:
  "bakery/core:cobra":
    commit: "3a5f8b9e4c1d2e3f..."
  "bakery/core:slog":
    commit: "7b4c3d2e1f0a9b8c..."
```

## 4. pieces.lock

The `pieces.lock` file must be present in the root of any registry repository (both remote Git repos and local directories). It provides deterministic integrity verification for the pieces within that registry.
Instead of hashing individual files, it stores a single stable hash for the entire directory of each piece, computed using a stable algorithm (like `golang.org/x/mod/sumdb/dirhash`). 
Pie files (`pies/*.yaml`) are explicitly excluded from hashing as they are just simple configuration bundles.

```json
{
  "golang": "h1:...",
  "cobra": "h1:..."
}
```
The CLI provides a `bakery repo lock` command to compute and generate this file automatically.
