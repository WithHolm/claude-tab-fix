# claude-tab-fix

<p align="center">
  <img src="logo.png" alt="claude-tab-fix logo" width="256"/>
</p>

A Claude Code hook that warns Claude about tab-indented files — preventing silent Edit failures and mixed-indentation corruption.

## Problem

Claude Code's `Read` tool formats output as `N\t<line content>`, using a tab as the line-number separator. For tab-indented files this is visually identical to the file's own indentation — Claude cannot distinguish the separator tab from content tabs, so it often produces `old_string` with wrong indentation.

Claude Code also validates `old_string` against the file **before** calling PreToolUse hooks. This means hooks cannot fix indentation mismatches automatically — they can only warn Claude so it corrects itself.

## Installation

### prebuilt

go to the right side of this website and look for releases for a bin/exe download. download that. Save the exe in a place where it is safe from deletion, and set PATH to point to this folder. 

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

The plugin registers both hooks for you — no manual config editing needed.

### Manually

Install the binary, then add the hooks to your Claude Code settings.

**Per-project** — commit `.claude/settings.json` to your repo so anyone who clones it gets the hooks automatically:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [{ "type": "command", "command": "claude-tab-fix" }]
      },
      {
        "matcher": "Write",
        "hooks": [{ "type": "command", "command": "claude-tab-fix" }]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "Read",
        "hooks": [{ "type": "command", "command": "claude-tab-fix" }]
      }
    ]
  }
}
```

**Globally** — add the same block to `~/.claude/settings.json` to enable it for every project on your machine.

If the binary is not on your `PATH`, use the full path: `$(go env GOPATH)/bin/claude-tab-fix`.

## Nice Tip

Add explicit deny for the different commands claude normally uses for edit in the claude settings. the file for your current porject should be in `<project root>/.claude/settings.json` (or `settings.local.json`) or in your `home/.claude...`

for linux the folling commands are recomended (just remove the python3 if you do any python work..)
``` json
  "deny": [
    "Bash(sed:*)",
    "Bash(awk:*)",
    "Bash(tr:*)",
    "Bash(xargs:*)",
    "Bash(python3:*)"
  ],
```

**missing info from windows.. if anyone have any info here, please open a pr or write a issue and il add it..** 


when combined with the added context this tool gives ("hey claude, please remember that the read tool is kinda borked...") it forces claude to fall back into the edit path instead of using any other edit tools:
![hey](example.png)



## How it works

**PostToolUse / Read** — after every `Read` call on a tab-indented file, the hook injects a context note reminding Claude that the `N\t` line-number prefix is a separator tab, not part of the file content. Claude uses this information to construct correct `old_string` with proper tab indentation.

**PreToolUse / Bash** — before `sed`, `awk`, `perl`, or `python` commands that target a tab-indented file, the hook warns Claude to use the `Edit` tool instead, since shell commands bypass indentation awareness.

**PreToolUse / Write** — before overwriting a tab-indented file, the hook checks whether the new content uses a different indent style. If writing spaces to a tab file (or vice versa), it warns that the result will have mixed indentation and strongly suggests using `Edit` instead.

## Edge cases handled

| Situation | Behaviour |
|---|---|
| File doesn't exist | Pass through |
| Binary file | Pass through |
| File uses spaces | Pass through (no warning needed) |
| `Bash` on non-file-edit command (`go test`, etc.) | Pass through |
| `Write` with matching indent style | Pass through |
| `Read` on non-tab file | Pass through |

The hook fires on `Read` (PostToolUse), `Bash`, and `Write` (PreToolUse). A `CLAUDE.md` file is included in the release archive; placing it in your project root reinforces tab-awareness at session start.

## Development

```sh
make build   # build ./claude-tab-fix binary
make fmt     # run gofmt
make install # go install
go test ./...
```
