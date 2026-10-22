resource "spotlight_oidc_trust" "github_main" {
  workspace_id = spotlight_workspace.acme.id
  key          = "github-main"
  issuer       = "https://token.actions.githubusercontent.com"
  subject      = "repo:acme/platform:ref:refs/heads/main"
  claims = {
    repository_owner = "acme"
  }
  scopes           = ["configure", "secrets"]
  lifetime_minutes = 15
}
