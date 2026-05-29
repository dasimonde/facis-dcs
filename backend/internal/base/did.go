package base

import (
	"crypto/rand"
	"digital-contracting-service/internal/base/datatype"
	"fmt"
	"os"
)

// UUID v4
func generateUUID() (string, error) {
	uuid := make([]byte, 16)
	_, err := rand.Read(uuid)
	if err != nil {
		return "", err
	}

	uuid[6] = (uuid[6] & 0x0f) | 0x40
	uuid[8] = (uuid[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:]), nil
}

func GetDID(resourceType datatype.ResourceType) (*string, error) {

	dcsIssuer := os.Getenv("DCS_ISSUER")

	uuid, err := generateUUID()
	if err != nil {
		return nil, err
	}

	did := fmt.Sprintf("did:web:%s:%s:%s", dcsIssuer, resourceType.String(), uuid)
	return &did, nil
}
