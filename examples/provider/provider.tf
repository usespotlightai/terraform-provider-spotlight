terraform {
  required_providers {
    spotlight = {
      source = "usespotlightai/spotlight"
    }
  }
}

# If you do not set url and token, the provider uses SPOTLIGHT_URL and
# SPOTLIGHT_API_TOKEN. With no token, it uses the login that spotlight login
# keeps for url.
provider "spotlight" {
  url   = "https://spotlight.example.com"
  token = var.spotlight_token
}

variable "spotlight_token" {
  type      = string
  sensitive = true
}
