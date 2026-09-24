## 0.1.0 (Unreleased)

FEATURES:

* **New Resource:** `spotlight_workspace`
* **New Resource:** `spotlight_team`
* **New Resource:** `spotlight_line`
* **New Resource:** `spotlight_skill`
* **New Resource:** `spotlight_integration`
* **New Resource:** `spotlight_harness_profile`
* **New Resource:** `spotlight_secret`
* **New Resource:** `spotlight_runner`
* **New Resource:** `spotlight_runner_trust`
* **New Resource:** `spotlight_workspace_runners`
* **New Data Source:** `spotlight_workspace`
* **New Data Source:** `spotlight_team`
* **New List Resource:** `spotlight_workspace`
* **New List Resource:** `spotlight_team`
* **New List Resource:** `spotlight_line`
* **New List Resource:** `spotlight_skill`
* **New List Resource:** `spotlight_integration`
* **New List Resource:** `spotlight_harness_profile`
* **New List Resource:** `spotlight_secret`
* **New List Resource:** `spotlight_runner`
* **New List Resource:** `spotlight_runner_trust`
* **New List Resource:** `spotlight_workspace_runners`
* provider: every resource has a resource identity, `id`, and imports by it as well as by its `<workspace id>/<address>` import id
* resource/spotlight_integration: `terraform plan` type-checks each CEL expression in `definition` and reports it on its attribute
* resource/spotlight_secret, resource/spotlight_runner: `value_wo` and `token` are write-only and never stored in the plan or the state
