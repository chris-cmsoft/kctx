package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCurrentVersionPrefersBuildFlag(t *testing.T) {
	t.Cleanup(func() { version = "" })
	version = "v9.9.9"

	if got := currentVersion(); got != "v9.9.9" {
		t.Fatalf("currentVersion() = %q, want %q", got, "v9.9.9")
	}
}

func TestCurrentVersionFallsBackToBuildInfo(t *testing.T) {
	if got := currentVersion(); got == "" {
		t.Fatal("currentVersion() is empty without a build flag")
	}
}

func TestLatestVersionReadsTagName(t *testing.T) {
	var accept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept = r.Header.Get("Accept")
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3","name":"v1.2.3"}`))
	}))
	t.Cleanup(server.Close)
	useEndpoint(t, server.URL)

	got, err := latestVersion(context.Background())
	if err != nil {
		t.Fatalf("latestVersion returned error: %v", err)
	}
	if got != "v1.2.3" {
		t.Fatalf("latestVersion() = %q, want %q", got, "v1.2.3")
	}
	if accept != "application/vnd.github+json" {
		t.Fatalf("Accept header = %q, want %q", accept, "application/vnd.github+json")
	}
}

func TestLatestVersionReportsMissingReleases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	useEndpoint(t, server.URL)

	_, err := latestVersion(context.Background())
	if err == nil {
		t.Fatal("latestVersion returned nil, want error")
	}
	if got, want := err.Error(), "no releases published yet"; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func TestLatestVersionReportsUnexpectedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(server.Close)
	useEndpoint(t, server.URL)

	_, err := latestVersion(context.Background())
	if err == nil {
		t.Fatal("latestVersion returned nil, want error")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Fatalf("error = %q, want it to mention the status", err)
	}
}

func TestLatestVersionRejectsResponseWithoutTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)
	useEndpoint(t, server.URL)

	if _, err := latestVersion(context.Background()); err == nil {
		t.Fatal("latestVersion returned nil, want error")
	}
}

func TestWriteVersion(t *testing.T) {
	tests := []struct {
		name      string
		current   string
		latest    string
		lookupErr error
		want      []string
	}{
		{
			name:    "up to date",
			current: "v1.2.3",
			latest:  "v1.2.3",
			want:    []string{"kctx    v1.2.3\n", "latest  v1.2.3 (up to date)\n"},
		},
		{
			name:    "build metadata is still the same release",
			current: "v1.2.3+dirty",
			latest:  "v1.2.3",
			want:    []string{"kctx    v1.2.3+dirty\n", "latest  v1.2.3 (up to date)\n"},
		},
		{
			name:    "newer release links the downloads",
			current: "v1.2.3",
			latest:  "v1.3.0",
			want:    []string{"kctx    v1.2.3\n", "latest  v1.3.0 (" + releasesPage + ")\n"},
		},
		{
			name:      "failed lookup still reports the running version",
			current:   "v1.2.3",
			lookupErr: context.DeadlineExceeded,
			want:      []string{"kctx    v1.2.3\n", "latest  unknown (context deadline exceeded)\n"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := writeVersion(&out, test.current, test.latest, test.lookupErr); err != nil {
				t.Fatalf("writeVersion returned error: %v", err)
			}
			if got, want := out.String(), strings.Join(test.want, ""); got != want {
				t.Fatalf("writeVersion() wrote %q, want %q", got, want)
			}
		})
	}
}

func TestExecuteReportsVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3"}`))
	}))
	t.Cleanup(server.Close)
	useEndpoint(t, server.URL)
	t.Cleanup(func() { version = "" })
	version = "v1.2.3"

	for _, flag := range []string{"--version", "-version"} {
		var stdout bytes.Buffer
		err := execute(
			[]string{flag},
			&stdout,
			&bytes.Buffer{},
			t.TempDir(),
			&fakeLoader{},
			&fakeKubectl{},
			&fakeSelector{},
			&fakeShell{},
		)
		if err != nil {
			t.Fatalf("execute(%s) returned error: %v", flag, err)
		}

		want := "kctx    v1.2.3\nlatest  v1.2.3 (up to date)\n"
		if got := stdout.String(); got != want {
			t.Fatalf("execute(%s) wrote %q, want %q", flag, got, want)
		}
	}
}

func useEndpoint(t *testing.T, url string) {
	t.Helper()
	previous := releasesEndpoint
	t.Cleanup(func() { releasesEndpoint = previous })
	releasesEndpoint = url
}
