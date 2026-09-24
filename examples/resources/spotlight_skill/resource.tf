resource "spotlight_skill" "core_grafana" {
  team_id = spotlight_team.core.id
  key     = "grafana"
  files = {
    "SKILL.md"               = file("${path.module}/skills/core/grafana/SKILL.md")
    "queries/latency.promql" = file("${path.module}/skills/core/grafana/queries/latency.promql")
  }
}
