package client_test

import (
	"testing"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/client/apigen"
)

func TestEachForbiddenPermissionIsOneTheAPISends(t *testing.T) {
	for _, permission := range []client.Permission{
		client.PermissionPlan,
		client.PermissionConfigure,
		client.PermissionSecrets,
		client.PermissionRunners,
		client.PermissionTrusts,
		client.PermissionAdminister,
		client.PermissionCreateWorkspaces,
	} {
		if !apigen.ProblemResponseDtoPermission(permission).Valid() {
			t.Errorf("the API never sends the permission %q", permission)
		}
	}
}
