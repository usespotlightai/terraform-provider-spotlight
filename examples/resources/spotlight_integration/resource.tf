resource "spotlight_integration" "core_linear" {
  team_id = spotlight_team.core.id
  definition = {
    integration = "linear"
    webhook = {
      verify = { hmac = { secret = "secret.LINEAR_SIGNING" } }
      events = [
        {
          # Terraform does not interpolate an expression in $${{ }}. The plan type-checks it.
          when = "$${{ body.type == \"Issue\" && body.action == \"update\" }}"
          raise = {
            happening = "linear.issue.completed"
            marks     = [{ kind = "linear.issue", value = "$${{ body.data.id }}" }]
          }
        }
      ]
    }
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
        action      = "comment"
        description = "Comment on the issue."
        inputs      = { body = { type = "string", required = true } }
      }
    }
  }
}
