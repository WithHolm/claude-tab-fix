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

// --- integration: fuzzy fallback ---

func TestIntegration_FuzzyFallback(t *testing.T) {
	// File uses tabs; old_string uses spaces AND has a quote drift
	fileContent := "package main\n\nfunc foo() {\n\tif x == \"hello\" {\n\t\treturn true\n\t}\n}\n"
	path := writeTemp(t, fileContent)

	out := runHook(t, hookInput{
		ToolName: "Edit",
		ToolInput: editInput{
			FilePath:  path,
			OldString: "    if x == 'hello' {\n        return true\n    }",
			NewString: "    if x == 'world' {\n        return false\n    }",
		},
	})

	if out.HookSpecificOutput.UpdatedInput == nil {
		t.Fatal("expected updatedInput from fuzzy fallback, got nil")
	}
	// old_string should be exact file bytes
	want := "\tif x == \"hello\" {\n\t\treturn true\n\t}"
	if out.HookSpecificOutput.UpdatedInput.OldString != want {
		t.Fatalf("expected exact file bytes:\n%q\ngot:\n%q", want, out.HookSpecificOutput.UpdatedInput.OldString)
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

// --- golden file tests ---

// goldenCase describes one hook invocation against a testdata file.
// wantNormalized=true means the file uses different indent than oldString,
// so the hook must produce updatedInput with an old_string found in the file.
// wantNormalized=false means indents already match — hook passes through but
// old_string must still be found in the file verbatim.
type goldenCase struct {
	name            string
	file            string // relative to testdata/
	oldString       string
	newString       string
	wantNormalized  bool // expect hook to produce updatedInput
}

func TestGolden(t *testing.T) {
	cases := []goldenCase{
		// Tab-indented files: hook normalizes space old_string → tabs
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
			wantNormalized: true,
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
			wantNormalized: true,
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
			wantNormalized: true,
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
			wantNormalized: true,
		},
		// Space-indented files: hook passes through, but old_string must exist verbatim
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
			wantNormalized: false,
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
			wantNormalized: false,
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
			wantNormalized: false,
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
			wantNormalized: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join("testdata", tc.file)
			out := runHook(t, hookInput{
				ToolName: "Edit",
				ToolInput: editInput{
					FilePath:  path,
					OldString: tc.oldString,
					NewString: tc.newString,
				},
			})

			fileBytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("could not read testdata file: %v", err)
			}
			fileStr := string(fileBytes)

			if tc.wantNormalized {
				if out.HookSpecificOutput.UpdatedInput == nil {
					t.Fatal("expected updatedInput (normalization or fuzzy), got nil")
				}
				got := out.HookSpecificOutput.UpdatedInput.OldString
				if !strings.Contains(fileStr, got) {
					t.Fatalf("normalized old_string not found in file\ngot:\n%s", got)
				}
			} else {
				// Space-indented file: hook passes through, old_string must already be in file
				if !strings.Contains(fileStr, tc.oldString) {
					t.Fatalf("old_string not found in testdata file — fix the test case\ngot:\n%s", tc.oldString)
				}
			}
		})
	}
}
