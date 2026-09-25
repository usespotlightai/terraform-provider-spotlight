terraform {
  required_providers {
    spotlight = {
      source = "usespotlightai/spotlight"
    }
  }
}

import {
  to = spotlight_workspace.acme
  id = "acme"
}

resource "spotlight_workspace" "acme" {
  name = "Acme"
  slug = "acme"
}

import {
  to = spotlight_secret.linear_api_key
  id = "00000000-0000-4000-8000-00000000a0a0/LINEAR_API_KEY"
}

resource "spotlight_secret" "linear_api_key" {
  workspace_id = spotlight_workspace.acme.id
  name         = "LINEAR_API_KEY"
  source = {
    kind = "built-in"
  }
  bindings = [
    {
      header = "authorization"
      host   = "api.linear.app"
      prefix = "Bearer "
    },
  ]
  value_wo         = var.secret_linear_api_key
  value_wo_version = 1
}

import {
  to = spotlight_secret.linear_signing
  id = "00000000-0000-4000-8000-00000000a0a0/LINEAR_SIGNING"
}

resource "spotlight_secret" "linear_signing" {
  workspace_id = spotlight_workspace.acme.id
  name         = "LINEAR_SIGNING"
  source = {
    field     = "secret"
    kind      = "aws-secrets-manager"
    secret_id = "linear/signing"
  }
  bindings = []
}

import {
  to = spotlight_runner_trust.production-cluster
  id = "00000000-0000-4000-8000-00000000a0a0/production-cluster"
}

resource "spotlight_runner_trust" "production-cluster" {
  workspace_id = spotlight_workspace.acme.id
  key          = "production-cluster"
  issuer       = "https://oidc.eks.eu-west-1.amazonaws.com/id/EXAMPLE"
  subject      = "system:serviceaccount:spotlight:runner"
  audience     = "spotlight"
  labels       = ["gpu"]
}

import {
  to = spotlight_runner_pool.burst
  id = "00000000-0000-4000-8000-00000000a0a0/burst"
}

resource "spotlight_runner_pool" "burst" {
  workspace_id  = spotlight_workspace.acme.id
  key           = "burst"
  labels        = ["vpc-internal"]
  max_runs      = 4
  token         = var.runner_pool_burst_token
  token_version = 2
}

import {
  to = spotlight_runner_pool.k8s-jobs
  id = "00000000-0000-4000-8000-00000000a0a0/k8s-jobs"
}

resource "spotlight_runner_pool" "k8s-jobs" {
  workspace_id = spotlight_workspace.acme.id
  key          = "k8s-jobs"
  labels       = []
  provision = {
    url            = "https://provisioner.acme.dev/runs"
    secret         = var.runner_pool_k8s-jobs_provision_secret
    secret_version = 1
  }
}

import {
  to = spotlight_workspace_runners.acme
  id = "00000000-0000-4000-8000-00000000a0a0/acme"
}

resource "spotlight_workspace_runners" "acme" {
  workspace_id = spotlight_workspace.acme.id
  trusts       = [spotlight_runner_trust.production-cluster.key]
  pools        = [spotlight_runner_pool.burst.key, spotlight_runner_pool.k8s-jobs.key]
}

import {
  to = spotlight_team.core
  id = "00000000-0000-4000-8000-00000000a0a0/core"
}

resource "spotlight_team" "core" {
  workspace_id = spotlight_workspace.acme.id
  key          = "core"
  name         = "Core"
}

import {
  to = spotlight_line.core_delivery
  id = "00000000-0000-4000-8000-00000000a0a0/core/delivery"
}

resource "spotlight_line" "core_delivery" {
  team_id = spotlight_team.core.id
  key     = "delivery"
  name    = "Delivery"
  role    = "review"
  runners = ["vpc-internal"]

  repository {
    name  = "acme/api"
    image = "ghcr.io/acme/api-runner:main"
    setup = "npm ci"
  }

  station {
    key       = "backlog"
    name      = "Backlog"
    kind      = "backlog"
    color     = "#a3a8b0"
    diffs     = false
    decisions = false
    tools     = []
    skills    = []
    passes_to = ["implementation"]
    ends      = false
  }

  station {
    key       = "implementation"
    name      = "Implementation"
    kind      = "work"
    color     = "#6d78d5"
    harness   = "claude-code"
    profile   = spotlight_harness_profile.core_careful.key
    prompt    = file("${path.module}/prompts/core/delivery/implementation.md")
    check     = "npm test"
    diffs     = true
    decisions = true
    tools     = ["${spotlight_integration.core_linear.source}.comment", "pull-request"]
    skills    = [spotlight_skill.core_grafana.key]
    runners   = ["gpu"]
    passes_to = ["code-review"]
    ends      = false
  }

  station {
    key       = "code-review"
    name      = "Code review"
    kind      = "work"
    color     = "#56b98d"
    harness   = "claude-code"
    diffs     = false
    decisions = false
    tools     = []
    skills    = []
    passes_to = ["implementation"]
    ends      = true
  }

  station {
    key       = "completed"
    name      = "Completed"
    kind      = "completed"
    color     = "#3fa46a"
    diffs     = false
    decisions = false
    tools     = []
    skills    = []
    passes_to = []
    ends      = false
  }

  reaction {
    happening = "github.checks.failed"
    from      = "implementation"
    to        = "implementation"
  }

  reviews {
    start   = "opened"
    mention = "@spotlight"

    skip {
      spotlight = true
      drafts    = true
      authors   = ["dependabot[bot]", "renovate[bot]"]
    }
  }
}

import {
  to = spotlight_skill.core_grafana
  id = "00000000-0000-4000-8000-00000000a0a0/core/grafana"
}

resource "spotlight_skill" "core_grafana" {
  team_id = spotlight_team.core.id
  key     = "grafana"
  files = {
    "SKILL.md"               = file("${path.module}/skills/core/grafana/SKILL.md")
    "queries/latency.promql" = file("${path.module}/skills/core/grafana/queries/latency.promql")
  }
}

import {
  to = spotlight_integration.core_linear
  id = "00000000-0000-4000-8000-00000000a0a0/core/linear"
}

resource "spotlight_integration" "core_linear" {
  team_id = spotlight_team.core.id
  definition = {
    actions = {
      comment = {
        request = {
          auth = {
            secrets = ["secret.${spotlight_secret.linear_api_key.name}"]
          }
          body = {
            query = "$${{ inputs.body }}"
          }
          headers = {
            Content-Type = "application/json"
          }
          method = "POST"
          url    = "https://api.linear.app/graphql"
        }
      }
    }
    integration = "linear"
    tools = {
      comment = {
        description = "Comment on the issue.\n"
        does        = "comment"
        inputs = {
          body = {
            required = true
            type     = "string"
          }
        }
      }
    }
  }
}

import {
  to = spotlight_harness_profile.core_careful
  id = "00000000-0000-4000-8000-00000000a0a0/core/careful"
}

resource "spotlight_harness_profile" "core_careful" {
  team_id = spotlight_team.core.id
  key     = "careful"
  permissions = {
    allow = ["read", "execute(npm test *)"]
    ask   = []
    deny  = ["read(.env)"]
  }
  mcp_servers = [
    {
      args    = ["-y", "linear-mcp"]
      command = "npx"
      env = {
        LINEAR_WORKSPACE = "acme"
      }
      name = "linear"
    },
  ]
  native = {
    claude-code = {
      includeCoAuthoredBy = false
    }
    codex = "model_verbosity = \"low\"\n"
  }
}
