---
page_title: "GitHub Actions without a stored token"
subcategory: ""
description: |-
  Run terraform plan and apply from GitHub Actions with no Spotlight token stored anywhere: each job exchanges the OIDC token GitHub issues it for a short-lived one, through a workspace token trust.
---

# GitHub Actions without a stored token

A pipeline that keeps a Spotlight API token as a repository secret has a credential that works from anywhere until someone rotates it. A `spotlight_workspace_token_trust` removes it. The trust names an OIDC issuer and the workloads it admits, and each job exchanges the OIDC token its CI platform issues it for a workspace token with the trust's scopes, lasting the trust's `lifetime_minutes` (15 by default, an hour at most). Nothing is stored in GitHub, and a token that leaks from a job's logs stops working within minutes.

The provider does the exchange itself: given a trust, it asks GitHub for an OIDC token, exchanges it, sends the token it gets back as `Authorization: Bearer`, and repeats the exchange a minute before that token expires, so an apply that outlives one token carries on with the next. Changing or deleting a trust stops every token it minted at once; a job whose token is refused that way exchanges again and carries on if the trust still admits it.

## 1. Define the trusts

This is a Terraform root of its own, run once by an admin as described below. Pull requests only plan, so they get a trust that mints `plan` tokens; pushes to `main` apply, so they get one that mints `configure` and `secrets` tokens. Add `runners` to the second if the configuration manages runner pools, runner trusts or the workspace's runners.

```terraform
data "spotlight_workspace" "acme" {
  slug = "acme"
}

resource "spotlight_workspace_token_trust" "github_pull_requests" {
  workspace_id = data.spotlight_workspace.acme.id
  key          = "github-pull-requests"
  issuer       = "https://token.actions.githubusercontent.com"
  subject      = "repo:acme/platform:pull_request"
  scopes       = ["plan"]
}

resource "spotlight_workspace_token_trust" "github_main" {
  workspace_id = data.spotlight_workspace.acme.id
  key          = "github-main"
  issuer       = "https://token.actions.githubusercontent.com"
  subject      = "repo:acme/platform:ref:refs/heads/main"
  scopes       = ["configure", "secrets"]
}

output "plan_trust" {
  value = spotlight_workspace_token_trust.github_pull_requests.id
}

output "apply_trust" {
  value = spotlight_workspace_token_trust.github_main.id
}
```

`subject` is matched against the `sub` claim of the job's OIDC token, which GitHub writes as `repo:<owner>/<repository>:pull_request` for a pull request, `repo:<owner>/<repository>:ref:refs/heads/<branch>` for a push, and `repo:<owner>/<repository>:environment:<name>` for a job that deploys to an environment. A job with `environment: production` could use `subject = "repo:acme/platform:environment:production"` instead, so that the environment's protection rules gate every apply. A `*` in `subject` stands for any run of characters, colons included, but a subject with one must name its owner before the first, so `repo:acme/*` is accepted and `repo:*` is refused. `claims` narrows the trust further by any other claim in the token, as in `claims = { repository_owner = "acme" }`.

### Bootstrap once, then store nothing

Creating, changing and deleting a trust needs a workspace admin, or a token granted the `trusts` scope that also has every scope the trust gives. A token can give a trust only the scopes that it has. A token can change a trust only when it has the current scopes and the new scopes of the trust. Any other token is refused with `403`. The trusts that let CI in cannot come from CI before they exist, so an admin applies them once:

1. Keep the trusts in a small Terraform root of their own, apart from the configuration CI applies.
2. Create an API token in the web app with `trusts` and every scope the trusts give, here `["configure", "secrets", "trusts"]` (`configure` includes `plan`), and the shortest expiry it offers. Add `runners` if a trust gives it.
3. Apply that root with it, as in `SPOTLIGHT_API_TOKEN=<token> terraform apply`, and note each trust's `id`, which `terraform output` shows.
4. Revoke the token.

From then on, CI runs with no stored token at all. Changing a trust later is the same: a short-lived `trusts` token, an apply of that root, and a revocation.

A pipeline that is to keep managing trusts itself needs a trust whose `scopes` include `trusts` and every scope the trusts it manages give. That is as powerful as it sounds: any job the trust admits can rewrite every trust in the workspace, its own included, and so mint itself tokens with any scope. Keep `trusts` off the trusts CI uses unless their subject admits nothing but a protected branch or environment.

## 2. Set the repository variables

