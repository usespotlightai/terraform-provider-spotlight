# Terraform Provider for Spotlight

Manages a Spotlight workspace as code. Full reference: [`docs/`](docs/index.md).

## Resources

| Resource | Token scope | Import id |
| --- | --- | --- |
| `spotlight_workspace` | `configure` | `<slug>` |
| `spotlight_team` | `configure` | `<workspace id>/<key>` |
| `spotlight_line` | `configure` | `<workspace id>/<team key>/<key>` |
| `spotlight_skill` | `configure` | `<workspace id>/<team key>/<key>` |
| `spotlight_integration` | `configure` | `<workspace id>/<team key>/<source>` |
| `spotlight_harness_profile` | `configure` | `<workspace id>/<team key>/<key>` |
| `spotlight_hook` | `configure` | `<workspace id>/<team key>/<key>` |
| `spotlight_secret` | `secrets` | `<workspace id>/<name>` |
| `spotlight_runner_trust` | `runners` | `<workspace id>/<key>` |
| `spotlight_runner_pool` | `runners` | `<workspace id>/<key>` |
| `spotlight_egress` | `runners` | `<workspace id>/<key>` |
| `spotlight_workspace_runners` | `runners` | `<workspace id>/<slug>` |

- **Import by identity:** every resource can also be imported by its identity, `{ id = "<id>" }` (Terraform 1.12+).
- **List resources:** every resource is also a list resource, for `terraform query` (Terraform 1.14+).
- **Data sources:** `spotlight_workspace` (by `slug`) and `spotlight_team` (by `workspace_id` and `key`).

## Usage

```hcl
terraform {
  required_providers {
    spotlight = { source = "usespotlightai/spotlight" }
  }
}

provider "spotlight" {
  url   = "https://spotlight.example.com" # or SPOTLIGHT_URL
  token = var.spotlight_token             # or SPOTLIGHT_API_TOKEN
}

import {
  to = spotlight_workspace.acme
  id = "acme"
}

resource "spotlight_workspace" "acme" {
  name = "Acme"
  slug = "acme"
}
```

- **Workspaces:** a workspace is created by signing up in the web app, then adopted with an `import` block. A token cannot create one.
- **Scopes:** one token with `configure`, `secrets`, `runners` and `trusts` manages everything.
- **CI without a stored token:** `oidc { trust = "<trust id>" }`, or `SPOTLIGHT_OIDC_TRUST`, exchanges the CI job's OIDC token for a short-lived one through a `spotlight_workspace_token_trust`.
- **Adopting an existing workspace:** **Download as Terraform** in the web app's workspace menu, or `terraform query -generate-config-out=…`, writes the configuration and import blocks for you.
- **Terraform versions:** Terraform 1.11+ is needed for the write-only `value_wo` (secrets), `token` (runner pools and egresses), `provision.secret` (runner pools) and `signing.secret_wo` (hooks).

## Behaviour

- **Drift:** each refresh reads the whole resource, and changes made elsewhere are planned back.
- **Defaults:** fields Spotlight fills in never show as a diff.
- **Concurrency:** writes send `If-Match` with the version last read, so a resource changed since is refused, not overwritten.
- **Retries:** creates carry an `Idempotency-Key`, so a retried create makes one resource.
- **Errors:** a refused write is reported on the attribute it's about, and a `403` names the scope the token lacks.
- **Expressions:** `terraform plan` type-checks every CEL expression in an integration or a hook. Write them as `$${{ ... }}`, and a hook's `when` bare.
- **Tools:** name a tool through its integration, as in `spotlight_integration.grafana.tools["search"]`, so a tool renamed or removed fails the plan where it is named.

## Develop

```shell
make install    # build into $GOBIN
make check      # fmt, lint, vet, build, unit tests
make generate   # regenerate the API client and docs
```

To use a local build, add a `dev_overrides` block for `usespotlightai/spotlight` pointing at `$GOBIN` in `~/.terraformrc`. To debug, run `go run . -debug` and export the `TF_REATTACH_PROVIDERS` it prints.

The layout follows HashiCorp's [provider scaffold](https://github.com/hashicorp/terraform-provider-scaffolding-framework):

| Path | Holds |
| --- | --- |
| `internal/provider/` | The provider, plus `<name>_resource.go`, `<name>_data_source.go` and `<name>_list_resource.go` for each type, with tests beside them and sweepers in `sweep_test.go` |
| `internal/client/` | The API client; `apigen/` is generated from `api/openapi.json` |
| `internal/acctest/` | Test helpers: provider factories and `PreCheck` |
| `internal/fakeapi/` | An in-memory control plane for unit tests |
| `internal/expressions/`, `fieldpath/`, `dynamic/`, `writeonly/` | Expression checks, API paths mapped to attributes, dynamic values, write-only attributes |
| `examples/`, `templates/`, `docs/` | Registry docs; `docs/` is generated |

## Test

```shell
make test                  # unit tests against the in-memory control plane
make testacc               # acceptance tests against a dev control plane
make sweep SWEEP=<slug>    # delete what an interrupted acceptance run left behind
```

- **Terraform binary:** unit tests run the real `terraform`. Tests skip below the version they need: 1.11 for write-only values, 1.12 for identity, 1.14 for list resources.
- **Acceptance tests:** these need `SPOTLIGHT_URL`, `SPOTLIGHT_API_TOKEN` (with `configure`, `secrets`, `runners` and `trusts`) and `SPOTLIGHT_ACC_WORKSPACE`. They work under random `tf-acc-` keys and leave the workspace as they found it.
- **Sweepers:** these delete only `tf-acc-` / `TF_ACC_` resources. There's no sweeper for the workspace or its runner declaration.

## Generate

```shell
make openapi SPOTLIGHT_REPO=../spotlight   # copy the control plane's openapi.json
make client                                # regenerate internal/client/apigen
make docs                                  # regenerate docs/
```

CI fails when the committed client differs from one generated from the control plane's `develop`.

## Release

Push a `v*` tag. GoReleaser builds the archives, a GPG-signed `SHA256SUMS` and the registry manifest, and the Terraform Registry publishes the GitHub release.

| CI secret | Used by | Holds |
| --- | --- | --- |
| `APP_REPO_APP_ID` | `generate.yml` | The CI GitHub App that reads `usespotlightai/spotlight` (the one the infrastructure repo uses) |
| `APP_REPO_PRIVATE_KEY` | `generate.yml` | That app's private key |
| `SPOTLIGHT_ACC_URL` | `acceptance.yml` | The dev control plane |
| `SPOTLIGHT_ACC_TOKEN` | `acceptance.yml` | A token with `configure`, `secrets` and `runners` |
| `SPOTLIGHT_ACC_WORKSPACE` | `acceptance.yml` | That token's workspace slug |
| `GPG_PRIVATE_KEY` | `release.yml` | The signing key registered with the namespace |
| `GPG_PASSPHRASE` | `release.yml` | Its passphrase |

## License

[Apache License 2.0](LICENSE)
