resource "spotlight_workspace" "acme" {
  name           = "Acme"
  slug           = "acme"
  outbound_hosts = ["*.acme.io", "api.linear.app"]
}
