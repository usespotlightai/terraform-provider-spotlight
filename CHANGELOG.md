# Changelog

## [0.1.0-rc.4](https://github.com/usespotlightai/terraform-provider-spotlight/compare/v0.1.0-rc.3...v0.1.0-rc.4) (2026-11-01)


### ⚠ BREAKING CHANGES

* call the control plane's versioned /api/v1 resource API ([#63](https://github.com/usespotlightai/terraform-provider-spotlight/issues/63))

### Features

* call the control plane's versioned /api/v1 resource API ([#63](https://github.com/usespotlightai/terraform-provider-spotlight/issues/63)) ([2b1e05c](https://github.com/usespotlightai/terraform-provider-spotlight/commit/2b1e05ce076b4999d0d9d1e6eabbdb210c6ba2d7))
* **SPO-232:** Add the search block to spotlight_line ([#60](https://github.com/usespotlightai/terraform-provider-spotlight/issues/60)) ([b4befbd](https://github.com/usespotlightai/terraform-provider-spotlight/commit/b4befbd1e80eb3744157528516b6f39aed9e65f2))


### Bug Fixes

* Accept a check and a setup of many lines, and refuse one longer than 65536 characters at plan ([#58](https://github.com/usespotlightai/terraform-provider-spotlight/issues/58)) ([c9c3ac8](https://github.com/usespotlightai/terraform-provider-spotlight/commit/c9c3ac8aa3131cdbfa9a10bffaae256b01cac2c3))

## [0.1.0-rc.3](https://github.com/usespotlightai/terraform-provider-spotlight/compare/v0.1.0-rc.2...v0.1.0-rc.3) (2026-10-22)


### ⚠ BREAKING CHANGES

* Rename spotlight_workspace_token_trust to spotlight_oidc_trust ([#55](https://github.com/usespotlightai/terraform-provider-spotlight/issues/55))

### Features

* Rename spotlight_workspace_token_trust to spotlight_oidc_trust ([#55](https://github.com/usespotlightai/terraform-provider-spotlight/issues/55)) ([c227dfd](https://github.com/usespotlightai/terraform-provider-spotlight/commit/c227dfd0a1813e329a80c48dc89f56efd0474bd4))
* Use the login from spotlight login when no token or OIDC trust is set ([#54](https://github.com/usespotlightai/terraform-provider-spotlight/issues/54)) ([b09d3a8](https://github.com/usespotlightai/terraform-provider-spotlight/commit/b09d3a89cb8c79a97142ef776737e9e50e93653a))

## [0.1.0-rc.2](https://github.com/usespotlightai/terraform-provider-spotlight/compare/v0.1.0-rc.1...v0.1.0-rc.2) (2026-10-19)


### Bug Fixes

* **SPO-212:** Bring the SPO-220 to SPO-230 vocabulary renames to main, and match the API's create-workspaces permission ([#51](https://github.com/usespotlightai/terraform-provider-spotlight/issues/51)) ([4e7fe9c](https://github.com/usespotlightai/terraform-provider-spotlight/commit/4e7fe9cc8dd6588f5b6aa6e3fa67c37d3469691d))
