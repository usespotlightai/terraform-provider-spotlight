data "spotlight_workspace" "acme" {
  slug = "acme"
}

resource "spotlight_oidc_trust" "github_pull_requests" {
  workspace_id = data.spotlight_workspace.acme.id
  key          = "github-pull-requests"
  issuer       = "https://token.actions.githubusercontent.com"
  subject      = "repo:acme/platform:pull_request"
  scopes       = ["plan"]
}

resource "spotlight_oidc_trust" "github_main" {
  workspace_id = data.spotlight_workspace.acme.id
  key          = "github-main"
  issuer       = "https://token.actions.githubusercontent.com"
  subject      = "repo:acme/platform:ref:refs/heads/main"
  scopes       = ["configure", "secrets"]
}

output "plan_trust" {
  value = spotlight_oidc_trust.github_pull_requests.id
}

output "apply_trust" {
  value = spotlight_oidc_trust.github_main.id
}
