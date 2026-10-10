resource "spotlight_integration" "core_linear_mcp" {
  team_id = spotlight_team.core.id
  definition = {
    integration = "linearmcp"
    name        = "Linear"
    connection = {
      oauth2 = { discover = true }
    }
    mcp = {
      url  = "https://mcp.linear.app/mcp"
      auth = { oauth2 = { connection = true } }
      tools = {
        exclude = ["delete-*"]
        approval = {
          create-comment = false
        }
      }
    }
  }
}
