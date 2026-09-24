resource "spotlight_workspace_runners" "acme" {
  workspace_id = spotlight_workspace.acme.id
  runners      = [spotlight_runner.build_1.name]
  trusts       = [spotlight_runner_trust.production_cluster.key]
}
