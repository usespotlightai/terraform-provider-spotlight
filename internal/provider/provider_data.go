package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/client"
	"github.com/usespotlightai/terraform-provider-spotlight/internal/expressions"
)

type providerData struct {
	client      *client.Client
	expressions *expressions.Checker
}

func providerDataOf(value any, diags *diag.Diagnostics) *providerData {
	if value == nil {
		return nil
	}

	data, ok := value.(*providerData)
	if !ok {
		diags.AddError("Unexpected provider data", fmt.Sprintf("Expected *providerData, got %T; this is a bug in the provider.", value))

		return nil
	}

	return data
}

func clientOf(value any, diags *diag.Diagnostics) *client.Client {
	if data := providerDataOf(value, diags); data != nil {
		return data.client
	}

	return nil
}
