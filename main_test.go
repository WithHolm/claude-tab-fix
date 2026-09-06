package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// --- detectIndent ---

func TestDetectIndent_Tabs(t *testing.T) {
	s := "func foo() {\n\tif true {\n\t\tx := 1\n\t}\n}"
	got := detectIndent(s)
	if got.char != '\t' {
		t.Fatalf("expected tab, got %q", got.char)
	}
}

func TestDetectIndent_Spaces4(t *testing.T) {
	s := "func foo() {\n    if true {\n        x := 1\n    }\n}"
	got := detectIndent(s)
	if got.char != ' ' || got.width != 4 {
		t.Fatalf("expected 4-space, got char=%q width=%d", got.char, got.width)
	}
}

func TestDetectIndent_Spaces2(t *testing.T) {
	s := "if true {\n  x := 1\n  y := 2\n}"
	got := detectIndent(s)
	if got.char != ' ' || got.width != 2 {
		t.Fatalf("expected 2-space, got char=%q width=%d", got.char, got.width)
	}
}

func TestDetectIndent_NoIndent(t *testing.T) {
	s := "package main\n\nfunc foo() {}\n"
	got := detectIndent(s)
	if got.char != 0 {
		t.Fatalf("expected zero value, got char=%q width=%d", got.char, got.width)
	}
}

func TestDetectIndent_Empty(t *testing.T) {
	got := detectIndent("")
	if got.char != 0 {
		t.Fatalf("expected zero value for empty string")
	}
}

// hookResult captures the outcome of a hook invocation. The hook now uses
// exit-2 + stderr feedback rather than updatedInput JSON, so callers need
// both the exit code and the stderr message.
type hookResult struct {
	exitCode int
	stderr   string
	// stdout JSON, only populated on passThrough (exit 0)
	out hookOutput
}

// makeInput builds a hookInput with the tool_input marshaled as raw JSON.
func makeInput(t *testing.T, toolName string, toolInput any) hookInput {
	t.Helper()
	raw, err := json.Marshal(toolInput)
	if err != nil {
		t.Fatal(err)
	}
	return hookInput{ToolName: toolName, ToolInput: json.RawMessage(raw)}
}

// runHook drives main() with the given input and captures stdout, stderr, and
// the exit code without actually terminating the test process.
func runHook(t *testing.T, input hookInput) hookResult {
	t.Helper()
	b, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}

	oldStdin := os.Stdin
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	t.Cleanup(func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	})

	// Pipe stdin
	stdinR, stdinW, _ := os.Pipe()
	os.Stdin = stdinR
	stdinW.Write(b)
	stdinW.Close()

	// Pipe stdout
	stdoutR, stdoutW, _ := os.Pipe()
	os.Stdout = stdoutW

	// Pipe stderr
	stderrR, stderrW, _ := os.Pipe()
	os.Stderr = stderrW

	main()

	stdoutW.Close()
	stderrW.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	stdoutBuf.ReadFrom(stdoutR)
	stderrBuf.ReadFrom(stderrR)

	res := hookResult{
		exitCode: 0,
		stderr:   stderrBuf.String(),
	}

	if stdoutBuf.Len() > 0 {
		if err := json.Unmarshal(stdoutBuf.Bytes(), &res.out); err != nil {
			t.Fatalf("failed to parse stdout JSON: %v\nraw: %s", err, stdoutBuf.String())
		}
	}
	return res
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "*.go")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(content)
	f.Close()
	return f.Name()
}

func assertPassThrough(t *testing.T, res hookResult) {
	t.Helper()
	if res.exitCode != 0 {
		t.Fatalf("expected exit 0 (pass-through), got exit %d\nstderr: %s", res.exitCode, res.stderr)
	}
}

// --- Bash / Write advisory warning tests ---

func TestBash_WarnOnTabFile(t *testing.T) {
	path := writeTemp(t, "package main\n\nfunc foo() {\n\tx := 1\n}\n")
	res := runHook(t, makeInput(t, "Bash", bashInput{
		Command: "sed -i 's/foo/bar/' " + path,
	}))
	// Must pass through (not block), but warn via additionalContext
	assertPassThrough(t, res)
	if !strings.Contains(res.out.HookSpecificOutput.AdditionalContext, "tab-indented") {
		t.Fatalf("expected tab-indented warning in additionalContext, got: %q", res.out.HookSpecificOutput.AdditionalContext)
	}
	if !strings.Contains(res.out.HookSpecificOutput.AdditionalContext, "Edit tool") {
		t.Fatalf("expected Edit tool suggestion in additionalContext, got: %q", res.out.HookSpecificOutput.AdditionalContext)
	}
}

func TestBash_NoWarnOnSpaceFile(t *testing.T) {
	path := writeTemp(t, "package main\n\nfunc foo() {\n    x := 1\n}\n")
	res := runHook(t, makeInput(t, "Bash", bashInput{
		Command: "sed -i 's/foo/bar/' " + path,
	}))
	assertPassThrough(t, res)
	if res.out.HookSpecificOutput.AdditionalContext != "" {
		t.Fatalf("expected no warning for space-indented file, got: %q", res.out.HookSpecificOutput.AdditionalContext)
	}
}

func TestBash_NoWarnOnUnrecognisedCommand(t *testing.T) {
	res := runHook(t, makeInput(t, "Bash", bashInput{
		Command: "go test ./...",
	}))
	assertPassThrough(t, res)
	if res.out.HookSpecificOutput.AdditionalContext != "" {
		t.Fatalf("expected no warning for non-file-edit command, got: %q", res.out.HookSpecificOutput.AdditionalContext)
	}
}

func TestWrite_WarnOnTabFileWithSpaceContent(t *testing.T) {
	path := writeTemp(t, "package main\n\nfunc foo() {\n\tx := 1\n}\n")
	res := runHook(t, makeInput(t, "Write", bashInput{
		FilePath: path,
		Content:  "package main\n\nfunc foo() {\n    x := 2\n}\n",
	}))
	assertPassThrough(t, res)
	ctx := res.out.HookSpecificOutput.AdditionalContext
	if !strings.Contains(ctx, "tab-indented") {
		t.Fatalf("expected tab-indented warning, got: %q", ctx)
	}
	if !strings.Contains(ctx, "mixed indentation") {
		t.Fatalf("expected mixed indentation warning, got: %q", ctx)
	}
}

func TestWrite_WarnOnTabFileWithTabContent(t *testing.T) {
	// Content also uses tabs — warn about bypassing hook but no mixed-indent note
	path := writeTemp(t, "package main\n\nfunc foo() {\n\tx := 1\n}\n")
	res := runHook(t, makeInput(t, "Write", bashInput{
		FilePath: path,
		Content:  "package main\n\nfunc foo() {\n\tx := 2\n}\n",
	}))
	assertPassThrough(t, res)
	ctx := res.out.HookSpecificOutput.AdditionalContext
	if !strings.Contains(ctx, "tab-indented") {
		t.Fatalf("expected tab-indented warning, got: %q", ctx)
	}
	if strings.Contains(ctx, "mixed indentation") {
		t.Fatalf("unexpected mixed indentation warning when content also uses tabs: %q", ctx)
	}
}

