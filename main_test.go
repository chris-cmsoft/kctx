package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	kubecontext "github.com/chris-cmsoft/gotool-kubecontext-picker"
)

func TestExecutePrintsZshCompletion(t *testing.T) {
	var stdout bytes.Buffer
	loader := &fakeLoader{}

	err := execute(
		[]string{"completion", "zsh"},
		&stdout,
		&bytes.Buffer{},
		t.TempDir(),
		loader,
		&fakeKubectl{},
		&fakeSelector{},
		&fakeShell{},
	)
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	for _, want := range []string{
		"#compdef kctx",
		"kctx __complete-contexts",
		"_describe 'Kubernetes context' contexts",
		"compdef _kctx kctx",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("completion output does not contain %q:\n%s", want, stdout.String())
		}
	}
	if loader.calls != 0 {
		t.Fatalf("contexts calls = %d, want 0", loader.calls)
	}
}

func TestExecutePrintsCompletionCandidates(t *testing.T) {
	var stdout bytes.Buffer
	loader := &fakeLoader{contexts: []string{"dev-euw1", "prd-euw1"}}

	err := execute(
		[]string{"__complete-contexts"},
		&stdout,
		&bytes.Buffer{},
		t.TempDir(),
		loader,
		&fakeKubectl{},
		&fakeSelector{},
		&fakeShell{},
	)
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	if got, want := stdout.String(), "dev-euw1\nprd-euw1\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestExecutePassesKubeconfigFlagToCompletionCandidates(t *testing.T) {
	loader := &fakeLoader{contexts: []string{"dev-euw1"}}

	err := execute(
		[]string{"__complete-contexts", "--kubeconfig", "/tmp/example"},
		&bytes.Buffer{},
		&bytes.Buffer{},
		t.TempDir(),
		loader,
		&fakeKubectl{},
		&fakeSelector{},
		&fakeShell{},
	)
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	if loader.kubeconfig != "/tmp/example" {
		t.Fatalf("loader kubeconfig = %q, want %q", loader.kubeconfig, "/tmp/example")
	}
}

func TestExecuteRejectsContextForCompletionCandidates(t *testing.T) {
	err := execute(
		[]string{"__complete-contexts", "prd-euw1"},
		&bytes.Buffer{},
		&bytes.Buffer{},
		t.TempDir(),
		&fakeLoader{},
		&fakeKubectl{},
		&fakeSelector{},
		&fakeShell{},
	)
	if !errors.Is(err, errUsage) {
		t.Fatalf("error = %v, want %v", err, errUsage)
	}
}

func TestExecuteRejectsUnsupportedCompletionShell(t *testing.T) {
	err := execute(
		[]string{"completion", "bash"},
		&bytes.Buffer{},
		&bytes.Buffer{},
		t.TempDir(),
		&fakeLoader{},
		&fakeKubectl{},
		&fakeSelector{},
		&fakeShell{},
	)
	if err == nil {
		t.Fatal("execute returned nil, want error")
	}
	if got, want := err.Error(), "usage: kctx completion zsh"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func TestExecutePrintsUsageForHelpFlag(t *testing.T) {
	var stdout bytes.Buffer

	err := execute(
		[]string{"--help"},
		&stdout,
		&bytes.Buffer{},
		t.TempDir(),
		&fakeLoader{},
		&fakeKubectl{},
		&fakeSelector{},
		&fakeShell{},
	)
	if err != nil {
		t.Fatalf("execute returned error: %v", err)
	}

	for _, want := range []string{"usage: kctx", "--kubeconfig", "--limit"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("usage output does not contain %q:\n%s", want, stdout.String())
		}
	}
}

func TestRunCreatesIsolatedKubeconfig(t *testing.T) {
	var stderr bytes.Buffer
	runner := &fakeKubectl{
		viewContents: []byte("apiVersion: v1\ncurrent-context: example\n"),
	}
	shell := &fakeShell{}

	err := run(
		[]string{"example"},
		&stderr,
		t.TempDir(),
		&fakeLoader{},
		runner,
		&fakeSelector{},
		shell,
	)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	if runner.viewContext != "example" {
		t.Fatalf("view context = %q, want %q", runner.viewContext, "example")
	}
	if runner.useContext != "example" {
		t.Fatalf("use context = %q, want %q", runner.useContext, "example")
	}
	if runner.useKubeconfig == "" {
		t.Fatal("use kubeconfig is empty")
	}
	if shell.kubeconfig != runner.useKubeconfig {
		t.Fatalf("shell kubeconfig = %q, want %q", shell.kubeconfig, runner.useKubeconfig)
	}
	if got := shell.mode.Perm(); got != 0o600 {
		t.Fatalf("kubeconfig permissions while shell runs = %o, want 600", got)
	}
	if got, want := string(shell.contents), string(runner.viewContents); got != want {
		t.Fatalf("kubeconfig contents = %q, want %q", got, want)
	}
	if _, statErr := os.Stat(runner.useKubeconfig); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("stat kubeconfig after shell exit returned %v, want os.ErrNotExist", statErr)
	}
	if got := stderr.String(); !strings.Contains(got, "Using Kubernetes context 'example' via ") {
		t.Fatalf("stderr = %q, want success message", got)
	}
}

