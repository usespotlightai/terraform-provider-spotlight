locals {
  shared = {
    key           = "shared"
    token_version = 1
  }
}

resource "random_password" "egress" {
  length  = 48
  special = false

  keepers = local.shared
}

resource "spotlight_egress" "shared" {
  workspace_id  = spotlight_workspace.acme.id
  key           = local.shared.key
  url           = "https://egress.example.com"
  token         = random_password.egress.result
  token_version = local.shared.token_version
}
