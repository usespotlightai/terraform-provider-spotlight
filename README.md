# Terraform Provider for Spotlight

Configure a Spotlight workspace as code: its teams, the lines their tasks move along, the skills and harness profiles their agents are given, the integrations that connect them to other tools, the secrets those integrations wear, and the runners the work runs on.

| Resource | Holds | Writes need | Imported as |
| --- | --- | --- | --- |
| `spotlight_workspace` | `name`, `slug` (adopted with an `import` block, not created) | `configure` | `<slug>` |
| `spotlight_team` | `workspace_id`, `key`, `name` | `configure` | `<workspace id>/<key>` |
| `spotlight_line` | `team_id`, `key`, `name`, `role`, `repositories`, `runners`, `station {}` blocks (with `profile`, `harness_version` and `runners`), `reaction {}` blocks, one `reviews { skip {} }` | `configure` | `<workspace id>/<team key>/<key>` |
| `spotlight_skill` | `team_id`, `key`, `files` | `configure` | `<workspace id>/<team key>/<key>` |
| `spotlight_integration` | `team_id`, `definition` (the whole definition, verbatim) | `configure` | `<workspace id>/<team key>/<source>` |
| `spotlight_harness_profile` | `team_id`, `key`, `permissions` (`allow`, `ask`, `deny`), `mcp_servers`, `native` (each harness's own settings) | `configure` | `<workspace id>/<team key>/<key>` |
| `spotlight_secret` | `workspace_id`, `name`, `source`, `bindings`, write-only `value_wo` with `value_wo_version` | `secrets` | `<workspace id>/<name>` |
| `spotlight_runner` | `workspace_id`, `name`, `labels`, write-only `token` with `token_version` | `runners` | `<workspace id>/<name>` |
| `spotlight_runner_trust` | `workspace_id`, `key`, `issuer`, `subject`, `audience`, `labels` | `runners` | `<workspace id>/<key>` |
| `spotlight_workspace_runners` | `workspace_id`, `runners`, `trusts` (one per workspace; `drift` read-only) | `runners` | `<workspace id>/<slug>` |

Every resource has a resource identity, its `id`, and is also served as a list resource, so `terraform query` (Terraform 1.14 and later) can list what a workspace holds. Two data sources read without managing: `spotlight_workspace` by its `slug`, and `spotlight_team` by its `workspace_id` and `key`.

A workspace is created by a person signing up in the web app. Terraform adopts it with an `import` block, then manages everything inside it:

```hcl
import {
  to = spotlight_workspace.acme
  id = "acme"
}

resource "spotlight_workspace" "acme" {
  name = "Acme"
  slug = "acme"
}
```

The provider can still create a workspace, but only for a caller allowed to open workspaces, which API tokens are not: a token acts only inside its own workspace, so the create is refused with `403` and the provider says to import the workspace instead. To stop managing a workspace without deleting it, replace its resource with a `removed { from = spotlight_workspace.acme  lifecycle { destroy = false } }` block.

`spotlight export` renders an existing workspace in exactly this shape, with an `import` block per resource, so applying what it writes adopts the workspace and a second plan shows no change. `terraform query -generate-config-out=generated.tf` with a `list` block per resource type does the same from Terraform's side, one type at a time.

## Install

```hcl
terraform {
  required_version = ">= 1.11"

  required_providers {
    spotlight = {
      source = "usespotlightai/spotlight"
    }
  }
}
```

`terraform init` downloads it from the public registry. Terraform 1.5 or later is enough for everything except the write-only secret values and runner tokens, which need 1.11.

## Authenticate

The provider calls the control plane with a Spotlight API token, sent as `Authorization: Bearer <token>`. Every request also names the provider in `Spotlight-Client: terraform-provider/<version>` and asks for API version `2026-09-23`.

A token acts only inside the workspace it was created in, with one scope from the ladder `read` < `plan` < `configure`, and the grants `secrets` and `runners` beside it. `read` refreshes and imports, `plan` also checks expressions, and `configure` also creates, updates and deletes configuration. Writing a secret needs `secrets`; writing a runner, a runner trust or the workspace's runners needs `runners`. **One token granted `configure`, `secrets` and `runners` manages everything this provider can.** A write the token's scopes do not allow is refused with `403`, and the diagnostic names the scope the token lacks.

```shell
export SPOTLIGHT_URL=https://spotlight.example.com
export SPOTLIGHT_API_TOKEN=<token>
terraform plan
```

Or in configuration:

```hcl
provider "spotlight" {
  url   = "https://spotlight.example.com" # SPOTLIGHT_URL, which the spotlight CLI reads too
  token = var.spotlight_token             # SPOTLIGHT_API_TOKEN
}
```

## How it behaves

- **Drift.** Every refresh reads each resource whole. A change made elsewhere, in the web app or by another client, shows up as a planned update back to the configuration; a resource deleted elsewhere is planned to be created again.
- **Defaults.** Spotlight fills in what a definition leaves out (a station's colour, a review policy's skips, an integration's request method). The provider remembers what Spotlight answered after each apply, so the filled-in fields never show up as a diff; only a real change made since does.
- **Versions.** Each write sends the version Terraform last read as `If-Match`. A resource changed since is refused rather than overwritten: run `terraform plan` again to see the change.
- **Imports.** Import a resource by its import id (`<workspace id>/<address>`, or a workspace's slug) or, in Terraform 1.12 and later, by its identity: `import { to = ..., identity = { id = "<id>" } }`.
- **Retries.** Creates carry an `Idempotency-Key`, so a create retried after a dropped connection makes one resource.
- **Errors name attributes.** A refused write is reported on the attribute it is about, as in `station[1].passes_to`.
- **Values Terraform never keeps.** A secret's `value_wo` and a runner's `token` are write-only: sent to Spotlight, never stored in the plan or the state. Change `value_wo_version` or `token_version` to send a new one. A runner created without a token is issued one by Spotlight, answered only once; the provider does not keep it, and warns so.
- **Runner drift.** Every refresh of `spotlight_workspace_runners` warns about runners enrolled outside the declaration, and names that answer to no runner or trust; `drift` lists the undeclared runners.
- **Expressions are checked by `terraform plan`.** Write each CEL expression in an integration as `$${{ ... }}`, so Terraform leaves it alone. The plan sends each known expression to the control plane's check endpoint, with the whole definition, and fails with the attribute named:

  ```text
  Error: Invalid expression in definition.webhook.events[0].when

  No such key: tpye

    body.tpye == "Issue"
         ^^^^

  Did you mean body.type?
  ```

  Expressions that depend on values not known until apply are skipped, and checked by the apply.

## Develop locally

Build and install the provider into `$GOBIN` (by default `~/go/bin`):

```shell
make install
```

Point Terraform at it with a `dev_overrides` block in `~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "usespotlightai/spotlight" = "/Users/you/go/bin"
  }

  direct {}
}
```

Terraform then uses the local build for `usespotlightai/spotlight` and warns that overrides are in effect. Skip `terraform init` for configurations that use only this provider, and run `terraform plan` or `terraform apply` directly against a local control plane:

```shell
export SPOTLIGHT_URL=http://localhost:3000
export SPOTLIGHT_API_TOKEN=<token>
cd examples/workspace
terraform plan
```

Remove the block, or point `TF_CLI_CONFIG_FILE` at a file without it, to go back to registry releases. To debug, run `go run . -debug` and export the `TF_REATTACH_PROVIDERS` it prints.

## Layout

The repository follows HashiCorp's [provider scaffold](https://github.com/hashicorp/terraform-provider-scaffolding-framework):

| Path | Holds |
| --- | --- |
| `main.go` | Serves the provider. |
| `internal/provider/` | The provider (`provider.go`) and, per type, `<name>_resource.go`, `<name>_data_source.go` and `<name>_list_resource.go`, each tested beside it in `_test.go`. Shared helpers: `diagnostics.go` (API problems as diagnostics on the attribute they name), `attributes.go`, `import.go` (import ids and identity), `private_state.go`, `list_results.go`. Sweepers are in `sweep_test.go`. |
| `internal/client/` | The control plane's API client: `Client` with `CreateTeam`, `ReadTeam`, … per resource, the models and problems it answers. `apigen/` is generated from `api/openapi.json`. |
| `internal/acctest/` | Test helpers: provider factories, `PreCheck` for acceptance tests, checks, export fixtures. |
| `internal/fakeapi/` | An in-memory control plane the unit tests run against. |
| `internal/expressions/`, `internal/fieldpath/`, `internal/dynamic/`, `internal/writeonly/` | Plan-time expression checks, API field paths to Terraform attributes, dynamic values to JSON and back, write-only attributes. |
| `examples/`, `templates/`, `docs/` | Registry documentation: `docs/` is generated from the schema, `examples/` and `templates/` by `make docs`. |

## Test

```shell
make test     # unit tests: the client against httptest, and every resource planned, applied, read, imported (by id and by identity), listed and drifted against an in-memory control plane, and an export fixture applied from scratch and adopted with no diff
make check    # formatting, lint, vet, build and unit tests
```

The resource tests run the real `terraform` binary, so it must be on `PATH` (or set `TF_ACC_TERRAFORM_PATH`). Tests are skipped below the Terraform version they need: write-only values need 1.11, import by identity 1.12, and list resources 1.14.

Acceptance tests (`TestAcc…`) sit beside the resource they cover in `internal/provider`, with the cross-resource scenarios in `workspace_end_to_end_test.go` and `export_test.go`. They run against a dev control plane, inside the token's own workspace, under random keys (`tf-acc-<random>`, `TF_ACC_<RANDOM>`). The end-to-end test imports the workspace (planning no change to it), creates a team, a line, a skill and an integration, re-plans expecting no change, imports each resource, then forgets the workspace with a `removed` block and destroys what it created. One test per resource does the same for secrets (with a value rotated, and one read from AWS), runners (one with a token, one issued a token Spotlight answers once) and runner trusts, the workspace's runners (skipped where the workspace already declares them), and harness profiles with a line placed on runners. The export round trip seeds every resource kind through the API, runs `spotlight export`, deletes what it seeded, applies the export (creating those from scratch and importing everything else the workspace holds, failing before the apply if anything else would change), and re-plans expecting no diff; it is skipped against a control plane that serves no export. Every test checks afterwards that the workspace is unchanged and nothing it created is left. Others check that a misspelt expression fails the plan at its attribute, and that creating a workspace with a token is refused with the advice to import it.

```shell
export SPOTLIGHT_URL=https://dev.spotlight.example.com
export SPOTLIGHT_API_TOKEN=<token>        # a token in that workspace with configure, secrets and runners
export SPOTLIGHT_ACC_WORKSPACE=<slug>     # the token's workspace
make testacc
```

Without `TF_ACC`, `SPOTLIGHT_URL`, `SPOTLIGHT_API_TOKEN` and `SPOTLIGHT_ACC_WORKSPACE` they are skipped.

A run that dies part way leaves what it created behind. The sweepers delete it: everything in the workspace whose key or name starts `tf-acc-` or `TF_ACC_` (teams after what they hold). There is no sweeper for the workspace itself or its runner declaration, which a test run cannot tell apart from the real ones.

```shell
make sweep SWEEP=<slug>   # or with SPOTLIGHT_ACC_WORKSPACE set
```

## Generate

The API client in `internal/client/apigen` is generated with [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) from `api/openapi.json`, a copy of the control plane's `control-plane/openapi.json`, restricted to the resource and expression operations (`api/oapi-codegen.yaml`). Both tools are pinned in `go.mod` and run with `go tool`.

```shell
make openapi SPOTLIGHT_REPO=../spotlight   # copy the schema from a control plane checkout
make client                                # regenerate the client
make docs                                  # regenerate docs/ from the schema, examples/ and templates/
make lint                                   # golangci-lint, as configured in .golangci.yml
```

CI regenerates the client from the control plane's `develop` and fails when it differs from what is committed, so every API change arrives here as a reviewed client diff.

## Release

Push a version tag:

```shell
git tag v0.1.0
git push origin v0.1.0
```

The release workflow runs GoReleaser: per-platform zip archives, `terraform-provider-spotlight_<version>_SHA256SUMS` signed with the release GPG key, and `terraform-provider-spotlight_<version>_manifest.json` from `terraform-registry-manifest.json` (protocol 6.0). The Terraform Registry publishes each GitHub release once the repository is registered at [registry.terraform.io/publish](https://registry.terraform.io/publish/provider) under the `usespotlightai` namespace, with the GPG public key added to the namespace's signing keys.

## CI secrets

| Secret | Used by | What it is |
| --- | --- | --- |
| `SPOTLIGHT_REPO_TOKEN` | `generate.yml` | A fine-grained GitHub token with read access to the contents of the private `usespotlightai/spotlight` repository, to read `control-plane/openapi.json`. |
| `SPOTLIGHT_ACC_URL` | `acceptance.yml` | The dev control plane the acceptance tests run against. |
| `SPOTLIGHT_ACC_TOKEN` | `acceptance.yml` | A workspace API token on it with the scopes `configure`, `secrets` and `runners`, created in the workspace named by `SPOTLIGHT_ACC_WORKSPACE`. |
| `SPOTLIGHT_ACC_WORKSPACE` | `acceptance.yml` | The slug of that token's workspace. The tests configure it under random keys and leave it as they found it. |
| `GPG_PRIVATE_KEY` | `release.yml` | The armored private key that signs `SHA256SUMS`; its public key is registered with the registry namespace. |
| `GPG_PASSPHRASE` | `release.yml` | That key's passphrase. |

`GITHUB_TOKEN` is provided by Actions.

## License

[Apache License 2.0](LICENSE). Changes are recorded in [CHANGELOG.md](CHANGELOG.md).
