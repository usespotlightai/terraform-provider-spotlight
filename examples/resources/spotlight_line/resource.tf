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

    judge {
      rubric = file("${path.module}/judges/core/delivery/implementation.md")
      claims = [
        "The change touches only the files the task names.",
        "Every new function has a test.",
      ]
      pass_share = 0.5
      sample     = 0.2
      trust      = 0.6
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
    most_regressions = 0
    ceiling          = 10
    currency         = "USD"
    min_gain         = 0.05
  }

  source {
    repository = "acme/platform"
    path       = "teams/core/delivery.tf"
  }
}
