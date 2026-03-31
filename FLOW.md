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
                             additionalContext: "the N\t prefix is the separator,
                             not file content — use one fewer leading tab in old_string"
```

## Edit (PreToolUse)

```
Claude issues an Edit tool call
            │
            ▼
    PreToolUse hook fires
    claude-tab-fix reads JSON from stdin
            │
            ├─ bad JSON / unreadable stdin
            │           └──► passThrough → exit 0 (allow edit unchanged)
            │
            ├─ file does not exist
            │           └──► passThrough → exit 0
            │
            ├─ binary file (contains null bytes)
            │           └──► passThrough → exit 0
            │
            ▼
    Detect indent style of file
    Detect indent style of old_string
            │
            ├─ either is undetectable (no indented lines)
            │           └──► passThrough → exit 0
            │
            ├─ both use the same style (tabs==tabs, spaces==spaces)
            │           └──► passThrough → exit 0
            │
            ▼
    reindent(old_string, from=old_style, to=file_style)
    reindent(new_string, from=old_style, to=file_style)
            │
            ▼
    Does reindented old_string exist verbatim in file?
            │
            ├─ YES
            │   └──► blockWithFeedback → exit 2
            │         stderr: "Retry the Edit with these exact strings:
            │                  old_string: <reindented>
            │                  new_string: <reindented>"
            │         Claude sees the message, retries with corrected strings
            │
            └─ NO  (content has drifted since Claude read the file)
                        │
                        ▼
                fuzzyFindBlock()
                slides a window over the file, scores each candidate
                by line similarity (strips indent, uses LCS ratio)
                        │
                        ├─ match found (≥85% of lines score ≥0.85)
                        │   └──► use exact file bytes as old_string
                        │        blockWithFeedback → exit 2
                        │        (same retry flow as above)
                        │
                        └─ no match
                                    └──► passThrough → exit 0
                                         (can't help — let Claude try anyway)
```

## Exit codes

| Code | Meaning |
|------|---------|
| `0` | Pass-through — edit proceeds unchanged |
| `2` | Blocked with feedback — Claude Code surfaces stderr to Claude, which retries the Edit with the corrected strings |

## Why exit 2 instead of `updatedInput`

Claude Code may pre-validate `old_string` before applying hook output. Sending
corrected strings via `updatedInput` can therefore still fail if Claude's
internal check runs first. Exiting with code 2 causes Claude Code to surface
the stderr message directly to the model as an error, which prompts Claude to
issue a brand-new Edit call with the exact corrected strings — a more reliable
retry path.
