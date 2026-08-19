```
kctx() {
  if [ "$#" -ne 1 ]; then
    echo "Usage: kctx <context>" >&2
    return 2
  fi

  local context="$1"
  local tmp

  tmp="$(mktemp "${TMPDIR:-/tmp}/kubeconfig.XXXXXX")" || return 1
  chmod 600 "$tmp"

  if ! command kubectl --context "$context" config view \
      --flatten --minify --raw >"$tmp"; then
    rm -f "$tmp"
    return 1
  fi

  if ! KUBECONFIG="$tmp" command kubectl config use-context "$context" >/dev/null; then
    rm -f "$tmp"
    return 1
  fi

  export KUBECONFIG="$tmp"
  echo "Using Kubernetes context '$context' via $KUBECONFIG"
}
```