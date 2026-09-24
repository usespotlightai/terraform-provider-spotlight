package providertest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// Module is a Terraform module `spotlight export` wrote: each file's
// content by its path within the module.
type Module map[string]string

// ModuleIn reads the module in dir.
func ModuleIn(dir string) (Module, error) {
	module := Module{}

	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		within, err := filepath.Rel(dir, path)
		module[filepath.ToSlash(within)] = string(content)

		return err
	})

	return module, err
}

// WriteTo writes the module into dir.
func (m Module) WriteTo(dir string) error {
	for within, content := range m {
		path := filepath.Join(dir, filepath.FromSlash(within))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return err
		}
	}

	return nil
}

// Config joins the module's .tf files into one configuration, with each
// file() it reads resolved against dir, where the module was written,
// since the test harness copies no directories.
func (m Module) Config(dir string) string {
	var joined strings.Builder

	names := make([]string, 0, len(m))
	for within := range m {
		if !strings.Contains(within, "/") && strings.HasSuffix(within, ".tf") {
			names = append(names, within)
		}
	}
	slices.Sort(names)

	for _, name := range names {
		joined.WriteString(m[name])
		joined.WriteString("\n")
	}

	return strings.ReplaceAll(joined.String(), "${path.module}", filepath.ToSlash(dir))
}

var (
	declared = regexp.MustCompile(`(?m)^resource "([a-z_]+)" "([A-Za-z0-9_-]+)" \{`)
	imported = regexp.MustCompile(`(?m)^import \{\n  to = ([a-z_]+\.[A-Za-z0-9_-]+)\n  id = "([^"]*)"\n\}\n\n?`)
)

// Addresses lists the resources a configuration declares.
func Addresses(config string) []string {
	var out []string
	for _, found := range declared.FindAllStringSubmatch(config, -1) {
		out = append(out, found[1]+"."+found[2])
	}

	return out
}

// Imports maps each address a configuration imports to its import id.
func Imports(config string) map[string]string {
	out := map[string]string{}
	for _, found := range imported.FindAllStringSubmatch(config, -1) {
		out[found[1]] = found[2]
	}

	return out
}

// WithoutImports drops the import blocks of the addresses given, so those
// resources are created from scratch.
func WithoutImports(config string, addresses ...string) string {
	return imported.ReplaceAllStringFunc(config, func(block string) string {
		if slices.Contains(addresses, imported.FindStringSubmatch(block)[1]) {
			return ""
		}

		return block
	})
}

// Protected adds prevent_destroy to the resource at address, so that no
// destroy of the configuration, as a failed test runs, can delete it or
// anything else it holds.
func Protected(config, address string) string {
	kind, name, _ := strings.Cut(address, ".")
	opening := fmt.Sprintf("resource %q %q {\n", kind, name)

	start := strings.Index(config, opening)
	if start < 0 {
		return config
	}

	closing := strings.Index(config[start:], "\n}\n")
	if closing < 0 {
		return config
	}

	end := start + closing + 1

	return config[:end] + "\n  lifecycle {\n    prevent_destroy = true\n  }\n" + config[end:]
}

// Released is configuration that forgets every address without destroying
// what it names.
func Released(addresses []string) string {
	var out strings.Builder
	for _, address := range addresses {
		fmt.Fprintf(&out, "removed {\n  from = %s\n\n  lifecycle {\n    destroy = false\n  }\n}\n\n", address)
	}

	return out.String()
}

// Adopts is a plan check that the plan creates the addresses given and
// otherwise does nothing but import: nothing else is created, changed,
// replaced or destroyed.
type Adopts struct {
	Creating []string
}

// CheckPlan fails when any other resource is planned to change.
func (a Adopts) CheckPlan(_ context.Context, req plancheck.CheckPlanRequest, resp *plancheck.CheckPlanResponse) {
	var changed []string

	for _, each := range req.Plan.ResourceChanges {
		creating := slices.Contains(a.Creating, each.Address) && each.Change.Actions.Create()
		if !each.Change.Actions.NoOp() && !creating {
			changed = append(changed, fmt.Sprintf("%s is planned to %v", each.Address, each.Change.Actions))
		}
	}

	if len(changed) > 0 {
		resp.Error = fmt.Errorf("the export does not match what Spotlight holds:\n%s", strings.Join(changed, "\n"))
	}
}
