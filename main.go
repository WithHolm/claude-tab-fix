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

type editInput struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
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
	PermissionDecision string `json:"permissionDecision"`
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

// lineSimilarity returns a 0.0–1.0 score comparing two lines after stripping
// leading/trailing whitespace. Uses longest-common-subsequence character ratio.
func lineSimilarity(a, b string) float64 {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == b {
		return 1.0
	}
	if len(a) == 0 && len(b) == 0 {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}
	// LCS length via DP
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	curr := make([]int, len(rb)+1)
	for i := 1; i <= len(ra); i++ {
		for j := 1; j <= len(rb); j++ {
			if ra[i-1] == rb[j-1] {
				curr[j] = prev[j-1] + 1
			} else if prev[j] > curr[j-1] {
				curr[j] = prev[j]
			} else {
				curr[j] = curr[j-1]
			}
		}
		prev, curr = curr, prev
		for k := range curr {
			curr[k] = 0
		}
	}
	lcs := prev[len(rb)]
	maxLen := len(ra)
	if len(rb) > maxLen {
		maxLen = len(rb)
	}
	return float64(lcs) / float64(maxLen)
}

// fuzzyFindBlock slides a window of len(query) lines over fileLines and returns
// the start index and score of the best-matching window.
func fuzzyFindBlock(fileLines, query []string) (bestStart int, bestScore float64) {
	n := len(query)
	if n == 0 || len(fileLines) < n {
		return -1, 0
	}
	bestStart = -1
	for i := 0; i <= len(fileLines)-n; i++ {
		matched := 0
		total := 0.0
		for j, ql := range query {
			s := lineSimilarity(fileLines[i+j], ql)
			total += s
			if s >= 0.85 {
				matched++
			}
		}
		score := total / float64(n)
		// require 85% of lines to individually match
		if float64(matched)/float64(n) >= 0.85 && score > bestScore {
			bestScore = score
			bestStart = i
		}
	}
	return bestStart, bestScore
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[indent-normalize] "+format+"\n", args...)
}

func indentName(s indentStyle) string {
	if s.char == '\t' {
		return "tabs"
	}
	return fmt.Sprintf("%d-space", s.width)
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

func passThroughWithContext(ctx string) {
	out := hookOutput{
		HookSpecificOutput: hookSpecific{
			HookEventName:      "PreToolUse",
			PermissionDecision: "allow",
			AdditionalContext:  ctx,
		},
	}
	json.NewEncoder(os.Stdout).Encode(out)
}

// exitFn is a variable so tests can override os.Exit.
var exitFn = os.Exit

func blockWithFeedback(msg string) {
	// Exit 2 causes Claude Code to surface stderr as feedback to Claude,
	// allowing it to retry the Edit with corrected inputs.
	fmt.Fprintln(os.Stderr, msg)
	exitFn(2)
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

// handleEdit is the main path: fix indent mismatches in Edit tool calls.
func handleEdit(raw json.RawMessage) {
	var ei editInput
	if err := json.Unmarshal(raw, &ei); err != nil {
		passThrough()
		return
	}

	content, err := os.ReadFile(ei.FilePath)
	if err != nil || bytes.IndexByte(content, 0) >= 0 {
		passThrough()
		return
	}

	fileStr := string(content)
	fileIndent := detectIndent(fileStr)
	oldIndent := detectIndent(ei.OldString)

	// If either detection failed or they match, pass through
	if fileIndent.char == 0 || oldIndent.char == 0 || fileIndent.char == oldIndent.char {
		passThrough()
		return
	}

	newOld := reindent(ei.OldString, oldIndent, fileIndent)
	newNew := reindent(ei.NewString, oldIndent, fileIndent)

	oldLines := len(strings.Split(ei.OldString, "\n"))
	logf("normalizing %s → %s across %d lines in old_string", indentName(oldIndent), indentName(fileIndent), oldLines)

	if !strings.Contains(fileStr, newOld) {
		// Exact match failed — try fuzzy block match
		fileLines := strings.Split(fileStr, "\n")
		queryLines := strings.Split(newOld, "\n")
		start, score := fuzzyFindBlock(fileLines, queryLines)
		if start >= 0 {
			matched := strings.Join(fileLines[start:start+len(queryLines)], "\n")
			newOld = matched
			logf("fuzzy match: found block (score=%.2f, lines %d–%d)", score, start+1, start+len(queryLines))
		} else {
			logf("WARNING: normalized old_string not found in file and fuzzy match failed — edit will likely fail")
			logf("normalized old_string was:\n%s", newOld)
			passThrough()
			return
		}
	}

	// Block the edit and feed corrected strings back to Claude so it retries
	// with exact file bytes. This is more reliable than updatedInput because
	// Claude Code may pre-validate old_string before applying hook output.
	blockWithFeedback(fmt.Sprintf(
		"old_string indentation mismatch (%s in old_string vs %s in file). "+
			"Retry the Edit with these exact strings:\n\nold_string:\n%s\n\nnew_string:\n%s",
		indentName(oldIndent), indentName(fileIndent), newOld, newNew,
	))
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
	case "Edit":
		handleEdit(input.ToolInput)
	case "Bash", "Write":
		handleBashOrWrite(input.ToolName, input.ToolInput)
	default:
		passThrough()
	}
}
