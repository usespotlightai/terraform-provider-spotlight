terraform {
  required_providers {
    spotlight = {
      source = "usespotlightai/spotlight"
    }
  }
}

# The endpoint and token default to SPOTLIGHT_URL and SPOTLIGHT_API_TOKEN.
provider "spotlight" {
  endpoint = "https://spotlight.example.com"
  token    = var.spotlight_token
}

variable "spotlight_token" {
  type      = string
  sensitive = true
}
