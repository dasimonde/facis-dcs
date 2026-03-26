package selfdescription

import "time"

// TODO: replace with the actual verification method.
// BuildProof returns a proof template.
func BuildProof(_ map[string]interface{}, proofPurpose string) map[string]interface{} {
	now := time.Now().UTC()

	proof := map[string]interface{}{
		"type":               "JsonWebSignature2020",
		"created":            now.Format(time.RFC3339),
		"proofPurpose":       proofPurpose,
		"verificationMethod": "did:web:argo.asd-stack.eu#key-1",
		"jws":                "eyJhbGciOiJSUzI1NiIsImI2NCI6ZmFsc2UsImNyaXQiOlsiYjY0Il19..kTCYt5XsITJX1CxPCT8yAV-TVIw5WEuts01mqpQy7UJiN5mgREEMGlv50aqzpqh4Qq_PbChOMqsLfRoPsnsgxD-WUcX16dUOqV0G_zS245-kronKb78cPktb3rk-BuQy72IFLN25DYuNzVBAh4vGHSrQyHUGlcTwLtjPAnKb78",
	}
	if proofPurpose == "authentication" {
		proof["challenge"] = "1f44d55f-f161-4938-a659-f8026467f126"
		proof["domain"] = "4jt78h47fh47"
	}
	return proof
}
