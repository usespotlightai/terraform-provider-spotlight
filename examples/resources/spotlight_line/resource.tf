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
    skills    = ["grafana"]
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
