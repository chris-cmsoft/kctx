# kctx

Create a temporary, single-context kubeconfig without changing the default
kubeconfig used by other terminal windows.

## Install

```console
go install github.com/chris-cmsoft/kctx@latest
```

Or build locally:

```console
make build
```

## Usage

Run `kctx` without arguments to open the interactive context picker:

```console
kctx
```

Type to filter contexts. Space-delimited terms are combined, so `prd euw1`
matches context names containing both `prd` and `euw1`. Use the arrow keys to
move, Enter to select, or Esc/Ctrl-C to cancel. The picker is
[kubecontext](https://github.com/chris-cmsoft/gotool-kubecontext-picker),
shared with [knein](https://github.com/chris-cmsoft/knein), so both tools filter
and select identically.

You can still provide a context directly to skip the picker:

```console
kctx my-context
```

### Flags

| Flag | Default | Purpose |
| --- | --- | --- |
| `--kubeconfig` | standard resolution (`$KUBECONFIG`, then `~/.kube/config`) | Path to the kubeconfig to read contexts from. |
| `--limit` | `9` | Maximum contexts the picker shows at once. |

Flags come before the optional context, so `kctx --limit 3 my-context` parses
but `kctx my-context --limit 3` does not.

## zsh completion

Load the generated completion after zsh's completion system is initialized:

```zsh
autoload -Uz compinit
compinit
source <(kctx completion zsh)
```

If a shell framework already initializes completion, only the `source` line is
needed. Open a new shell or reload `.zshrc`, then use:

```console
kctx <Tab><Tab>
```

To select completions with the arrow keys, add:

```zsh
zstyle ':completion:*' menu select
```

The Go command creates the temporary file with mode `0600`, copies only the
selected context into it using `kubectl config view --flatten --minify --raw`,
and starts a login shell in the current terminal with `KUBECONFIG` pointing to
that file.

Run `exit` or press Ctrl-D to leave the isolated shell and return to the
original shell. The temporary kubeconfig is removed when the isolated shell
exits.
