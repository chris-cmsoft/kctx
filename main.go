package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	kubecontext "github.com/chris-cmsoft/gotool-kubecontext-picker"
)

const usage = `usage: kctx [--kubeconfig path] [--limit n] [context]

Flags:
      --kubeconfig string   Path to kubeconfig file
      --limit int           Maximum contexts to show (default 9)
      --version             Show the running and latest released version`

var (
	errUsage = errors.New(usage)
	errHelp  = errors.New("help requested")
)

const zshCompletion = `#compdef kctx

_kctx() {
  if (( CURRENT != 2 )); then
    return
  fi

  local -a contexts
  contexts=("${(@f)$(command kctx __complete-contexts 2>/dev/null)}")
  _describe 'Kubernetes context' contexts
}

compdef _kctx kctx
`

func main() {
	err := execute(
		os.Args[1:],
		os.Stdout,
		os.Stderr,
		os.TempDir(),
		kubeconfigLoader{},
		kubectlCommand{stderr: os.Stderr},
		interactiveSelector{},
		interactiveShell{
			path:   os.Getenv("SHELL"),
			stdin:  os.Stdin,
			stdout: os.Stdout,
			stderr: os.Stderr,
		},
	)
	if err == nil {
		return
	}

	fmt.Fprintln(os.Stderr, err)
	if errors.Is(err, errUsage) {
		os.Exit(2)
	}
	os.Exit(1)
}

func execute(
	args []string,
	out io.Writer,
	errOut io.Writer,
	tempDir string,
	loader contextLoader,
	kubectl kubectlRunner,
	selector contextSelector,
	shell shellRunner,
) error {
	if len(args) == 1 && (args[0] == "--version" || args[0] == "-version") {
		return reportVersion(context.Background(), out)
	}

	if len(args) > 0 && args[0] == "completion" {
		if len(args) != 2 || args[1] != "zsh" {
			return errors.New("usage: kctx completion zsh")
		}
		if _, err := io.WriteString(out, zshCompletion); err != nil {
			return fmt.Errorf("write zsh completion: %w", err)
		}
		return nil
	}

	if len(args) > 0 && args[0] == "__complete-contexts" {
		opts, err := parseOptions(args[1:], false)
		if err != nil {
			return err
		}

		contexts, err := loader.Contexts(opts.kubeconfig)
		if err != nil {
			return err
		}
		for _, context := range contexts {
			if _, err := fmt.Fprintln(out, context); err != nil {
				return fmt.Errorf("write Kubernetes contexts: %w", err)
			}
		}
		return nil
	}

	err := run(args, errOut, tempDir, loader, kubectl, selector, shell)
	if errors.Is(err, errHelp) {
		_, err = fmt.Fprintln(out, usage)
	}
	return err
}

func run(
	args []string,
	errOut io.Writer,
	tempDir string,
	loader contextLoader,
	kubectl kubectlRunner,
	selector contextSelector,
	shell shellRunner,
) error {
	opts, err := parseOptions(args, true)
	if err != nil {
		return err
	}

	context := opts.context
	if context == "" {
		contexts, err := loader.Contexts(opts.kubeconfig)
		if err != nil {
			return err
		}

		context, err = selector.Select(contexts, opts.limit)
		if err != nil {
			return err
		}
		if context == "" {
			return nil
		}
	}

	kubeconfig, err := os.CreateTemp(tempDir, "kubeconfig.*")
	if err != nil {
		return fmt.Errorf("create temporary kubeconfig: %w", err)
	}

	path := kubeconfig.Name()
	defer func() {
		_ = os.Remove(path)
	}()

	if err := kubeconfig.Chmod(0o600); err != nil {
		_ = kubeconfig.Close()
		return fmt.Errorf("secure temporary kubeconfig: %w", err)
	}

	if err := kubectl.View(context, opts.kubeconfig, kubeconfig); err != nil {
		_ = kubeconfig.Close()
		return fmt.Errorf("load Kubernetes context %q: %w", context, err)
	}
	if err := kubeconfig.Close(); err != nil {
		return fmt.Errorf("close temporary kubeconfig: %w", err)
	}

	if err := kubectl.UseContext(context, path); err != nil {
		return fmt.Errorf("select Kubernetes context %q: %w", context, err)
	}

	fmt.Fprintf(errOut, "Using Kubernetes context '%s' via %s\n", context, path)
	if err := shell.Run(path); err != nil {
		return fmt.Errorf("run isolated shell: %w", err)
	}
	return nil
}

