variable "secret_linear_api_key" {
  description = "The value of the secret LINEAR_API_KEY, which Spotlight keeps and never answers"
  type        = string
  sensitive   = true
}

variable "secret_google_maps_key" {
  description = "The value of the secret GOOGLE_MAPS_KEY, which Spotlight keeps and never answers"
  type        = string
  sensitive   = true
}

# Spotlight keeps the value. The value is write-only. Terraform sends it when
# it creates the secret and when value_wo_version changes. The plan and the
# state never store it.
resource "spotlight_secret" "linear_api_key" {
  workspace_id = spotlight_workspace.acme.id
  name         = "LINEAR_API_KEY"
  bindings = [
    { host = "api.linear.app", prefix = "Bearer " },
  ]
  value_wo         = var.secret_linear_api_key
  value_wo_version = 1
}

# Spotlight puts this secret in the key query parameter. Use this for an API
# that takes its key in the URL.
resource "spotlight_secret" "google_maps_key" {
  workspace_id = spotlight_workspace.acme.id
  name         = "GOOGLE_MAPS_KEY"
  bindings = [
    { host = "maps.googleapis.com", carrier = "query", name = "key" },
  ]
  value_wo         = var.secret_google_maps_key
  value_wo_version = 1
}

# This secret has no value because Spotlight reads it from AWS Secrets Manager
# each time it uses it.
resource "spotlight_secret" "linear_signing" {
  workspace_id = spotlight_workspace.acme.id
  name         = "LINEAR_SIGNING"
  source = {
    kind      = "aws-secrets-manager"
    secret_id = "linear/signing"
    field     = "secret"
  }
}