Each trust's `id` names it when a job exchanges a token. It is not a secret, since only a job the trust admits can use it, so set it in the repository's **variables**, not its secrets:

| Variable | Value |
| --- | --- |
| `SPOTLIGHT_URL` | The control plane's URL, as in `https://spotlight.example.com` |
| `SPOTLIGHT_PLAN_TRUST` | The `id` of `github-pull-requests` |
| `SPOTLIGHT_APPLY_TRUST` | The `id` of `github-main` |

## 3. Run Terraform

The workflow grants its jobs `id-token: write`, which lets them request OIDC tokens and sets `ACTIONS_ID_TOKEN_REQUEST_URL` and `ACTIONS_ID_TOKEN_REQUEST_TOKEN`, where the provider finds them. Each job names its trust in `SPOTLIGHT_OIDC_TRUST`. There is no repository secret at all.

```yaml
name: terraform

on:
  pull_request:
  push:
    branches: [main]

permissions:
  contents: read
  id-token: write

env:
  SPOTLIGHT_URL: ${{ vars.SPOTLIGHT_URL }}

jobs:
  plan:
    if: github.event_name == 'pull_request'
    runs-on: ubuntu-latest
    env:
      SPOTLIGHT_OIDC_TRUST: ${{ vars.SPOTLIGHT_PLAN_TRUST }}
    steps:
      - uses: actions/checkout@v4
      - uses: hashicorp/setup-terraform@v3
      - run: terraform init -input=false
      - run: terraform plan -input=false

  apply:
    if: github.event_name == 'push'
    runs-on: ubuntu-latest
    env:
      SPOTLIGHT_OIDC_TRUST: ${{ vars.SPOTLIGHT_APPLY_TRUST }}
    steps:
      - uses: actions/checkout@v4
      - uses: hashicorp/setup-terraform@v3
      - run: terraform init -input=false
      - run: terraform apply -input=false -auto-approve
```

The provider block needs nothing: `provider "spotlight" {}` reads `SPOTLIGHT_URL` and `SPOTLIGHT_OIDC_TRUST`. The same can be written in it, which leaves only the trust in the configuration:

```terraform
provider "spotlight" {
  url = "https://spotlight.example.com"

  oidc {
    trust = "0d6f3c1e-8a52-4f7b-9c1d-5e2a7b3f9c40"
  }
}
```

A trust and a stored token are never used together: with `SPOTLIGHT_API_TOKEN` or `token` set as well, the provider stops and says so. The OIDC token is requested for the audience `spotlight`, the trust's default; a trust with another `audience` needs the same in `oidc.audience` or `SPOTLIGHT_OIDC_AUDIENCE`. Terraform's state backend authenticates on its own, and is not covered here.

## Other CI platforms

A platform that hands its jobs an OIDC token works the same way once the token is given to the provider in `SPOTLIGHT_OIDC_TOKEN`, or `oidc.token`. Define a trust with the platform's issuer and the audience its tokens carry, and set `SPOTLIGHT_OIDC_TRUST` to its `id`.

| Platform | Issuer | Setting `SPOTLIGHT_OIDC_TOKEN` |
| --- | --- | --- |
| GitLab CI/CD | `https://gitlab.com`, or your instance's URL | An `id_tokens` entry named `SPOTLIGHT_OIDC_TOKEN` with `aud: spotlight` |
| CircleCI | `https://oidc.circleci.com/org/<organization id>` | `export SPOTLIGHT_OIDC_TOKEN=$CIRCLE_OIDC_TOKEN_V2`; its audience is the organization's id |
| Buildkite | `https://agent.buildkite.com` | `export SPOTLIGHT_OIDC_TOKEN=$(buildkite-agent oidc request-token --audience spotlight)` |

In GitLab, for example, with a plan trust whose subject is `project_path:acme/platform:ref_type:branch:ref:*`:

```yaml
plan:
  image:
    name: hashicorp/terraform:latest
    entrypoint: [""]
  id_tokens:
    SPOTLIGHT_OIDC_TOKEN:
      aud: spotlight
  variables:
    SPOTLIGHT_OIDC_TRUST: $SPOTLIGHT_PLAN_TRUST
  script:
    - terraform init -input=false
    - terraform plan -input=false
```

A token handed over this way cannot be requested again: when the minted token is due for renewal, the provider exchanges the same OIDC token again, so it must outlive the run. In GitHub Actions the provider requests a fresh one for every exchange.
