package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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

// --- reindent ---

func TestReindent_SpacesToTabs(t *testing.T) {
	input := "    if true {\n        x := 1\n    }"
	from := indentStyle{char: ' ', width: 4}
	to := indentStyle{char: '\t', width: 1}
	got := reindent(input, from, to)
	want := "\tif true {\n\t\tx := 1\n\t}"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestReindent_TabsToSpaces(t *testing.T) {
	input := "\tif true {\n\t\tx := 1\n\t}"
	from := indentStyle{char: '\t', width: 1}
	to := indentStyle{char: ' ', width: 4}
	got := reindent(input, from, to)
	want := "    if true {\n        x := 1\n    }"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestReindent_NoOpWhenZeroFrom(t *testing.T) {
	input := "    x := 1"
	from := indentStyle{}
	to := indentStyle{char: '\t', width: 1}
	got := reindent(input, from, to)
	if got != input {
		t.Fatalf("expected no-op, got %q", got)
	}
}

func TestReindent_PreservesUnindentedLines(t *testing.T) {
	input := "package main\n\nfunc foo() {\n\tx := 1\n}"
	from := indentStyle{char: '\t', width: 1}
	to := indentStyle{char: ' ', width: 4}
	got := reindent(input, from, to)
	want := "package main\n\nfunc foo() {\n    x := 1\n}"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

// --- integration via main() stdin/stdout ---

func runHook(t *testing.T, input hookInput) hookOutput {
	t.Helper()
	b, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}

	oldStdin := os.Stdin
	oldStdout := os.Stdout
	t.Cleanup(func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
	})

	r, w, _ := os.Pipe()
	os.Stdin = r
	w.Write(b)
	w.Close()

	outR, outW, _ := os.Pipe()
	os.Stdout = outW

	main()

	outW.Close()
	var buf bytes.Buffer
	buf.ReadFrom(outR)

	var out hookOutput
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("failed to parse output: %v\nraw: %s", err, buf.String())
	}
	return out
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

func TestIntegration_SpaceToTab(t *testing.T) {
	path := writeTemp(t, "package main\n\nfunc foo() {\n\tif true {\n\t\tx := 1\n\t}\n}\n")
	out := runHook(t, hookInput{
		ToolName: "Edit",
		ToolInput: editInput{
			FilePath:  path,
			OldString: "    if true {\n        x := 1\n    }",
			NewString: "    if false {\n        x := 2\n    }",
		},
	})

	if out.HookSpecificOutput.UpdatedInput == nil {
		t.Fatal("expected updatedInput, got nil")
	}
	if !strings.Contains(out.HookSpecificOutput.UpdatedInput.OldString, "\t") {
		t.Fatalf("old_string not converted to tabs: %q", out.HookSpecificOutput.UpdatedInput.OldString)
	}
	if strings.Contains(out.HookSpecificOutput.UpdatedInput.OldString, "    ") {
		t.Fatalf("old_string still has spaces: %q", out.HookSpecificOutput.UpdatedInput.OldString)
	}
}

func TestIntegration_AlreadyMatchingIndent(t *testing.T) {
	path := writeTemp(t, "package main\n\nfunc foo() {\n\tif true {\n\t\tx := 1\n\t}\n}\n")
	out := runHook(t, hookInput{
		ToolName: "Edit",
		ToolInput: editInput{
			FilePath:  path,
			OldString: "\tif true {\n\t\tx := 1\n\t}",
			NewString: "\tif false {\n\t\tx := 2\n\t}",
		},
	})

	if out.HookSpecificOutput.UpdatedInput != nil {
		t.Fatalf("expected no updatedInput, got %+v", out.HookSpecificOutput.UpdatedInput)
	}
	if out.HookSpecificOutput.PermissionDecision != "allow" {
		t.Fatalf("expected allow, got %q", out.HookSpecificOutput.PermissionDecision)
	}
}

func TestIntegration_NonExistentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.go")
	out := runHook(t, hookInput{
		ToolName: "Edit",
		ToolInput: editInput{
			FilePath:  path,
			OldString: "    x := 1",
			NewString: "    x := 2",
		},
	})

	if out.HookSpecificOutput.UpdatedInput != nil {
		t.Fatalf("expected pass-through for missing file, got updatedInput")
	}
}

func TestIntegration_BinaryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binary.bin")
	os.WriteFile(path, []byte("data\x00more"), 0644)

	out := runHook(t, hookInput{
		ToolName: "Edit",
		ToolInput: editInput{
			FilePath:  path,
			OldString: "    x := 1",
			NewString: "    x := 2",
		},
	})

	if out.HookSpecificOutput.UpdatedInput != nil {
		t.Fatalf("expected pass-through for binary file, got updatedInput")
	}
}

func TestIntegration_NoIndentInOldString(t *testing.T) {
	path := writeTemp(t, "package main\n\nfunc foo() {\n\tx := 1\n}\n")
	out := runHook(t, hookInput{
		ToolName: "Edit",
		ToolInput: editInput{
			FilePath:  path,
			OldString: "package main",
			NewString: "package main",
		},
	})

	if out.HookSpecificOutput.UpdatedInput != nil {
		t.Fatalf("expected pass-through when old_string has no indented lines")
	}
}
