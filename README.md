# Terraform Provider for Spotlight

Configure a Spotlight workspace as code: its teams, the lines their tasks move along, the skills their agents are given, and the integrations that connect them to other tools.

| Resource | Holds | Imported as |
| --- | --- | --- |
| `spotlight_workspace` | `name`, `slug` | `<slug>` |
| `spotlight_team` | `workspace_id`, `key`, `name` | `<workspace id>/<key>` |
| `spotlight_line` | `team_id`, `key`, `name`, `role`, `repositories`, `station {}` blocks, `reaction {}` blocks, one `reviews { skip {} }` | `<workspace id>/<team key>/<key>` |
| `spotlight_skill` | `team_id`, `key`, `files` | `<workspace id>/<team key>/<key>` |
| `spotlight_integration` | `team_id`, `definition` (the whole definition, verbatim) | `<workspace id>/<team key>/<source>` |

`spotlight export` renders an existing workspace in exactly this shape, with an `import` block per resource, so applying what it writes adopts the workspace and a second plan shows no change.

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

`terraform init` downloads it from the public registry. Terraform 1.5 or later is enough for everything except write-only secret values, which need 1.11.

## Authenticate

The provider calls the control plane with a Spotlight API token (`spt_...`), sent as a bearer token. Every request also names the provider in `Spotlight-Client: terraform-provider/<version>` and asks for API version `2026-09-23`.

```shell
export SPOTLIGHT_URL=https://spotlight.example.com
export SPOTLIGHT_API_TOKEN=spt_...
terraform plan
```

Or in configuration:

```hcl
provider "spotlight" {
  endpoint = "https://spotlight.example.com" # SPOTLIGHT_URL
  token    = var.spotlight_token             # SPOTLIGHT_API_TOKEN
}
```

## How it behaves

- **Drift.** Every refresh reads each resource whole. A change made elsewhere, in the web app or by another client, shows up as a planned update back to the configuration; a resource deleted elsewhere is planned to be created again.
- **Defaults.** Spotlight fills in what a definition leaves out (a station's colour, a review policy's skips, an integration's request method). The provider remembers what Spotlight answered after each apply, so the filled-in fields never show up as a diff; only a real change made since does.
- **Versions.** Each write sends the version Terraform last read as `If-Match`. A resource changed since is refused rather than overwritten: run `terraform plan` again to see the change.
- **Retries.** Creates carry an `Idempotency-Key`, so a create retried after a dropped connection makes one resource.
- **Errors name attributes.** A refused write is reported on the attribute it is about, as in `station[1].passes_to`.
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
export SPOTLIGHT_API_TOKEN=spt_...
cd examples/workspace
terraform plan
```

Remove the block, or point `TF_CLI_CONFIG_FILE` at a file without it, to go back to registry releases. To debug, run `go run . -debug` and export the `TF_REATTACH_PROVIDERS` it prints.

## Test

```shell
make test     # unit tests: the client against httptest, and every resource planned, applied, read, imported and drifted against an in-memory control plane
make check    # formatting, vet, build and unit tests
```

The resource tests run the real `terraform` binary, so it must be on `PATH` (or set `TF_ACC_TERRAFORM_PATH`). The write-only test is skipped below Terraform 1.11.

Acceptance tests run against a dev control plane. Each one creates a disposable workspace named `tf-acc-<random>`, configures it end to end, re-plans it expecting no change, imports every resource, and destroys it. Another checks that a misspelt expression fails the plan at its attribute.

```shell
export SPOTLIGHT_URL=https://dev.spotlight.example.com
export SPOTLIGHT_API_TOKEN=spt_...   # a user there who may create workspaces
make testacc
```

Without `TF_ACC`, `SPOTLIGHT_URL` and `SPOTLIGHT_API_TOKEN` they are skipped.

## Generate

The API client in `internal/client/apigen` is generated with [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) from `api/openapi.json`, a copy of the control plane's `control-plane/openapi.json`, restricted to the resource and expression operations (`api/oapi-codegen.yaml`). Both tools are pinned in `go.mod` and run with `go tool`.

```shell
make openapi SPOTLIGHT_REPO=../spotlight   # copy the schema from a control plane checkout
make client                                # regenerate the client
make docs                                  # regenerate docs/ from the schema, examples/ and templates/
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
| `SPOTLIGHT_ACC_TOKEN` | `acceptance.yml` | An `spt_` API token on it for a user who may create workspaces. |
| `GPG_PRIVATE_KEY` | `release.yml` | The armored private key that signs `SHA256SUMS`; its public key is registered with the registry namespace. |
| `GPG_PASSPHRASE` | `release.yml` | That key's passphrase. |

`GITHUB_TOKEN` is provided by Actions.
