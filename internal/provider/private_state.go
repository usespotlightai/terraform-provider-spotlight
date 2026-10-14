package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/usespotlightai/terraform-provider-spotlight/internal/dynamic"
)

const answerKey = "answer"

type privateState interface {
	GetKey(ctx context.Context, key string) ([]byte, diag.Diagnostics)
	SetKey(ctx context.Context, key string, value []byte) diag.Diagnostics
}

// rememberAnswer keeps a fingerprint of the API answer after a write. A
// later read uses it to find if something changed the resource after the
// write.
func rememberAnswer(ctx context.Context, private privateState, answer any) diag.Diagnostics {
	fingerprint, err := fingerprintOf(answer)
	if err != nil {
		var diags diag.Diagnostics
		diags.AddError("Could not remember the answer", err.Error())

		return diags
	}

	return private.SetKey(ctx, answerKey, fingerprint)
}

// answerUnchanged reports if the API answer is the same as the answer that
// rememberAnswer kept last. If it is, the model of each resource takes the
// configured attributes from the values that Terraform holds. If not, all
// values come from the answer, so that the plan removes a change made
// outside Terraform.
func answerUnchanged(ctx context.Context, private privateState, answer any) bool {
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
