# indent-normalize-hook

A Claude Code `PreToolUse` hook that fixes indentation mismatches in `Edit` tool calls before they reach the file system.

## Problem

Claude Code's `Edit` tool matches `old_string` against file content as a literal string. When the model generates `old_string` with spaces but the file uses tabs (or vice versa), the match fails silently and no edit is made. This is a persistent issue with `.templ`, Go, and other tab-indented files.

## Solution

A small Go binary that sits as a `PreToolUse` hook on the `Edit` tool. Before every edit:

1. Read the target file
2. Detect the dominant indent character (tab or space, and width if spaces)
3. Detect the indent character used in `old_string` and `new_string`
4. If they differ, re-indent both strings to match the file
5. Return the corrected `updatedInput` to Claude Code via stdout

## Hook contract

**stdin** — JSON from Claude Code:
```json
{
  "tool_name": "Edit",
  "tool_input": {
    "file_path": "/abs/path/to/file",
    "old_string": "...",
    "new_string": "...",
    "replace_all": false
  }
}
```

**stdout** — normalized response:
```json
{
  "hookSpecificOutput": {
    "hookEventName": "PreToolUse",
    "permissionDecision": "allow",
    "updatedInput": {
      "old_string": "...(re-indented)...",
      "new_string": "...(re-indented)..."
    }
  }
}
```

If indentation already matches, `updatedInput` is omitted and the original edit proceeds unchanged.

## Registration (`.claude/settings.json`)

```json
{
  "hooks": {
    "PreToolUse": [{
      "matcher": "Edit",
      "hooks": [{
        "type": "command",
        "command": "/path/to/indent-normalize-hook"
      }]
    }]
  }
}
```

## Edge cases to handle

- File doesn't exist yet (new file) → pass through unchanged
- Mixed indentation in file → use majority-wins detection
- `old_string` has no indented lines → pass through unchanged
- `replace_all: true` → same normalization applies
- Non-text / binary file → pass through unchanged

## Tech

- Single Go binary, no dependencies beyond stdlib
- `go install`-able
- ~100 lines
