package service

import (
	"context"
	"testing"

	"digital-contracting-service/internal/buildinfo"
)

func TestInfoGetReportsUnknownAndOmitsEmptyBuildTime(t *testing.T) {
	res, err := NewInfo().Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.GitSha != "unknown" {
		t.Fatalf("git_sha = %q, want unknown when no ldflags are set", res.GitSha)
	}
	if res.BuiltAt != nil {
		t.Fatalf("built_at = %q, want omitted when empty", *res.BuiltAt)
	}
}

func TestInfoGetReportsLinkedBuildValues(t *testing.T) {
	sha, builtAt := buildinfo.GitSHA, buildinfo.BuiltAt
	t.Cleanup(func() { buildinfo.GitSHA, buildinfo.BuiltAt = sha, builtAt })
	buildinfo.GitSHA = "0123456789abcdef"
	buildinfo.BuiltAt = "2026-10-28T12:00:00Z"

	res, err := NewInfo().Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.GitSha != "0123456789abcdef" {
		t.Fatalf("git_sha = %q", res.GitSha)
	}
	if res.BuiltAt == nil || *res.BuiltAt != "2026-10-28T12:00:00Z" {
		t.Fatalf("built_at = %v", res.BuiltAt)
	}
}
