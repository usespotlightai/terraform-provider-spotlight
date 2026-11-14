variable "inference_api_key" {
  description = "The API key of the inference endpoint, which Spotlight stores and never returns"
  type        = string
  sensitive   = true
}

resource "spotlight_inference_endpoint" "acme" {
  workspace_id              = spotlight_workspace.acme.id
  base_url                  = "https://api.openai.com/v1"
  model                     = "gpt-5"
  supports_reasoning_effort = true
  api_key_wo                = var.inference_api_key
  api_key_wo_version        = 1
}
