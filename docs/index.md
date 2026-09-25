---
page_title: "Spotlight Provider"
description: |-
  Configure a Spotlight workspace as code: its teams, lines, skills, integrations and harness profiles, its secrets, and its runner pools and trusts.
---

# Spotlight Provider

Configure a [Spotlight](https://github.com/usespotlightai) workspace as code: its teams, the lines their tasks move along, the skills and harness profiles their agents are given, the integrations that connect them to other tools, the secrets those integrations wear, and the runner pools and trusts the work runs through.

**Download as Terraform**, in the web app's workspace menu, gives you a workspace you already have as configuration for this provider: a zipped module with an `import` block for each resource, and each prompt and skill as its own file. A first `terraform apply` adopts what is there and plans no change.

## Authentication

The provider calls the Spotlight control plane with a workspace token, sent as `Authorization: Bearer <token>`, which it gets in one of two ways.

**A stored API token.** Create one in the web app and give it to the provider through the environment:

```shell
export SPOTLIGHT_URL=https://spotlight.example.com
export SPOTLIGHT_API_TOKEN=<token>
```

or through the provider block, as below. Keep the token out of version control.

**From CI, with no stored token.** A `spotlight_workspace_token_trust` admits the OIDC tokens a CI platform issues its jobs, and the provider exchanges the job's token for a short-lived one with the trust's scopes, exchanging again before it expires. In GitHub Actions, a job granted `permissions: id-token: write` needs only the trust's id:

```shell
export SPOTLIGHT_URL=https://spotlight.example.com
export SPOTLIGHT_OIDC_TRUST=<trust id>
```

or `oidc { trust = "<trust id>" }` in the provider block. On other platforms, give the job's OIDC token in `SPOTLIGHT_OIDC_TOKEN` too. [GitHub Actions without a stored token](guides/github-actions-without-a-stored-token) sets up a pipeline that plans pull requests and applies `main` this way. A stored token and a trust are never used together.

A token, stored or minted, acts only inside the workspace it was created in, and only as its scopes allow. `read`, `plan` and `configure` are a ladder, each allowing what the one before it does: `read` refreshes and imports, `plan` also checks expressions, and `configure` also creates, updates and deletes configuration. Three grants go beside them:

| Scope | Lets the token write |
| --- | --- |
| `configure` | the workspace, teams, lines, skills, integrations and harness profiles |
| `secrets` | `spotlight_secret` |
| `runners` | `spotlight_runner_pool`, `spotlight_runner_trust` and `spotlight_workspace_runners` |
| `trusts` | `spotlight_workspace_token_trust` |

**One token granted `configure`, `secrets`, `runners` and `trusts` manages everything this provider can.** `trusts` decides which CI jobs get tokens and with which scopes, so it is as powerful as all the others together: grant it only to a short-lived token that sets up the trusts, as the guide below does. A token may give a trust only scopes it holds itself. A write the token's scopes do not allow is refused with `403`, and the provider's diagnostic names the scope the token lacks.

## Workspaces are adopted, not created

A workspace is created by a person signing up in the web app. Terraform adopts it with an `import` block that names its slug, then manages everything inside it:

```terraform
import {
  to = spotlight_workspace.acme
  id = "acme"
}

resource "spotlight_workspace" "acme" {
  name = "Acme"
  slug = "acme"
}
```

A token may not open a workspace, so creating a `spotlight_workspace` with one fails and says to import it instead.

## Reading and listing

The `spotlight_workspace` and `spotlight_team` data sources read a workspace by its slug and a team by its key, for configuration that works inside them without managing them.

Every resource is also a list resource. In Terraform 1.14 and later, a `list` block in a `.tfquery.hcl` file lists what a workspace holds, and `terraform query -generate-config-out=generated.tf` writes a resource and an `import` block for each, which is the same adoption the web app's download offers, with prompts and skills written inline rather than as files. Every resource has a resource identity, its `id`, so each can also be imported with an `import` block that names its `identity` (Terraform 1.12 and later).

## Where work runs

Every run gets a fresh runner of its own that ends with it, and only a runner pool or a runner trust lets a machine take work. A `spotlight_runner_pool` without `provision` is pulled from by `spotlight-host`, installed on any machine with the pool's token:

```shell
curl -fsSL https://spotlight.example.com/api/install.sh | SPOTLIGHT_URL=https://spotlight.example.com SPOTLIGHT_TOKEN=<pool token> sh
```

The host claims each run with that token and starts a fresh sandbox for it. A host on a platform that issues its workloads tokens, as Kubernetes does, claims through a `spotlight_runner_trust` instead, holding no Spotlight token: set `SPOTLIGHT_TRUST` to the trust's `id` and `SPOTLIGHT_OIDC_TOKEN_FILE` to the path of the projected token. A pool with `provision` is pushed to instead: Spotlight calls its URL for each run, handing a one-run runner its credential. A host is live state, shown on the web app's Runners page, and never a Terraform resource. A line or station names in `runners` the labels of a pool or trust, or a `<harness>@<version>` release, to place its work.

## Plans check expressions

Integrations hold CEL expressions, written `$${{ ... }}` in HCL so Terraform leaves them alone. `terraform plan` sends each one to the control plane to be type-checked where it is written, so a misspelt field fails the plan with the attribute that holds it named, rather than failing the apply.

## Values Terraform never keeps

A secret's value (`value_wo`), a pull pool's token (`token`) and a push pool's signing secret (`provision.secret`) are write-only attributes: they are sent to Spotlight and never stored in the plan or the state, so they need Terraform 1.11 or later. Terraform cannot see a change to a value it never keeps, so each has a version beside it (`value_wo_version`, `token_version`, `provision.secret_version`): change it to send the value again. A pull pool created without a token is issued one by Spotlight, answered only once; the provider does not keep it, and warns that no host can claim with it until it is given a token you hold. A pool token is given once, ever: rotating it, or deleting or replacing its pool, spends it for good, so a new `token_version` or a replaced pool needs a fresh value, such as a new `random_password`.

## Example Usage

```terraform
terraform {
  required_providers {
    spotlight = {
      source = "usespotlightai/spotlight"
    }
  }
}

# The url and token default to SPOTLIGHT_URL and SPOTLIGHT_API_TOKEN.
provider "spotlight" {
  url   = "https://spotlight.example.com"
  token = var.spotlight_token
}

variable "spotlight_token" {
  type      = string
  sensitive = true
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Optional

- `oidc` (Block, Optional) Authenticate from CI with no stored token: the OIDC token the CI platform issues the job is exchanged, through a `spotlight_workspace_token_trust`, for a short-lived token, renewed before it expires. In GitHub Actions, a job granted `permissions: id-token: write` needs only `trust`; elsewhere, give the platform's OIDC token as `token`. (see [below for nested schema](#nestedblock--oidc))
- `token` (String, Sensitive) A Spotlight API token. Defaults to `SPOTLIGHT_API_TOKEN`. Leave it unset to authenticate through `oidc`.
- `url` (String) The control plane's URL, as in `https://spotlight.example.com`. Defaults to `SPOTLIGHT_URL`, which the `spotlight` CLI reads too.

<a id="nestedblock--oidc"></a>
### Nested Schema for `oidc`

Optional:

- `audience` (String) The audience to request the OIDC token for, which must be the trust's. Defaults to `SPOTLIGHT_OIDC_AUDIENCE`, then `spotlight`.
- `token` (String, Sensitive) An OIDC token the CI platform issued the job, as GitLab's `id_tokens`, CircleCI's `$CIRCLE_OIDC_TOKEN_V2` or `buildkite-agent oidc request-token` give one. Defaults to `SPOTLIGHT_OIDC_TOKEN`; in GitHub Actions, one is requested for each exchange.
- `trust` (String) The `id` of the workspace token trust to exchange through. Defaults to `SPOTLIGHT_OIDC_TRUST`.
