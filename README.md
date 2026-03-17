# claude-tab-fix

A Claude Code `PreToolUse` hook that fixes indentation mismatches in `Edit` tool calls before they reach the file system.

## Problem

Claude Code's `Edit` tool matches `old_string` against file content as a literal string. When the model generates `old_string` with spaces but the file uses tabs (or vice versa), the match fails silently and no edit is applied. This is a persistent issue with Go, `.templ`, and other tab-indented files.

## Installation

```sh
go install github.com/WithHolm/claude-tab-fix@latest
```

Or build from source:

```sh
git clone https://github.com/WithHolm/claude-tab-fix
cd claude-tab-fix
make install
```

## Setup

Register the hook in your Claude Code settings. You can do this globally (`~/.claude/settings.json`) or per-project (`.claude/settings.json`):

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

If the binary is not on your `PATH`, use the full path returned by `which claude-tab-fix` or `go env GOPATH`/bin/claude-tab-fix`.

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
