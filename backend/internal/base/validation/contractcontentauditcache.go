package validation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
)

const contractContentAuditCacheLimit = 512

type contractContentAuditCacheState struct {
	mu       sync.Mutex
	findings map[string][]PolicyFinding
	order    []string
}

var contractContentAuditCache = contractContentAuditCacheState{
	findings: make(map[string][]PolicyFinding),
}

// contractContentAuditCacheKey returns a key only for produced documents whose
// Semantic Hub shapes and validation profile are pinned. Drafts without an
// immutable semantic bundle must always be evaluated against the active hub.
func contractContentAuditCacheKey(contract map[string]any, policy ContractContentPolicy) string {
	refs, err := EffectiveShapeRefs(contract)
	if err != nil || len(refs) == 0 || pinnedHubProfileVersion(contract) <= 0 {
		return ""
	}
	raw, err := json.Marshal([]any{contract, policy})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func cachedContractContentAudit(key string) ([]PolicyFinding, bool) {
	if key == "" {
		return nil, false
	}
	contractContentAuditCache.mu.Lock()
	defer contractContentAuditCache.mu.Unlock()
	findings, ok := contractContentAuditCache.findings[key]
	return clonePolicyFindings(findings), ok
}

func cacheContractContentAudit(key string, findings []PolicyFinding) {
	if key == "" {
		return
	}
	contractContentAuditCache.mu.Lock()
	defer contractContentAuditCache.mu.Unlock()
	if _, exists := contractContentAuditCache.findings[key]; exists {
		contractContentAuditCache.findings[key] = clonePolicyFindings(findings)
		return
	}
	if len(contractContentAuditCache.order) >= contractContentAuditCacheLimit {
		oldest := contractContentAuditCache.order[0]
		contractContentAuditCache.order = contractContentAuditCache.order[1:]
		delete(contractContentAuditCache.findings, oldest)
	}
	contractContentAuditCache.order = append(contractContentAuditCache.order, key)
	contractContentAuditCache.findings[key] = clonePolicyFindings(findings)
}

func resetContractContentAuditCache() {
	contractContentAuditCache.mu.Lock()
	defer contractContentAuditCache.mu.Unlock()
	contractContentAuditCache.findings = make(map[string][]PolicyFinding)
	contractContentAuditCache.order = nil
}

func clonePolicyFindings(findings []PolicyFinding) []PolicyFinding {
	if findings == nil {
		return nil
	}
	cloned := make([]PolicyFinding, len(findings))
	copy(cloned, findings)
	for index := range cloned {
		cloned[index].ActualValue = deepCopyValue(findings[index].ActualValue)
		cloned[index].ExpectedValue = deepCopyValue(findings[index].ExpectedValue)
		if findings[index].ExpectedValues != nil {
			cloned[index].ExpectedValues = deepCopyValue(findings[index].ExpectedValues).([]any)
		}
	}
	return cloned
}
