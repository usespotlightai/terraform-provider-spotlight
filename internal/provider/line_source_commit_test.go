package provider_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/acctest"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/fakeapi"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/provider"
)

const sourceCommitHeader = "Spotlight-Source-Commit"

const lineSourceSHA = "3f1c9a7e5b2d4c6f8a0e1b3d5c7f9a2e4b6d8c0f"

func lineWrite(each fakeapi.Request) bool {
	return (each.Method == "POST" && strings.HasSuffix(each.Path, "/lines")) ||
		(each.Method == "PUT" && strings.Contains(each.Path, "/resources/lines/"))
}

func sentSourceCommit(fake *fakeapi.Server, want string) func(*terraform.State) error {
	return func(*terraform.State) error {
		writes := 0

		for _, each := range fake.Requests() {
			got := each.Headers.Get(sourceCommitHeader)

			if !lineWrite(each) {
				if got != "" {
					return fmt.Errorf("%s %s sent %s %q", each.Method, each.Path, sourceCommitHeader, got)
				}

				continue
			}

			writes++

			if got != want {
				return fmt.Errorf("%s %s sent %s %q, want %q", each.Method, each.Path, sourceCommitHeader, got, want)
			}
		}

		if writes == 0 {
			return fmt.Errorf("no line was written")
		}

		return nil
	}
}

func TestLineWritesNameTheCommitOfTheGitHubActionsJob(t *testing.T) {
	fake := fakeapi.New(t)
	t.Setenv(provider.GitHubRepositoryEnv, "acme/spotlight-config")
	t.Setenv(provider.GitHubSHAEnv, lineSourceSHA)
	t.Setenv(provider.GitHubServerURLEnv, "https://github.com")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace + lineMinimal,
				Check:  sentSourceCommit(fake, "acme/spotlight-config@"+lineSourceSHA),
			},
			{
				Config: acctest.Workspace + lineGrown,
				Check:  sentSourceCommit(fake, "acme/spotlight-config@"+lineSourceSHA),
			},
		},
	})
}

func TestLineWritesNameNoCommitOutsideGitHubActions(t *testing.T) {
	fake := fakeapi.New(t)
	t.Setenv(provider.GitHubRepositoryEnv, "")
	t.Setenv(provider.GitHubSHAEnv, "")
	t.Setenv(provider.GitHubServerURLEnv, "")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: acctest.FakeProviderFactories(fake),
		Steps: []resource.TestStep{
			{
				Config: acctest.Workspace + lineMinimal,
				Check:  sentSourceCommit(fake, ""),
			},
			{
				Config: acctest.Workspace + lineGrown,
				Check:  sentSourceCommit(fake, ""),
			},
		},
	})
}
