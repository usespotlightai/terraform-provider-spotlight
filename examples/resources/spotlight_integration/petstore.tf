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
