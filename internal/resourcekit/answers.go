package resourcekit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/dynamic"
)

const answerKey = "answer"

// PrivateState is the private state Terraform keeps for a resource.
type PrivateState interface {
	GetKey(ctx context.Context, key string) ([]byte, diag.Diagnostics)
	SetKey(ctx context.Context, key string, value []byte) diag.Diagnostics
}

// Remember keeps a fingerprint of what the API answered after a write, so a
// later read can tell whether anything changed it since.
func Remember(ctx context.Context, private PrivateState, answer any) diag.Diagnostics {
	fingerprint, err := fingerprintOf(answer)
	if err != nil {
		var diags diag.Diagnostics
		diags.AddError("Could not remember the answer", err.Error())

		return diags
	}

	return private.SetKey(ctx, answerKey, fingerprint)
}

// Unchanged reports whether the API answers what it answered when last
// remembered, so what Terraform holds can stand as written.
func Unchanged(ctx context.Context, private PrivateState, answer any) bool {
	held, diags := private.GetKey(ctx, answerKey)
	if diags.HasError() || len(held) == 0 {
		return false
	}

	fingerprint, err := fingerprintOf(answer)

	return err == nil && string(fingerprint) == string(held)
}

func fingerprintOf(answer any) ([]byte, error) {
	canonical, err := dynamic.Canonical(answer)
	if err != nil {
		return nil, err
	}

	sum := sha256.Sum256(canonical)

	return json.Marshal(hex.EncodeToString(sum[:]))
}
