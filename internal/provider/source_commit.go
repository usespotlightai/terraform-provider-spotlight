package provider

import (
	"regexp"
	"strings"
)

const gitHubServer = "https://github.com"

var (
	gitHubRepository = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)
	commitSHA        = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)
)

// sourceCommit names the commit a GitHub Actions job runs on, as
// owner/name@sha, or is empty outside a job on github.com.
func sourceCommit(getenv func(string) string) string {
	repository := strings.TrimSpace(getenv(GitHubRepositoryEnv))
	sha := strings.ToLower(strings.TrimSpace(getenv(GitHubSHAEnv)))
	server := strings.TrimRight(strings.TrimSpace(getenv(GitHubServerURLEnv)), "/")

	if server != "" && server != gitHubServer {
		return ""
	}

	if !gitHubRepository.MatchString(repository) || !commitSHA.MatchString(sha) {
		return ""
	}

	return repository + "@" + sha
}
