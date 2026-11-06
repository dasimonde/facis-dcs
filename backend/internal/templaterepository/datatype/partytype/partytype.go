package partytype

import "fmt"

type PartyType string

const (
	SingleParty PartyType = "SINGLE_PARTY"
	MultiParty  PartyType = "MULTI_PARTY"
)

func New(s string) (PartyType, error) {
	p := PartyType(s)
	if p != SingleParty && p != MultiParty {
		return "", fmt.Errorf("invalid party type %q", s)
	}
	return p, nil
}
func (p PartyType) String() string { return string(p) }