func TestRunSelectsContextWhenNoneProvided(t *testing.T) {
	loader := &fakeLoader{contexts: []string{"dev-euw1", "prd-euw1"}}
	runner := &fakeKubectl{viewContents: []byte("apiVersion: v1\n")}
	selector := &fakeSelector{selected: "prd-euw1"}
	shell := &fakeShell{}

	err := run(nil, &bytes.Buffer{}, t.TempDir(), loader, runner, selector, shell)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	if !slices.Equal(selector.contexts, loader.contexts) {
		t.Fatalf("selector contexts = %#v, want %#v", selector.contexts, loader.contexts)
	}
	if selector.limit != kubecontext.DefaultLimit {
		t.Fatalf("selector limit = %d, want %d", selector.limit, kubecontext.DefaultLimit)
	}
	if runner.viewContext != "prd-euw1" {
		t.Fatalf("view context = %q, want %q", runner.viewContext, "prd-euw1")
	}
	if shell.kubeconfig == "" {
		t.Fatal("shell was not started")
	}
}

func TestRunPassesFlagsToLoaderKubectlAndSelector(t *testing.T) {
	loader := &fakeLoader{contexts: []string{"dev-euw1", "prd-euw1"}}
	runner := &fakeKubectl{viewContents: []byte("apiVersion: v1\n")}
	selector := &fakeSelector{selected: "prd-euw1"}
	path := filepath.Join(t.TempDir(), "config")

	err := run(
		[]string{"--kubeconfig", path, "--limit", "3"},
		&bytes.Buffer{},
		t.TempDir(),
		loader,
		runner,
		selector,
		&fakeShell{},
	)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	if loader.kubeconfig != path {
		t.Fatalf("loader kubeconfig = %q, want %q", loader.kubeconfig, path)
	}
	if selector.limit != 3 {
		t.Fatalf("selector limit = %d, want 3", selector.limit)
	}
	if runner.viewKubeconfig != path {
		t.Fatalf("view kubeconfig = %q, want %q", runner.viewKubeconfig, path)
	}
}

func TestRunRejectsLimitsBelowOne(t *testing.T) {
	err := run(
		[]string{"--limit", "0"},
		&bytes.Buffer{},
		t.TempDir(),
		&fakeLoader{},
		&fakeKubectl{},
		&fakeSelector{},
		&fakeShell{},
	)
	if !errors.Is(err, kubecontext.ErrInvalidLimit) {
		t.Fatalf("error = %v, want %v", err, kubecontext.ErrInvalidLimit)
	}
}

func TestRunReportsMissingContexts(t *testing.T) {
	err := run(
		nil,
		&bytes.Buffer{},
		t.TempDir(),
		&fakeLoader{},
		&fakeKubectl{},
		&fakeSelector{err: kubecontext.ErrNoContexts},
		&fakeShell{},
	)
	if !errors.Is(err, kubecontext.ErrNoContexts) {
		t.Fatalf("error = %v, want %v", err, kubecontext.ErrNoContexts)
	}
}

