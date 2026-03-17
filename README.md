# claude-tab-fix

A Claude Code `PreToolUse` hook that fixes indentation mismatches in `Edit` tool calls before they reach the file system.

## Problem

Claude Code's `Edit` tool matches `old_string` against file content as a literal string. When the model generates `old_string` with spaces but the file uses tabs (or vice versa), the match fails silently and no edit is applied. This is a persistent issue with Go, `.templ`, and other tab-indented files.

## Installation

### prebuilt

go to the right side of this website and look for releases for a bin/exe download. download that


## via go

```sh
go install github.com/WithHolm/claude-tab-fix@latest
```

### build from source

```sh
git clone https://github.com/WithHolm/claude-tab-fix
cd claude-tab-fix
make install
```

## Setup

### As a plugin (recommended)

Install the binary first, then install the plugin so Claude Code picks up the hook automatically:

```sh
# 1. Install the binary
go install github.com/WithHolm/claude-tab-fix@latest

# 2. Install the plugin
/plugin marketplace add WithHolm/claude-tab-fix
/plugin install claude-tab-fix@WithHolm/claude-tab-fix
```

The plugin registers the `PreToolUse` hook for you — no manual config editing needed.

### Manually

Install the binary, then add the hook to your Claude Code settings.

**Per-project** — commit `.claude/settings.json` to your repo so anyone who clones it gets the hook automatically:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Edit",
        "hooks": [
          {
            "type": "command",
            "command": "claude-tab-fix"
          }
        ]
      }
    ]
  }
}
```

**Globally** — add the same block to `~/.claude/settings.json` to enable it for every project on your machine.

If the binary is not on your `PATH`, use the full path: `$(go env GOPATH)/bin/claude-tab-fix`.

## How it works

Before every `Edit` tool call, the hook:

1. Reads the target file
2. Detects the dominant indent style (tabs or spaces, and width if spaces)
3. Detects the indent style used in `old_string`
4. If they differ, re-indents both `old_string` and `new_string` to match the file
5. Returns the corrected strings to Claude Code via `updatedInput`

If the indentation already matches, the hook passes through unchanged and the original edit proceeds normally.

## Edge cases handled

| Situation | Behaviour |
|---|---|
| File doesn't exist yet | Pass through (new file creation) |
| Binary file | Pass through |
| `old_string` has no indented lines | Pass through |
| Mixed indentation in file | Majority-wins detection |
| `replace_all: true` | Same normalization applies |

## Development

```sh
make build   # build ./claude-tab-fix binary
make fmt     # run gofmt
make install # go install
go test ./...
```
