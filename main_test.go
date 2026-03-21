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

// Deep indentation: 8-space (4 levels of 2-space) → tabs
func TestReindent_DeepSpacesToTabs(t *testing.T) {
	// Simulates Claude sending 8-space-indented old_string for a file that uses tabs
	input := "        if label == \"skip\" {\n" +
		"                continue\n" +
		"        }"
	from := indentStyle{char: ' ', width: 8}
	to := indentStyle{char: '\t', width: 1}
	got := reindent(input, from, to)
	want := "\tif label == \"skip\" {\n\t\tcontinue\n\t}"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

// --- lineSimilarity ---

func TestLineSimilarity_Identical(t *testing.T) {
	if s := lineSimilarity("foo bar", "foo bar"); s != 1.0 {
		t.Fatalf("expected 1.0, got %f", s)
	}
}

func TestLineSimilarity_Empty(t *testing.T) {
	if s := lineSimilarity("", ""); s != 1.0 {
		t.Fatalf("expected 1.0, got %f", s)
	}
}

func TestLineSimilarity_OneEmpty(t *testing.T) {
	if s := lineSimilarity("foo", ""); s != 0.0 {
		t.Fatalf("expected 0.0, got %f", s)
	}
}

func TestLineSimilarity_HighSimilarity(t *testing.T) {
	// one character differs — should be well above 0.85
	s := lineSimilarity(`\tif x == 'foo' {`, `\tif x == "foo" {`)
	if s < 0.85 {
		t.Fatalf("expected high similarity, got %f", s)
	}
}

func TestLineSimilarity_StripsIndent(t *testing.T) {
	// leading whitespace should not affect score
	s := lineSimilarity("    foo()", "\tfoo()")
	if s != 1.0 {
		t.Fatalf("expected 1.0 after stripping indent, got %f", s)
	}
}

// --- fuzzyFindBlock ---

func TestFuzzyFindBlock_ExactMatch(t *testing.T) {
	file := strings.Split("a\nb\nc\nd\ne", "\n")
	query := strings.Split("b\nc\nd", "\n")
	start, score := fuzzyFindBlock(file, query)
	if start != 1 {
		t.Fatalf("expected start=1, got %d", start)
	}
	if score < 0.99 {
		t.Fatalf("expected near-perfect score, got %f", score)
	}
}

func TestFuzzyFindBlock_SlightDrift(t *testing.T) {
	// query has a single-quote where file has double-quote, but lines are long enough to score well
	file := strings.Split(
		"func foo() {\n\tif someCondition && x == \"expected-value-here\" {\n\t\treturn true\n\t}\n}",
		"\n",
	)
	query := strings.Split(
		"func foo() {\n\tif someCondition && x == 'expected-value-here' {\n\t\treturn true\n\t}\n}",
		"\n",
	)
	start, score := fuzzyFindBlock(file, query)
	if start != 0 {
		t.Fatalf("expected start=0, got %d (score=%f)", start, score)
	}
	if score < 0.85 {
		t.Fatalf("expected score >= 0.85, got %f", score)
	}
}

func TestFuzzyFindBlock_NoMatch(t *testing.T) {
	file := strings.Split("a\nb\nc", "\n")
	query := strings.Split("x\ny\nz", "\n")
	start, score := fuzzyFindBlock(file, query)
	if start >= 0 {
		t.Fatalf("expected no match, got start=%d score=%f", start, score)
	}
}

func TestFuzzyFindBlock_QueryLongerThanFile(t *testing.T) {
	file := strings.Split("a\nb", "\n")
	query := strings.Split("a\nb\nc\nd", "\n")
	start, _ := fuzzyFindBlock(file, query)
	if start >= 0 {
		t.Fatalf("expected no match when query longer than file")
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

	// Override exitFn so blockWithFeedback doesn't kill the test process.
	var captured int
	exitFn = func(code int) { captured = code }
	t.Cleanup(func() { exitFn = os.Exit })

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
		exitCode: captured,
		stderr:   stderrBuf.String(),
	}

	if captured == 0 && stdoutBuf.Len() > 0 {
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

// extractFeedbackOldString parses the corrected old_string out of the stderr
// feedback message produced by blockWithFeedback.
func extractFeedbackOldString(t *testing.T, stderr string) string {
	t.Helper()
	_, after, ok := strings.Cut(stderr, "old_string:\n")
	if !ok {
		t.Fatalf("stderr feedback missing 'old_string:' marker\n%s", stderr)
	}
	corrected, _, ok := strings.Cut(after, "\n\nnew_string:")
	if !ok {
		t.Fatalf("stderr feedback missing 'new_string:' section\n%s", stderr)
	}
	return corrected
}

// assertBlocked verifies the hook exited with code 2 and that the feedback
// message contains the expected corrected old_string.
func assertBlocked(t *testing.T, res hookResult, wantOldString string) {
	t.Helper()
	if res.exitCode != 2 {
		t.Fatalf("expected exit 2 (block), got exit %d\nstderr: %s", res.exitCode, res.stderr)
	}
	if !strings.Contains(res.stderr, wantOldString) {
		t.Fatalf("stderr feedback missing expected old_string\nwant old_string:\n%s\n\nstderr:\n%s", wantOldString, res.stderr)
	}
}

// assertPassThrough verifies the hook allowed the edit through (exit 0, allow decision).
func assertPassThrough(t *testing.T, res hookResult) {
	t.Helper()
	if res.exitCode != 0 {
		t.Fatalf("expected exit 0 (pass-through), got exit %d\nstderr: %s", res.exitCode, res.stderr)
	}
	if res.out.HookSpecificOutput.PermissionDecision != "allow" {
		t.Fatalf("expected allow decision, got %q", res.out.HookSpecificOutput.PermissionDecision)
	}
}

// --- integration tests ---

func TestIntegration_SpaceToTab(t *testing.T) {
	path := writeTemp(t, "package main\n\nfunc foo() {\n\tif true {\n\t\tx := 1\n\t}\n}\n")
	res := runHook(t, makeInput(t, "Edit", editInput{
		FilePath:  path,
		OldString: "    if true {\n        x := 1\n    }",
		NewString: "    if false {\n        x := 2\n    }",
	}))
	assertBlocked(t, res, "\tif true {\n\t\tx := 1\n\t}")
}

func TestIntegration_FuzzyFallback(t *testing.T) {
	// File uses tabs; old_string uses spaces AND has a quote drift
	fileContent := "package main\n\nfunc foo() {\n\tif x == \"hello\" {\n\t\treturn true\n\t}\n}\n"
	path := writeTemp(t, fileContent)

	res := runHook(t, makeInput(t, "Edit", editInput{
		FilePath:  path,
		OldString: "    if x == 'hello' {\n        return true\n    }",
		NewString: "    if x == 'world' {\n        return false\n    }",
	}))

	// Fuzzy match succeeds — hook blocks with corrected exact file bytes
	wantOld := "\tif x == \"hello\" {\n\t\treturn true\n\t}"
	assertBlocked(t, res, wantOld)
}

// Deep indentation: Claude sends 8-space old_string for a tab-indented file.
// This is the scenario that was broken on larger documents.
func TestIntegration_DeepIndentSpaceToTab(t *testing.T) {
	fileContent := "package main\n\nfunc outer() {\n\tfor _, x := range items {\n\t\tif x != nil {\n\t\t\tif x.Valid() {\n\t\t\t\tprocess(x)\n\t\t\t}\n\t\t}\n\t}\n}\n"
	path := writeTemp(t, fileContent)

	// Claude sends 8-space (doubling each level) — a common failure mode on
	// deeply indented blocks when the model loses track of the real indent width.
	res := runHook(t, makeInput(t, "Edit", editInput{
		FilePath:  path,
		OldString: "                if x.Valid() {\n                        process(x)\n                }",
		NewString: "                if x.Valid() {\n                        process(x)\n                        log(x)\n                }",
	}))

	assertBlocked(t, res, "\t\t\tif x.Valid() {\n\t\t\t\tprocess(x)\n\t\t\t}")
}

// Simulate the common "bigger document, deep indent" failure: a long file where
// Claude picks up an 8-space indent from the visual context even though the
// file uses tabs.
func TestIntegration_LargeFileDeepNesting(t *testing.T) {
	path := "testdata/deep_nested.go"

	// Claude sends old_string as if indented with 8 spaces per level (a common
	// failure on large files where the model infers indent from rendered context).
	res := runHook(t, makeInput(t, "Edit", editInput{
		FilePath: path,
		OldString: "                if label == \"skip\" {\n" +
			"                        continue\n" +
			"                } else if label == \"stop\" {\n" +
			"                        return fmt.Errorf(\"stopped at item %q\", item)\n" +
			"                } else {\n" +
			"                        fmt.Printf(\"processing %q with label %q\\n\", item, label)\n" +
			"                }",
		NewString: "                if label == \"skip\" {\n" +
			"                        continue\n" +
			"                } else {\n" +
			"                        fmt.Printf(\"processing %q with label %q\\n\", item, label)\n" +
			"                }",
	}))

	// Hook must block and provide corrected tab-indented strings
	if res.exitCode != 2 {
		t.Fatalf("expected exit 2 (block with feedback), got exit %d\nstderr:\n%s", res.exitCode, res.stderr)
	}
	// The corrected old_string must be findable in the actual file
	fileBytes, _ := os.ReadFile(path)
	fileStr := string(fileBytes)

	correctedOld := extractFeedbackOldString(t, res.stderr)
	if !strings.Contains(fileStr, correctedOld) {
		t.Fatalf("corrected old_string from feedback not found in file\ngot:\n%s", correctedOld)
	}
}

func TestIntegration_AlreadyMatchingIndent(t *testing.T) {
	path := writeTemp(t, "package main\n\nfunc foo() {\n\tif true {\n\t\tx := 1\n\t}\n}\n")
	res := runHook(t, makeInput(t, "Edit", editInput{
		FilePath:  path,
		OldString: "\tif true {\n\t\tx := 1\n\t}",
		NewString: "\tif false {\n\t\tx := 2\n\t}",
	}))
	assertPassThrough(t, res)
}

func TestIntegration_NonExistentFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.go")
	res := runHook(t, makeInput(t, "Edit", editInput{
			FilePath:  path,
			OldString: "    x := 1",
			NewString: "    x := 2",
		}))
	assertPassThrough(t, res)
}

func TestIntegration_BinaryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binary.bin")
	os.WriteFile(path, []byte("data\x00more"), 0644)

	res := runHook(t, makeInput(t, "Edit", editInput{
			FilePath:  path,
			OldString: "    x := 1",
			NewString: "    x := 2",
		}))
	assertPassThrough(t, res)
}

func TestIntegration_NoIndentInOldString(t *testing.T) {
	path := writeTemp(t, "package main\n\nfunc foo() {\n\tx := 1\n}\n")
	res := runHook(t, makeInput(t, "Edit", editInput{
			FilePath:  path,
			OldString: "package main",
			NewString: "package main",
		}))
	assertPassThrough(t, res)
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

// --- golden file tests ---

type goldenCase struct {
	name string
	file string // relative to testdata/
	oldString string
	newString string
	// wantBlocked=true: indent mismatch — hook must exit 2 with corrected strings in stderr
	// wantBlocked=false: indents match — hook must pass through and old_string exists verbatim
	wantBlocked bool
}

func TestGolden(t *testing.T) {
	cases := []goldenCase{
		// Tab-indented files: hook normalises space old_string → tabs via exit-2 feedback
		{
			name: "go/deep_nested exact match",
			file: "deep_nested.go",
			oldString: "        if label, ok := cfg.Labels[item]; ok {\n" +
				"                if label == \"skip\" {\n" +
				"                        continue\n" +
				"                } else if label == \"stop\" {\n" +
				"                        return fmt.Errorf(\"stopped at item %q\", item)\n" +
				"                } else {\n" +
				"                        fmt.Printf(\"processing %q with label %q\\n\", item, label)\n" +
				"                }\n" +
				"        }",
			newString: "        if label, ok := cfg.Labels[item]; ok {\n" +
				"                fmt.Printf(\"processing %q with label %q\\n\", item, label)\n" +
				"        }",
			wantBlocked: true,
		},
		{
			name: "go/deep_nested validate block",
			file: "deep_nested.go",
			oldString: "        for k, v := range cfg.Labels {\n" +
				"                if k == \"\" {\n" +
				"                        errs = append(errs, \"label key must not be empty\")\n" +
				"                }\n" +
				"                if v == \"\" {\n" +
				"                        errs = append(errs, fmt.Sprintf(\"label %q has empty value\", k))\n" +
				"                }\n" +
				"        }",
			newString: "        for k, v := range cfg.Labels {\n" +
				"                if k == \"\" || v == \"\" {\n" +
				"                        errs = append(errs, fmt.Sprintf(\"invalid label %q=%q\", k, v))\n" +
				"                }\n" +
				"        }",
			wantBlocked: true,
		},
		{
			name: "templ/navbar fuzzy quote drift",
			file: "component.templ",
			oldString: "                <a\n" +
				"                        href={ templ.URL(\"/\" + item) }\n" +
				"                        class=\"navbar-link\"\n" +
				"                        @click.stop={ \"navigate('\" + item + \"')\" }\n" +
				"                >\n" +
				"                        { item }\n" +
				"                </a>",
			newString: "                <a\n" +
				"                        href={ templ.URL(\"/\" + item) }\n" +
				"                        class=\"navbar-link\"\n" +
				"                        @click.stop={ \"navigate('\" + item + \"')\" }\n" +
				"                        data-item={ item }\n" +
				"                >\n" +
				"                        { item }\n" +
				"                </a>",
			wantBlocked: true,
		},
		{
			name: "templ/modal close button",
			file: "component.templ",
			oldString: "                        <button class=\"modal-close\" @click=\"closeModal\" aria-label=\"Close\">\n" +
				"                                <span aria-hidden=\"true\">&times;</span>\n" +
				"                        </button>",
			newString: "                        <button class=\"modal-close\" @click=\"closeModal\" aria-label=\"Close\" type=\"button\">\n" +
				"                                <span aria-hidden=\"true\">&times;</span>\n" +
				"                        </button>",
			wantBlocked: true,
		},
		// Space-indented files: hook passes through, old_string must already exist verbatim
		{
			name: "typescript/fetchUser error branch",
			file: "service.ts",
			oldString: "        const response = await fetch(`/api/users/${id}`);\n" +
				"        if (!response.ok) {\n" +
				"            return {\n" +
				"                data: null as unknown as User,\n" +
				"                error: `HTTP ${response.status}`,\n" +
				"                status: response.status,\n" +
				"            };\n" +
				"        }",
			newString: "        const response = await fetch(`/api/users/${id}`, { signal: AbortSignal.timeout(5000) });\n" +
				"        if (!response.ok) {\n" +
				"            return {\n" +
				"                data: null as unknown as User,\n" +
				"                error: `HTTP ${response.status}: ${await response.text()}`,\n" +
				"                status: response.status,\n" +
				"            };\n" +
				"        }",
			wantBlocked: false,
		},
		{
			name: "python/pipeline run loop",
			file: "pipeline.py",
			oldString: "        for item in data:\n" +
				"            try:\n" +
				"                out = self._process(item)\n" +
				"                if out is not None:\n" +
				"                    results.append(out)\n" +
				"            except Exception as exc:\n" +
				"                if self.config.get(\"fail_fast\", False):\n" +
				"                    raise PipelineError(f\"stage {self.name!r} failed on item {item!r}\") from exc\n" +
				"                logger.warning(\"stage %r: skipping item due to error: %s\", self.name, exc)",
			newString: "        for item in data:\n" +
				"            try:\n" +
				"                out = self._process(item)\n" +
				"                if out is not None:\n" +
				"                    results.append(out)\n" +
				"            except Exception as exc:\n" +
				"                logger.warning(\"stage %r: error: %s\", self.name, exc)\n" +
				"                if self.config.get(\"fail_fast\", False):\n" +
				"                    raise PipelineError(f\"stage {self.name!r} failed\") from exc",
			wantBlocked: false,
		},
		{
			name: "html/hero section",
			file: "layout.html",
			oldString: "          <h1 class=\"hero__title\">Build faster with <span class=\"hero__accent\">My App</span></h1>\n" +
				"          <p class=\"hero__subtitle\">\n" +
				"            The all-in-one platform for teams who ship. Integrate your tools,\n" +
				"            automate your workflows, and focus on what matters.\n" +
				"          </p>",
			newString: "          <h1 class=\"hero__title\">Ship faster with <span class=\"hero__accent\">My App</span></h1>\n" +
				"          <p class=\"hero__subtitle\">\n" +
				"            The all-in-one platform for teams who ship.\n" +
				"          </p>",
			wantBlocked: false,
		},
		{
			name: "yaml/deploy job",
			file: "ci.yaml",
			oldString: "      - name: Deploy to staging\n" +
				"        env:\n" +
				"          DEPLOY_TOKEN: ${{ secrets.DEPLOY_TOKEN }}\n" +
				"          DEPLOY_HOST: ${{ secrets.STAGING_HOST }}\n" +
				"        run: |\n" +
				"          echo \"Deploying to staging...\"\n" +
				"          ./scripts/deploy.sh staging dist/app-linux-amd64",
			newString: "      - name: Deploy to staging\n" +
				"        env:\n" +
				"          DEPLOY_TOKEN: ${{ secrets.DEPLOY_TOKEN }}\n" +
				"          DEPLOY_HOST: ${{ secrets.STAGING_HOST }}\n" +
				"        run: |\n" +
				"          echo \"Deploying to staging...\"\n" +
				"          ./scripts/deploy.sh staging dist/app-linux-amd64\n" +
				"          ./scripts/smoke-test.sh https://staging.example.com",
			wantBlocked: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join("testdata", tc.file)
			res := runHook(t, makeInput(t, "Edit", editInput{
					FilePath:  path,
					OldString: tc.oldString,
					NewString: tc.newString,
				}))

			fileBytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("could not read testdata file: %v", err)
			}
			fileStr := string(fileBytes)

			if tc.wantBlocked {
				if res.exitCode != 2 {
					t.Fatalf("expected exit 2 (block with feedback), got exit %d\nstderr:\n%s", res.exitCode, res.stderr)
				}
				correctedOld := extractFeedbackOldString(t, res.stderr)
				if !strings.Contains(fileStr, correctedOld) {
					t.Fatalf("corrected old_string from feedback not found in file\ngot:\n%s", correctedOld)
				}
			} else {
				// Space-indented file: hook must pass through, old_string must already exist verbatim
				if !strings.Contains(fileStr, tc.oldString) {
					t.Fatalf("old_string not found in testdata file — fix the test case\ngot:\n%s", tc.oldString)
				}
				assertPassThrough(t, res)
			}
		})
	}
}
