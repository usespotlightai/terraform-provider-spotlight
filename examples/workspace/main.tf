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
  description = "The value of the secret LINEAR_API_KEY, which Spotlight keeps and never answers"
  type        = string
  sensitive   = true
}

variable "runner_build_1_token" {
  description = "The token the runner build-1 calls with; left null, Spotlight issues one Terraform does not keep"
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

resource "spotlight_runner" "build_1" {
  workspace_id = spotlight_workspace.acme.id
  name         = "build-1"
  labels       = ["vpc-internal"]
  token        = var.runner_build_1_token
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
  runners      = [spotlight_runner.build_1.name]
  trusts       = [spotlight_runner_trust.production_cluster.key]
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
        does        = "comment"
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
  team_id      = spotlight_team.core.id
  key          = "delivery"
  name         = "Delivery"
  role         = "delivery"
  repositories = ["acme/api"]
  runners      = [spotlight_runner.build_1.name, "vpc-internal"]

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
}
