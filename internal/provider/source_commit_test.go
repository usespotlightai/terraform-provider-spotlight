package provider

import "testing"

const testSHA = "3f1c9a7e5b2d4c6f8a0e1b3d5c7f9a2e4b6d8c0f"

func environment(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

func TestSourceCommitNamesTheCommitOfAGitHubActionsJob(t *testing.T) {
	got := sourceCommit(environment(map[string]string{
		GitHubRepositoryEnv: "acme/spotlight-config",
		GitHubSHAEnv:        testSHA,
		GitHubServerURLEnv:  "https://github.com",
	}))

	if want := "acme/spotlight-config@" + testSHA; got != want {
		t.Fatalf("sourceCommit = %q, want %q", got, want)
	}
}

func TestSourceCommitIsEmptyWithoutAGitHubCommit(t *testing.T) {
	for name, values := range map[string]map[string]string{
		"outside GitHub Actions": {},
		"without a commit": {
			GitHubRepositoryEnv: "acme/spotlight-config",
			GitHubServerURLEnv:  "https://github.com",
		},
		"without a repository": {
			GitHubSHAEnv:       testSHA,
			GitHubServerURLEnv: "https://github.com",
		},
		"on GitHub Enterprise Server": {
			GitHubRepositoryEnv: "acme/spotlight-config",
			GitHubSHAEnv:        testSHA,
			GitHubServerURLEnv:  "https://github.acme.internal",
		},
		"with a branch for a commit": {
			GitHubRepositoryEnv: "acme/spotlight-config",
			GitHubSHAEnv:        "main",
			GitHubServerURLEnv:  "https://github.com",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := sourceCommit(environment(values)); got != "" {
				t.Fatalf("sourceCommit = %q, want none", got)
			}
		})
	}
}
