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
    profile   = "careful"
    prompt    = file("${path.module}/prompts/core/delivery/implementation.md")
    check     = "npm test"
    tools     = ["linear.comment"]
    skills    = ["grafana"]
    runners   = ["gpu"]
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
