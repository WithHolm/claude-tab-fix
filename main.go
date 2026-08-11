package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
)

var version = func() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}()

type hookInput struct {
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
}
// bashInput covers the Bash tool (command) and Write tool (file_path + content).
type bashInput struct {
	Command  string `json:"command"`
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

type hookOutput struct {
	HookSpecificOutput hookSpecific `json:"hookSpecificOutput"`
}

type hookSpecific struct {
	HookEventName      string `json:"hookEventName"`
	PermissionDecision string `json:"permissionDecision,omitempty"`
	AdditionalContext  string `json:"additionalContext,omitempty"`
}

type indentStyle struct {
	char  rune
	width int
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func detectIndent(s string) indentStyle {
	spaceCounts := map[int]int{}
	tabLines := 0

	for _, line := range strings.Split(s, "\n") {
		if len(line) == 0 {
			continue
		}
		if line[0] == '\t' {
			tabLines++
			continue
		}
		if line[0] == ' ' {
			count := 0
			for _, ch := range line {
				if ch == ' ' {
					count++
				} else {
					break
				}
			}
			if count > 0 {
				spaceCounts[count]++
			}
		}
	}

	if tabLines > 0 {
		return indentStyle{char: '\t', width: 1}
	}

	if len(spaceCounts) == 0 {
		return indentStyle{}
	}

	// Find the GCD of all observed space-run lengths — this is the indent unit
	width := 0
	for w := range spaceCounts {
		width = gcd(width, w)
	}
	if width == 0 {
		return indentStyle{}
	}

	return indentStyle{char: ' ', width: width}
}


func indentName(s indentStyle) string {
	if s.char == '\t' {
		return "tabs"
	}
	return fmt.Sprintf("%d-space", s.width)
}

func passThrough() {
	// Exit 0 with no stdout — hook makes no decision, normal permission flow applies.
}

func passThroughWithContext(ctx string) {
	out := hookOutput{
		HookSpecificOutput: hookSpecific{
			HookEventName:     "PreToolUse",
			AdditionalContext: ctx,
		},
	}
	json.NewEncoder(os.Stdout).Encode(out)
}

func postPassThroughWithContext(ctx string) {
	out := hookOutput{
		HookSpecificOutput: hookSpecific{
			HookEventName:     "PostToolUse",
			AdditionalContext: ctx,
		},
	}
	json.NewEncoder(os.Stdout).Encode(out)
}

// extractFileFromBashCommand tries to identify the target file in common
// file-editing shell patterns (sed -i, awk, perl -i, python -c).
func extractFileFromBashCommand(cmd string) string {
	editPatterns := []string{"sed ", "awk ", "perl ", "python ", "python3 "}
	for _, p := range editPatterns {
		if !strings.Contains(cmd, p) {
			continue
		}
		// Take the last token that looks like a file path (has . or /)
		fields := strings.Fields(cmd)
		for i := len(fields) - 1; i >= 0; i-- {
			f := fields[i]
			if strings.HasPrefix(f, "-") || strings.HasPrefix(f, "'") || strings.HasPrefix(f, "\"") {
				continue
			}
			if strings.ContainsAny(f, "./") {
				return f
			}
		}
	}
	return ""
}

type readInput struct {
	FilePath string `json:"file_path"`
}

// handleRead fires after the Read tool completes. If the file uses tab
// indentation, it injects a context note reminding Claude that the Read
// tool's line-number separator is also a tab, so old_string for Edit calls
// should have one fewer leading tab than the raw output suggests.
func handleRead(raw json.RawMessage) {
	var ri readInput
	if err := json.Unmarshal(raw, &ri); err != nil {
		return
	}

	content, err := os.ReadFile(ri.FilePath)
	if err != nil || bytes.IndexByte(content, 0) >= 0 {
		return
	}

	fileIndent := detectIndent(string(content))
	if fileIndent.char != '\t' {
		return
	}

	postPassThroughWithContext(
		"claude-tab-fix: " + ri.FilePath + " uses tab indentation. " +
			"IMPORTANT: the Read tool prefixes each line with \"N\\t\" (line number + tab). " +
			"That leading tab is the separator, NOT part of the file content. " +
			"When constructing old_string or new_string for an Edit call, " +
			"use one fewer leading tab than you see in the Read output.",
	)
}

// handleBashOrWrite warns (non-blocking) when Claude tries to edit a
// tab-indented file via Bash or Write, bypassing indent normalization.
func handleBashOrWrite(toolName string, raw json.RawMessage) {
	var bi bashInput
	if err := json.Unmarshal(raw, &bi); err != nil {
		passThrough()
		return
	}

	var filePath, content string
	if toolName == "Write" {
		filePath = bi.FilePath
		content = bi.Content
	} else {
		filePath = extractFileFromBashCommand(bi.Command)
		if filePath == "" {
			passThrough()
			return
		}
	}

	existing, err := os.ReadFile(filePath)
	if err != nil || bytes.IndexByte(existing, 0) >= 0 {
		passThrough()
		return
	}

	fileIndent := detectIndent(string(existing))
	if fileIndent.char != '\t' {
		passThrough()
		return
	}

	// For Write: also flag if the new content uses spaces
	var mismatch string
	if toolName == "Write" && content != "" {
		newIndent := detectIndent(content)
		if newIndent.char == ' ' {
			mismatch = fmt.Sprintf(
				" New content uses %s but the file uses tabs — result will have mixed indentation.",
				indentName(newIndent),
			)
		}
	}

	passThroughWithContext(fmt.Sprintf(
		"WARNING (claude-tab-fix): %q is a tab-indented file. "+
			"Editing it via %s bypasses indent normalization.%s "+
			"Strongly prefer the Edit tool for targeted changes to this file.",
		filePath, toolName, mismatch,
	))
}

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "-version") {
		fmt.Println(version)
		return
	}

	var input hookInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		passThrough()
		return
	}

	switch input.ToolName {
case "Bash", "Write":
		handleBashOrWrite(input.ToolName, input.ToolInput)
	case "Read":
		handleRead(input.ToolInput)
	default:
		passThrough()
	}
}
