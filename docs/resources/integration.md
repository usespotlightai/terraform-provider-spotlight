---
page_title: "spotlight_integration Resource - spotlight"
subcategory: ""
description: |-
  An integration a team has created: how Spotlight receives events from a service, and the actions and tools it provides for that service. Spotlight stores the definition whole, exactly as the integration's definition is written, with its keys verbatim; write each expression as $${{ ... }} so Terraform leaves it alone. terraform plan type-checks every expression in it where it is written. A definition may name an API spec, openapi or graphql, that Spotlight generates a tool from for each operation when it reads it, or a remote MCP server, mcp, whose tools it lists; those tools are known only after apply. Import it as <workspace id>/<team key>/<integration>.
---

# spotlight_integration (Resource)

An integration a team has created: how Spotlight receives events from a service, and the actions and tools it provides for that service. Spotlight stores the definition whole, exactly as the integration's definition is written, with its keys verbatim; write each expression as `$${{ ... }}` so Terraform leaves it alone. `terraform plan` type-checks every expression in it where it is written. A definition may name an API spec, `openapi` or `graphql`, that Spotlight generates a tool from for each operation when it reads it, or a remote MCP server, `mcp`, whose tools it lists; those tools are known only after apply. Import it as `<workspace id>/<team key>/<integration>`.

## Example Usage

```terraform
resource "spotlight_integration" "core_linear" {
  team_id = spotlight_team.core.id
  definition = {
    integration = "linear"
    webhook = {
      verify = { hmac = { secret = "secret.LINEAR_SIGNING" } }
      events = [
        {
          # Terraform does not interpolate an expression in $${{ }}. The plan type-checks it.
          when = "$${{ body.type == \"Issue\" && body.action == \"update\" }}"
          raise = {
            happening = "linear.issue.completed"
            marks     = [{ kind = "linear.issue", value = "$${{ body.data.id }}" }]
          }
        }
      ]
    }
    actions = {
      comment = {
        request = {
          method  = "POST"
          url     = "https://api.linear.app/graphql"
          headers = { "Content-Type" = "application/json" }
          body    = { query = "$${{ inputs.body }}" }
        }
      }
    }
    tools = {
      comment = {
        action      = "comment"
        description = "Comment on the issue."
        inputs      = { body = { type = "string", required = true } }
      }
    }
  }
}
```

## Tools from an API spec

An integration can name an OpenAPI or GraphQL spec instead of writing each tool out. Spotlight generates a tool for each operation `tools.include` and `tools.exclude` leave in. It reads the spec when the integration is created, and again when its `url`, `document`, `baseUrl`, `revision`, `depth` or kind of `auth` (`oauth2` or `secrets`) changes, on any update after a failed read, and when the integration is connected over OAuth. Calling a tool never reads it. `tools.approval` asks a person before a tool runs, or with `false` lets it run without asking.

```terraform
resource "spotlight_integration" "core_petstore" {
  team_id = spotlight_team.core.id
  definition = {
    integration = "petstore"
    openapi = {
      url = "https://petstore3.swagger.io/api/v3/openapi.json"
      tools = {
        include = ["find-pets-*", "*-pet"]
        approval = {
          delete-pet = { because = "A deleted pet cannot be brought back." }
        }
      }
    }
  }
}
```

The generated tools are known only after apply, so `tools` reads `(known after apply)` in the plan. A generated tool named through `tools["…"]` that the spec does not provide fails the apply, not the plan. When a later read fails, the integration keeps the tools from the last good read: `spec.hash` stays the hash of that read, and `spec.problem` says why the latest one failed.

## Tools from a remote MCP server

An integration can name a remote MCP server, reached over streamable HTTP, in `mcp.url`. Spotlight lists its tools and provides each one as a tool of the integration, and calls them itself, so they get approvals, hooks and the run's journal. A tool the server marks `readOnlyHint` runs without asking; any other asks a person first unless `tools.approval` says `false`. Spotlight lists the tools again every ten minutes, so `tools` can change between applies. With `connection.oauth2.discover`, Spotlight finds where to sign in from what the server publishes, and registers itself there when the server allows it.