type options struct {
	kubeconfig string
	limit      int
	context    string
}

// parseOptions reads the shared kubeconfig and limit flags. Flags come before
// the optional context, so "kctx --limit 3 prd-euw1" parses but
// "kctx prd-euw1 --limit 3" does not.
func parseOptions(args []string, allowContext bool) (options, error) {
	opts := options{limit: kubecontext.DefaultLimit}

	flags := flag.NewFlagSet("kctx", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.kubeconfig, "kubeconfig", "", "Path to kubeconfig file")
	flags.IntVar(&opts.limit, "limit", opts.limit, "Maximum contexts to show")

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return options{}, errHelp
		}
		return options{}, errors.Join(err, errUsage)
	}

	remaining := flags.Args()
	if len(remaining) > 1 || (len(remaining) == 1 && !allowContext) {
		return options{}, errUsage
	}
	if len(remaining) == 1 {
		opts.context = remaining[0]
	}
	if opts.limit < 1 {
		return options{}, kubecontext.ErrInvalidLimit
	}

	return opts, nil
}

type contextLoader interface {
	Contexts(kubeconfig string) ([]string, error)
}

type kubeconfigLoader struct{}

func (kubeconfigLoader) Contexts(kubeconfig string) ([]string, error) {
	return kubecontext.Load(kubeconfig)
}

type kubectlRunner interface {
	// View writes a flattened, single context kubeconfig for context to
	// destination, reading from kubeconfig when it is not empty.
	View(context string, kubeconfig string, destination *os.File) error

	// UseContext points the kubeconfig at path to context.
	UseContext(context string, path string) error
}

type kubectlCommand struct {
	stderr io.Writer
}

func (runner kubectlCommand) View(context string, kubeconfig string, destination *os.File) error {
	args := []string{"--context", context}
	if kubeconfig != "" {
		args = append(args, "--kubeconfig", kubeconfig)
	}
	args = append(args, "config", "view", "--flatten", "--minify", "--raw")

	command := exec.Command("kubectl", args...)
	command.Stdout = destination
	command.Stderr = runner.stderr
	return command.Run()
}

func (runner kubectlCommand) UseContext(context string, path string) error {
	command := exec.Command("kubectl", "config", "use-context", context)
	command.Env = replaceEnvironment(os.Environ(), "KUBECONFIG", path)
	command.Stdout = io.Discard
	command.Stderr = runner.stderr
	return command.Run()
}

type shellRunner interface {
	Run(kubeconfig string) error
}

type contextSelector interface {
	Select(contexts []string, limit int) (string, error)
}

type interactiveSelector struct{}

func (interactiveSelector) Select(contexts []string, limit int) (string, error) {
	return kubecontext.Select(contexts, limit)
}

type interactiveShell struct {
	path   string
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

func (shell interactiveShell) Run(kubeconfig string) error {
	path := shell.path
	if path == "" {
		path = "/bin/sh"
	}

	command := exec.Command(path, "-l")
	command.Env = replaceEnvironment(os.Environ(), "KUBECONFIG", kubeconfig)
	command.Stdin = shell.stdin
	command.Stdout = shell.stdout
	command.Stderr = shell.stderr
	return command.Run()
}

func replaceEnvironment(environment []string, name string, value string) []string {
	prefix := name + "="
	result := make([]string, 0, len(environment)+1)
	for _, variable := range environment {
		if !strings.HasPrefix(variable, prefix) {
			result = append(result, variable)
		}
	}

	return append(result, prefix+value)
}
