resource "spotlight_harness_profile" "core_careful" {
  team_id = spotlight_team.core.id
  key     = "careful"
  permissions = {
    allow = ["read", "edit", "execute(npm test *)"]
    ask   = ["execute(git push *)"]
    deny  = ["read(.env)"]
  }
  mcp_servers = [
    {
      name    = "linear"
      command = "npx"
      args    = ["-y", "linear-mcp"]
      env     = { LINEAR_WORKSPACE = "acme" }
    },
  ]
  native = {
    claude-code = {
      includeCoAuthoredBy = false
    }
    codex = "model_verbosity = \"low\"\n"
  }
  secrets = [spotlight_secret.staging_viewer.name]
  env     = { GITHUB_APP_PRIVATE_KEY = spotlight_secret.dev_github_app_key.name }
}
