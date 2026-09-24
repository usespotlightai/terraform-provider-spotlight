variable "runner_build_1_token" {
  description = "The token the runner build-1 calls with"
  type        = string
  sensitive   = true
}

resource "spotlight_runner" "build_1" {
  workspace_id  = spotlight_workspace.acme.id
  name          = "build-1"
  labels        = ["vpc-internal"]
  token         = var.runner_build_1_token
  token_version = 1
}
