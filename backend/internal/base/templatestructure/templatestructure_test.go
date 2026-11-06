package templatestructure

import (
	"errors"
	"testing"
)

func TestCheckDependencyPointsUpTheHierarchy(t *testing.T) {
	for _, tc := range []struct {
		level, dependency Level
		ok                bool
	}{
		{SubAgreement, FrameAgreement, true},
		{Agreement, FrameAgreement, true},
		{Addendum, Agreement, true},
		{Addendum, SubAgreement, true},
		{Addendum, FrameAgreement, true},
		{FrameAgreement, FrameAgreement, false},
		{SubAgreement, Agreement, false},
		{Agreement, Agreement, false},
		{Agreement, Addendum, false},
		{Addendum, Addendum, false},
	} {
		err := CheckDependency(tc.level, tc.dependency)
		if (err == nil) != tc.ok || (err != nil && !errors.Is(err, ErrInvalid)) {
			t.Errorf("%s depends on %s: err = %v, want ok=%v", tc.level, tc.dependency, err, tc.ok)
		}
	}
}

func TestCheckParent(t *testing.T) {
	frame := &Template{DID: "frame-v2", Family: "frame", Level: FrameAgreement}
	agreement := &Template{DID: "agreement", Family: "agreement", Level: Agreement}
	addendum := &Template{DID: "addendum", Family: "addendum", Level: Addendum}
	for name, tc := range map[string]struct {
		child  Template
		parent *Template
		ok     bool
	}{
		"standalone agreement":                {Template{Level: Agreement}, nil, true},
		"agreement under an agreement":        {Template{Level: Agreement}, agreement, true},
		"frame without parent":                {Template{Level: FrameAgreement}, nil, true},
		"frame with a parent":                 {Template{Level: FrameAgreement}, frame, false},
		"sub-agreement without parent":        {Template{Level: SubAgreement}, nil, false},
		"sub-agreement under a frame":         {Template{Level: SubAgreement}, frame, true},
		"sub-agreement under an agreement":    {Template{Level: SubAgreement}, agreement, false},
		"addendum without parent":             {Template{Level: Addendum}, nil, false},
		"addendum under an agreement":         {Template{Level: Addendum}, agreement, true},
		"child of an addendum":                {Template{Level: Agreement}, addendum, false},
		"required template without parent":    {Template{Level: Agreement, Requires: []string{"frame"}}, nil, false},
		"parent from the required template":   {Template{Level: SubAgreement, Requires: []string{"frame"}}, frame, true},
		"parent from another template family": {Template{Level: Agreement, Requires: []string{"frame"}}, &Template{Family: "other", Level: FrameAgreement}, false},
	} {
		err := CheckParent(tc.child, tc.parent)
		if (err == nil) != tc.ok || (err != nil && !errors.Is(err, ErrInvalid)) {
			t.Errorf("%s: err = %v, want ok=%v", name, err, tc.ok)
		}
	}
}