func TestRunStopsWhenContextSelectionIsCancelled(t *testing.T) {
	tempDir := t.TempDir()
	loader := &fakeLoader{contexts: []string{"dev-euw1", "prd-euw1"}}
	runner := &fakeKubectl{}

	err := run(
		nil,
		&bytes.Buffer{},
		tempDir,
		loader,
		runner,
		&fakeSelector{},
		&fakeShell{},
	)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if runner.viewContext != "" {
		t.Fatalf("view context = %q, want empty", runner.viewContext)
	}

	entries, readErr := os.ReadDir(tempDir)
	if readErr != nil {
		t.Fatalf("read temp directory: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("temp directory contains %d entries, want none", len(entries))
	}
}

func TestRunRemovesKubeconfigWhenKubectlViewFails(t *testing.T) {
	tempDir := t.TempDir()
	runner := &fakeKubectl{viewErr: errors.New("view failed")}

	err := run(
		[]string{"example"},
		&bytes.Buffer{},
		tempDir,
		&fakeLoader{},
		runner,
		&fakeSelector{},
		&fakeShell{},
	)
	if err == nil {
		t.Fatal("run returned nil, want error")
	}

	entries, readErr := os.ReadDir(tempDir)
	if readErr != nil {
		t.Fatalf("read temp directory: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("temp directory contains %d entries, want none", len(entries))
	}
}

func TestRunRemovesKubeconfigWhenUseContextFails(t *testing.T) {
	tempDir := t.TempDir()
	runner := &fakeKubectl{
		viewContents: []byte("apiVersion: v1\n"),
		useErr:       errors.New("use failed"),
	}

	err := run(
		[]string{"example"},
		&bytes.Buffer{},
		tempDir,
		&fakeLoader{},
		runner,
		&fakeSelector{},
		&fakeShell{},
	)
	if err == nil {
		t.Fatal("run returned nil, want error")
	}

	if _, statErr := os.Stat(runner.useKubeconfig); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("stat removed kubeconfig returned %v, want os.ErrNotExist", statErr)
	}
}

func TestRunRemovesKubeconfigWhenShellFails(t *testing.T) {
	runner := &fakeKubectl{viewContents: []byte("apiVersion: v1\n")}
	shell := &fakeShell{runErr: errors.New("shell failed")}

	err := run(
		[]string{"example"},
		&bytes.Buffer{},
		t.TempDir(),
		&fakeLoader{},
		runner,
		&fakeSelector{},
		shell,
	)
	if err == nil {
		t.Fatal("run returned nil, want error")
	}

	if _, statErr := os.Stat(shell.kubeconfig); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("stat kubeconfig after shell failure returned %v, want os.ErrNotExist", statErr)
	}
}

func TestRunRejectsMoreThanOneContext(t *testing.T) {
	err := run(
		[]string{"one", "two"},
		&bytes.Buffer{},
		t.TempDir(),
		&fakeLoader{},
		&fakeKubectl{},
		&fakeSelector{},
		&fakeShell{},
	)
	if !errors.Is(err, errUsage) {
		t.Fatalf("error = %v, want %v", err, errUsage)
	}
}

func TestRunRejectsUnknownFlags(t *testing.T) {
	err := run(
		[]string{"--nope"},
		&bytes.Buffer{},
		t.TempDir(),
		&fakeLoader{},
		&fakeKubectl{},
		&fakeSelector{},
		&fakeShell{},
	)
	if !errors.Is(err, errUsage) {
		t.Fatalf("error = %v, want %v", err, errUsage)
	}
}

func TestReplaceEnvironmentOverridesKubeconfig(t *testing.T) {
	got := replaceEnvironment(
		[]string{"HOME=/home/example", "KUBECONFIG=/default/config"},
		"KUBECONFIG",
		"/temporary/config",
	)
	want := []string{"HOME=/home/example", "KUBECONFIG=/temporary/config"}

	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("replaceEnvironment() = %#v, want %#v", got, want)
	}
}

type fakeLoader struct {
	contexts   []string
	err        error
	kubeconfig string
	calls      int
}

func (loader *fakeLoader) Contexts(kubeconfig string) ([]string, error) {
	loader.calls++
	loader.kubeconfig = kubeconfig
	return loader.contexts, loader.err
}

type fakeKubectl struct {
	viewContext    string
	viewKubeconfig string
	viewContents   []byte
	viewErr        error
	useContext     string
	useKubeconfig  string
	useErr         error
}

func (runner *fakeKubectl) View(context string, kubeconfig string, destination *os.File) error {
	runner.viewContext = context
	runner.viewKubeconfig = kubeconfig
	if runner.viewErr != nil {
		return runner.viewErr
	}

	_, err := destination.Write(runner.viewContents)
	return err
}

func (runner *fakeKubectl) UseContext(context string, path string) error {
	runner.useContext = context
	runner.useKubeconfig = path
	return runner.useErr
}

type fakeSelector struct {
	contexts []string
	limit    int
	selected string
	err      error
}

func (selector *fakeSelector) Select(contexts []string, limit int) (string, error) {
	selector.contexts = contexts
	selector.limit = limit
	return selector.selected, selector.err
}

type fakeShell struct {
	kubeconfig string
	mode       os.FileMode
	contents   []byte
	runErr     error
}

func (shell *fakeShell) Run(kubeconfig string) error {
	shell.kubeconfig = kubeconfig

	info, err := os.Stat(kubeconfig)
	if err != nil {
		return err
	}
	shell.mode = info.Mode()

	shell.contents, err = os.ReadFile(kubeconfig)
	if err != nil {
		return err
	}

	return shell.runErr
}
