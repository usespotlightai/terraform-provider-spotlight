variable "secret_linear_api_key" {
  description = "The value of the secret LINEAR_API_KEY, which Spotlight keeps and never answers"
  type        = string
  sensitive   = true
}

# Spotlight keeps the value. It is write-only: sent when the secret is
# created and whenever value_wo_version changes, never stored in the plan or
# the state.
resource "spotlight_secret" "linear_api_key" {
  workspace_id = spotlight_workspace.acme.id
  name         = "LINEAR_API_KEY"
  bindings = [
    { host = "api.linear.app", prefix = "Bearer " },
  ]
  value_wo         = var.secret_linear_api_key
  value_wo_version = 1
}

# Read from AWS Secrets Manager whenever it is used, so it takes no value.
resource "spotlight_secret" "linear_signing" {
  workspace_id = spotlight_workspace.acme.id
  name         = "LINEAR_SIGNING"
  source = {
    kind      = "aws-secrets-manager"
    secret_id = "linear/signing"
    field     = "secret"
  }
}
