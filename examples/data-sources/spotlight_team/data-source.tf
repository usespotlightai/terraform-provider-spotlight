data "spotlight_workspace" "acme" {
  slug = "acme"
}

data "spotlight_team" "core" {
  workspace_id = data.spotlight_workspace.acme.id
  key          = "core"
}

resource "spotlight_skill" "core_grafana" {
  team_id = data.spotlight_team.core.id
  key     = "grafana"
  files = {
    "SKILL.md" = file("${path.module}/skills/grafana/SKILL.md")
  }
}
