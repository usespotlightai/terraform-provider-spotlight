variable "secret_linear_api_key" {
  description = "The value of the secret LINEAR_API_KEY, which Spotlight stores and never returns"
  type        = string
  sensitive   = true
}

variable "secret_google_maps_key" {
  description = "The value of the secret GOOGLE_MAPS_KEY, which Spotlight stores and never returns"
  type        = string
  sensitive   = true
}

# Spotlight stores the value. The value is write-only. Terraform sends it when
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
    { host = "maps.googleapis.com", location = "query", name = "key" },
  ]
  value_wo         = var.secret_google_maps_key
  value_wo_version = 1
}

# Terraform does not set this value. A person sets it on the Secrets settings
# page or with spotlight secret set LINEAR_SIGNING. Until then, a run that
# needs the secret does not start.
resource "spotlight_secret" "linear_signing" {
  workspace_id = spotlight_workspace.acme.id
  name         = "LINEAR_SIGNING"
}

# Terraform reads this value from AWS Secrets Manager. The value never enters
# the plan or the state. Change value_wo_version after you rotate the value in
# AWS, so Terraform sends the new value.
ephemeral "aws_secretsmanager_secret_version" "sentry_auth_token" {
  secret_id = "sentry/auth-token"
}

resource "spotlight_secret" "sentry_auth_token" {
  workspace_id = spotlight_workspace.acme.id
  name         = "SENTRY_AUTH_TOKEN"
  bindings = [
    { host = "sentry.io", prefix = "Bearer " },
  ]
  value_wo         = ephemeral.aws_secretsmanager_secret_version.sentry_auth_token.secret_string
  value_wo_version = 1
}

# A harness profile can give this value to its runs as an environment
# variable, through its env. The agent and every process in the sandbox can
# read it, so use this only for a value made for the sandbox, such as a
# development key.
resource "spotlight_secret" "dev_github_app_key" {
  workspace_id = spotlight_workspace.acme.id
  name         = "DEV_GITHUB_APP_KEY"
  sandbox      = "env"
}
