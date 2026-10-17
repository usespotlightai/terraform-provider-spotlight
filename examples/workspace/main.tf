terraform {
  required_version = ">= 1.11"

  required_providers {
    spotlight = {
      source = "usespotlightai/spotlight"
    }
  }
}

provider "spotlight" {}

import {
  to = spotlight_workspace.acme
  id = "acme"
}

resource "spotlight_workspace" "acme" {
  name = "Acme"
  slug = "acme"
}

variable "secret_linear_api_key" {
  description = "The value of the secret LINEAR_API_KEY, which Spotlight stores and never answers"
  type        = string
  sensitive   = true
}

variable "runner_pool_ephemeral_token" {
  description = "The token the runner pool ephemeral claims with, which a spotlight-host serving it has; left null, Spotlight issues one that Terraform does not store"
  type        = string
  sensitive   = true
  default     = null
}

resource "spotlight_secret" "linear_api_key" {
  workspace_id = spotlight_workspace.acme.id
  name         = "LINEAR_API_KEY"
  bindings = [
    { host = "api.linear.app", prefix = "Bearer " },
  ]
  value_wo         = var.secret_linear_api_key
  value_wo_version = 1
}

resource "spotlight_runner_pool" "ephemeral" {
  workspace_id  = spotlight_workspace.acme.id
  key           = "ephemeral"
  labels        = ["vpc-internal"]
  token         = var.runner_pool_ephemeral_token
  token_version = 1
}

resource "spotlight_runner_trust" "production_cluster" {
  workspace_id = spotlight_workspace.acme.id
  key          = "production-cluster"
  issuer       = "https://oidc.eks.eu-west-1.amazonaws.com/id/EXAMPLE"
  subject      = "system:serviceaccount:spotlight:runner"
  labels       = ["gpu"]
}

resource "spotlight_workspace_runners" "acme" {
  workspace_id = spotlight_workspace.acme.id
  trusts       = [spotlight_runner_trust.production_cluster.key]
  pools        = [spotlight_runner_pool.ephemeral.key]
}

resource "spotlight_team" "core" {
  workspace_id = spotlight_workspace.acme.id
  key          = "core"
  name         = "Core"
}

resource "spotlight_skill" "core_grafana" {
  team_id = spotlight_team.core.id
  key     = "grafana"
  files = {
    "SKILL.md" = file("${path.module}/skills/core/grafana/SKILL.md")
  }
}

resource "spotlight_integration" "core_linear" {
  team_id = spotlight_team.core.id
  definition = {
    integration = "linear"
    actions = {
      comment = {
        request = {
          method  = "POST"
          url     = "https://api.linear.app/graphql"
          headers = { "Content-Type" = "application/json" }
          body    = { query = "$${{ inputs.body }}" }
          auth    = { secrets = ["secret.${spotlight_secret.linear_api_key.name}"] }
        }
      }
    }
    tools = {
      comment = {
        action      = "comment"
        description = "Comment on the issue."
        inputs      = { body = { type = "string", required = true } }
      }
    }
  }
}

resource "spotlight_harness_profile" "core_careful" {
  team_id = spotlight_team.core.id
  key     = "careful"
  permissions = {
    allow = ["read", "edit", "execute(npm test *)"]
    ask   = ["execute(git push *)"]
    deny  = ["read(.env)"]
  }
}

resource "spotlight_line" "core_delivery" {
  team_id = spotlight_team.core.id
  key     = "delivery"
  name    = "Delivery"
  role    = "delivery"
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
    passes_to = ["implementation"]
  }

  station {
    key       = "implementation"
    name      = "Implementation"
    harness   = "claude-code"
    profile   = spotlight_harness_profile.core_careful.key
    prompt    = file("${path.module}/prompts/core/delivery/implementation.md")
    check     = "npm test"
    tools     = ["linear.comment"]
    skills    = [spotlight_skill.core_grafana.key]
    passes_to = ["code-review"]

    judge {
      rubric = file("${path.module}/judges/core/delivery/implementation.md")
      sample = 0.2
      trust  = 0.6
    }

    alert {
      measure  = "sent_back_rate"
      rises_by = 0.2
      over     = 20
    }

    alert {
      measure = "cost_per_run"
      above   = 5
      over    = 10
    }

    alert {
      measure  = "accepted_rate"
      falls_by = 0.1
      over     = 20
    }

    alert {
      measure = "merged_rate"
      below   = 0.4
      over    = 20
    }
  }

  station {
    key       = "code-review"
    name      = "Code review"
    harness   = "claude-code"
    prompt    = "Review the change."
    passes_to = ["implementation"]
    ends      = true
  }

  station {
    key  = "completed"
    name = "Completed"
    kind = "completed"
  }

  reaction {
    happening = "github.checks.failed"
    from      = "implementation"
    to        = "implementation"
  }

  replay_check {
    max_regressions = 0
    ceiling         = 10
    currency        = "USD"
    min_gain        = 0.05
  }

  source {
    repository = "acme/platform"
    path       = "teams/core/delivery.tf"
  }
}
