variable "secret_linear_api_key" {
  description = "The value of the secret LINEAR_API_KEY, which Spotlight keeps and never answers"
  type        = string
  sensitive   = true
}

variable "runner_build-1_token" {
  description = "The token the runner build-1 calls with; left null, Spotlight issues one when it creates the runner"
  type        = string
  sensitive   = true
  default     = null
}

variable "runner_pool_burst_token" {
  description = "The token the runner pool burst claims with; left null, Spotlight issues one when it creates the pool"
  type        = string
  sensitive   = true
  default     = null
}

variable "runner_pool_k8s-jobs_provision_secret" {
  description = "The secret the runner pool k8s-jobs signs its provision calls with, as whsec_ and base64 bytes"
  type        = string
  sensitive   = true
}