```terraform
resource "spotlight_integration" "core_linear_mcp" {
  team_id = spotlight_team.core.id
  definition = {
    integration = "linearmcp"
    name        = "Linear"
    connection = {
      oauth2 = { discover = true }
    }
    mcp = {
      url  = "https://mcp.linear.app/mcp"
      auth = { oauth2 = { connection = true } }
      tools = {
        exclude = ["delete-*"]
        approval = {
          create-comment = false
        }
      }
    }
  }
}
```

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `definition` (Dynamic) The integration's whole definition, as an object. Spotlight fills in each field it leaves out when it reads it back.
- `team_id` (String) The team the integration belongs to. Changing it replaces the integration.

### Read-Only

- `address` (String) Names the resource within its workspace, as in `core/linear`.
- `id` (String) Issued by Spotlight when the integration is created, and never changed.
- `integration` (String) The name of the integration, from `definition.integration`. It is immutable, so changing it replaces the integration.
- `spec` (Attributes) The API spec or MCP server the integration's tools are generated from, as Spotlight last read it; null when the definition names none. (see [below for nested schema](#nestedatt--spec))
- `tools` (Map of String) Each tool the integration provides, by its name in `definition.tools` or as generated from its API spec or MCP server, to the name that a station or hook uses for it, as in `{ search = "grafana.search" }`. Name a tool through it, as in `spotlight_integration.grafana.tools["search"]`, so a tool renamed or removed fails the plan where it is named. When the definition names an API spec or an MCP server the tools are known only after apply, so a generated tool named through it that is not there fails the apply, not the plan.
- `version` (Number) Rises with every change to the integration; each write sends it back so a change made since is not overwritten.

<a id="nestedatt--spec"></a>
### Nested Schema for `spec`

Read-Only:

- `hash` (String) The sha256 of the last good read, unchanged while a later read fails; null when the spec has never been read.
- `kind` (String) `openapi`, `graphql` or `mcp`.
- `problem` (String) Why the latest read failed; null when it succeeded. While it is set, the integration keeps the tools from the last good read. Spotlight reads the spec when the integration is created, and again when its `url`, `document`, `baseUrl`, `revision`, `depth` or kind of `auth` changes, on any update after a failed read, and when the integration is connected over OAuth. Spotlight lists an MCP server's tools again every ten minutes, or as often as its deployment sets. Calling a tool never reads it.
- `read_at` (String) When the spec was last read, in RFC 3339.
- `tools` (Number) How many of the integration's tools were generated from the spec's last good read.

## Import

Import is supported using the following syntax:

In Terraform v1.12.0 and later, the [`import` block](https://developer.hashicorp.com/terraform/language/import) can be used with the `identity` attribute, for example:

```terraform
import {
  to = spotlight_integration.core_linear
  identity = {
    id = "3f8a1c2e-6b4d-4e7a-9c1f-0d2b5e8a7c63"
  }
}
```

<!-- schema generated by tfplugindocs -->
### Identity Schema

#### Required

- `id` (String) The id Spotlight issued the integration when it was created.

In Terraform v1.5.0 and later, the [`import` block](https://developer.hashicorp.com/terraform/language/import) can be used with the `id` attribute, for example:

```terraform
import {
  to = spotlight_integration.core_linear
  id = "5b1e9d2c-7a43-4f8e-b6c0-3d9a8e1f2b57/core/linear"
}
```

The [`terraform import` command](https://developer.hashicorp.com/terraform/cli/commands/import) can be used, for example:

```shell
# Import an integration with the id of its workspace and its address, <team key>/<integration>.
terraform import spotlight_integration.core_linear 5b1e9d2c-7a43-4f8e-b6c0-3d9a8e1f2b57/core/linear
```
