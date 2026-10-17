locals {
  ephemeral = {
    key           = "ephemeral"
    token_version = 1
  }
}

resource "random_password" "ephemeral" {
  length  = 48
  special = false

  keepers = local.ephemeral
}

resource "spotlight_runner_pool" "ephemeral" {
  workspace_id  = spotlight_workspace.acme.id
  key           = local.ephemeral.key
  labels        = ["linux"]
  max_runs      = 8
  token         = random_password.ephemeral.result
  token_version = local.ephemeral.token_version
}

resource "random_password" "provision" {
  length  = 32
  special = false
}

resource "spotlight_runner_pool" "k8s_jobs" {
  workspace_id = spotlight_workspace.acme.id
  key          = "k8s-jobs"
  labels       = ["gpu"]

  provision = {
    url            = "https://provision.example.com/runs"
    secret         = "whsec_${base64encode(random_password.provision.result)}"
    secret_version = 1
    egress         = spotlight_egress.shared.id
  }
}
