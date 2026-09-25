resource "spotlight_workspace_runners" "acme" {
  workspace_id = spotlight_workspace.acme.id
  trusts       = [spotlight_runner_trust.production_cluster.key]
  pools        = [spotlight_runner_pool.ephemeral.key]
}
