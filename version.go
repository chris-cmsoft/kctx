package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

// version is set by release builds with -ldflags "-X main.version=v1.2.3".
// Builds without it fall back to the module version Go records in the binary.
var version string

const (
	repository    = "chris-cmsoft/kctx"
	releasesPage  = "https://github.com/" + repository + "/releases/latest"
	lookupTimeout = 5 * time.Second
)

// releasesEndpoint is a variable so tests can point it at a local server.
var releasesEndpoint = "https://api.github.com/repos/" + repository + "/releases/latest"

// currentVersion reports the version of the running binary.
func currentVersion() string {
	if version != "" {
		return version
	}

	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" {
		return "unknown"
	}
	return info.Main.Version
}

// latestVersion returns the tag of the most recent published release.
func latestVersion(ctx context.Context) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesEndpoint, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "kctx/"+currentVersion())

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = response.Body.Close()
	}()

	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return "", errors.New("no releases published yet")
	default:
		return "", fmt.Errorf("GitHub returned %s", response.Status)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&release); err != nil {
		return "", fmt.Errorf("read GitHub response: %w", err)
	}
	if release.TagName == "" {
		return "", errors.New("GitHub response contained no tag name")
	}

	return release.TagName, nil
}

// reportVersion writes the running version and the latest release to out. A
// failed lookup is reported in place of the latest version instead of failing
// the command, so --version still answers the question it can answer offline.
func reportVersion(ctx context.Context, out io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()

	latest, lookupErr := latestVersion(ctx)
	return writeVersion(out, currentVersion(), latest, lookupErr)
}

// sameRelease reports whether current is a build of the latest release. Build
// metadata such as the "+dirty" Go records for a modified checkout does not
// change which release a build came from.
func sameRelease(current string, latest string) bool {
	base, _, _ := strings.Cut(current, "+")
	return base == latest
}

func writeVersion(out io.Writer, current string, latest string, lookupErr error) error {
	if _, err := fmt.Fprintf(out, "%-7s %s\n", "kctx", current); err != nil {
		return err
	}

	switch {
	case lookupErr != nil:
		_, err := fmt.Fprintf(out, "%-7s unknown (%v)\n", "latest", lookupErr)
		return err
	case sameRelease(current, latest):
		_, err := fmt.Fprintf(out, "%-7s %s (up to date)\n", "latest", latest)
		return err
	default:
		_, err := fmt.Fprintf(out, "%-7s %s (%s)\n", "latest", latest, releasesPage)
		return err
	}
}
