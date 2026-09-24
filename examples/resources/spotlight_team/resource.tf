resource "spotlight_team" "core" {
  workspace_id = spotlight_workspace.acme.id
  key          = "core"
  name         = "Core"
}
