resource "spotlight_runner_trust" "production_cluster" {
  workspace_id = spotlight_workspace.acme.id
  key          = "production-cluster"
  issuer       = "https://oidc.eks.eu-west-1.amazonaws.com/id/EXAMPLE"
  subject      = "system:serviceaccount:spotlight:runner"
  labels       = ["gpu"]
}
