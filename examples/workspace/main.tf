terraform {
  required_version = ">= 1.11"

  required_providers {
    spotlight = {
      source = "usespotlightai/spotlight"
    }
  }
}

provider "spotlight" {}

resource "spotlight_workspace" "acme" {
  name = "Acme"
  slug = "acme"
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

resource "spotlight_line" "core_delivery" {
  team_id      = spotlight_team.core.id
  key          = "delivery"
  name         = "Delivery"
  role         = "delivery"
  repositories = ["acme/api"]

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
    prompt    = file("${path.module}/prompts/core/delivery/implementation.md")
    check     = "npm test"
    tools     = ["linear.comment"]
    skills    = [spotlight_skill.core_grafana.key]
    passes_to = ["completed"]
  }

  station {
    key  = "completed"
    name = "Completed"
    kind = "completed"
    ends = true
  }

  reaction {
    happening = "github.checks.failed"
    from      = "implementation"
    to        = "implementation"
  }
}
