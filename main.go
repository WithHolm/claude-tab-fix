package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
)

type hookInput struct {
	ToolName  string    `json:"tool_name"`
	ToolInput editInput `json:"tool_input"`
}

type editInput struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

type hookOutput struct {
	HookSpecificOutput hookSpecific `json:"hookSpecificOutput"`
}

type hookSpecific struct {
	HookEventName      string     `json:"hookEventName"`
	PermissionDecision string     `json:"permissionDecision"`
	UpdatedInput       *editInput `json:"updatedInput,omitempty"`
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

func reindent(s string, from, to indentStyle) string {
	if from.width == 0 || to.width == 0 {
		return s
	}
	var sb strings.Builder
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if i > 0 {
			sb.WriteByte('\n')
		}
		// Count leading indent units
		pos := 0
		units := 0
		for pos < len(line) {
			if from.char == '\t' {
				if line[pos] != '\t' {
					break
				}
				units++
				pos++
			} else {
				if pos+from.width > len(line) {
					break
				}
				segment := line[pos : pos+from.width]
				if strings.TrimLeft(segment, " ") != "" {
					break
				}
				units++
				pos += from.width
			}
		}
		// Write new indent
		if to.char == '\t' {
			sb.WriteString(strings.Repeat("\t", units))
		} else {
			sb.WriteString(strings.Repeat(strings.Repeat(" ", to.width), units))
		}
		sb.WriteString(line[pos:])
	}
	return sb.String()
}

func passThrough() {
	out := hookOutput{
		HookSpecificOutput: hookSpecific{
			HookEventName:      "PreToolUse",
			PermissionDecision: "allow",
		},
	}
	json.NewEncoder(os.Stdout).Encode(out)
}

func main() {
	var input hookInput
	if err := json.NewDecoder(os.Stdin).Decode(&input); err != nil {
		passThrough()
		return
	}

	content, err := os.ReadFile(input.ToolInput.FilePath)
	if err != nil || bytes.IndexByte(content, 0) >= 0 {
		passThrough()
		return
	}

	fileIndent := detectIndent(string(content))
	oldIndent := detectIndent(input.ToolInput.OldString)

	// If either detection failed or they match, pass through
	if fileIndent.char == 0 || oldIndent.char == 0 || fileIndent.char == oldIndent.char {
		passThrough()
		return
	}

	newOld := reindent(input.ToolInput.OldString, oldIndent, fileIndent)
	newNew := reindent(input.ToolInput.NewString, oldIndent, fileIndent)

	updated := input.ToolInput
	updated.OldString = newOld
	updated.NewString = newNew

	out := hookOutput{
		HookSpecificOutput: hookSpecific{
			HookEventName:      "PreToolUse",
			PermissionDecision: "allow",
			UpdatedInput:       &updated,
		},
	}
	json.NewEncoder(os.Stdout).Encode(out)
}
