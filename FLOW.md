# Hook flow

## Read (PostToolUse)

```
Claude issues a Read tool call
            │
            ▼
    Read tool executes (file content already in Claude's context)
            │
            ▼
    PostToolUse hook fires
    claude-tab-fix reads JSON from stdin
            │
            ├─ file does not exist / unreadable / binary
            │           └──► silent exit (no output)
            │
            ├─ file uses space indentation
            │           └──► silent exit (no output)
            │
            └─ file uses tab indentation
                        └──► postPassThroughWithContext
                             additionalContext: "file uses tab indentation.
                             The N\t prefix is the separator, not file content —
                             use one fewer leading tab in old_string"
```

## Bash / Write (PreToolUse)

```
Claude issues a Bash or Write tool call
            │
            ▼
    PreToolUse hook fires
    claude-tab-fix reads JSON from stdin
            │
            ├─ bad JSON / unreadable stdin
            │           └──► passThrough → exit 0
            │
            ├─ file does not exist
            │           └──► passThrough → exit 0
            │
            ├─ binary file
            │           └──► passThrough → exit 0
            │
            ├─ Bash: command is not a file-editing pattern
            │           └──► passThrough → exit 0
            │
            ├─ file uses space indentation
            │           └──► passThrough → exit 0
            │
            └─ file uses tab indentation
                        │
                        ├─ Write: new content uses different indent style
                        │   └──► passThroughWithContext
                        │        additionalContext: "WARNING: tab-indented file.
                        │        New content uses N-space — mixed indentation.
                        │        Strongly prefer the Edit tool."
                        │
                        ├─ Write: new content also uses tabs
                        │   └──► passThroughWithContext
                        │        additionalContext: "WARNING: tab-indented file.
                        │        Editing via Write bypasses indent normalization.
                        │        Strongly prefer the Edit tool."
                        │
                        └─ Bash: file-editing command detected
                            └──► passThroughWithContext
                                 additionalContext: "WARNING: tab-indented file.
                                 Editing via Bash bypasses indent normalization.
                                 Strongly prefer the Edit tool."
```

## Why no Edit PreToolUse hook

Claude Code validates `old_string` against the file **before** calling PreToolUse
hooks. If `old_string` uses spaces and the file uses tabs, Claude Code rejects
the Edit with "String to replace not found" — the hook never runs.

The same validation applies to `updatedInput` and `blockWithFeedback` responses:
Claude Code checks the original input first, so the hook cannot intercept
mismatched edits.

Instead, the Read hook tells Claude the file uses tabs. Claude constructs
`old_string` with correct tab indentation, the bytes match, and the Edit
proceeds without hook intervention.
