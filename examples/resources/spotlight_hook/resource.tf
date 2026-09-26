resource "random_password" "hook" {
  length  = 32
  special = false
}

# Whenever an agent searches Grafana, tell the team in Slack and keep an audit trail.
resource "spotlight_hook" "grafana_reads" {
  team_id = spotlight_team.core.id
  key     = "grafana-reads"
  on      = "tool.used"
  match = {
    # A tool renamed or removed in the integration fails the plan here.
    tools = [spotlight_integration.grafana.tools["search"]]
  }
  # when is bare CEL; plan type-checks it.
  when = "event.inputs.query != ''"
  steps = [
    {
      name = "notify"
      call = spotlight_integration.slack.tools["post-message"]
      with = {
        channel = "C024BE91L"
        # $${{ }} keeps the expression from Terraform; plan type-checks it.
        text = "Agent searched Grafana: $${{ event.inputs.query }}"
      }
    },
    {
      name = "audit"
      request = {
        url = aws_lambda_function_url.audit.function_url
      }
    },
  ]
  signing = {
    secret_wo         = "whsec_${base64encode(random_password.hook.result)}"
    secret_wo_version = 1
  }
}

# Hold back a pull request that touches the migrations until a DBA's endpoint lets it open.
resource "spotlight_hook" "migrations" {
  team_id = spotlight_team.core.id
  key     = "migrations-need-a-dba"
  on      = "pull-request.opening"
  match = {
    paths = ["db/migrations/**"]
  }
  vars = { migrations = "db/migrations/" }
  steps = [
    {
      name = "review"
      request = {
        url  = "https://dba.acme.dev/review"
        body = "$${{ {'pr': event.title, 'files': event.changed.filter(path, path.startsWith(vars.migrations))} }}"
      }
    },
  ]
  signing = {
    oidc = { audience = "https://dba.acme.dev" }
  }
  failure         = "block"
  timeout_seconds = 20
}
