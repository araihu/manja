package domain

import "testing"

func TestValidateOperationRequestTargetAcceptsFixedQuery(t *testing.T) {
	t.Parallel()
	fixed := []FixedQueryParameter{{Name: "action", Value: "change"}, {Name: "vmw-task", Value: "true"}}
	if err := ValidateOperationRequestTarget("/appliance/networking", "/appliance/networking?action=change&vmw-task=true", fixed); err != nil {
		t.Fatal(err)
	}

	reset, err := NewOperationDetailIDWithRequestTarget("vmware", "vcenter", "POST", "/appliance/networking", "/appliance/networking?action=reset", []FixedQueryParameter{{Name: "action", Value: "reset"}})
	if err != nil {
		t.Fatal(err)
	}
	change, err := NewOperationDetailIDWithRequestTarget("vmware", "vcenter", "POST", "/appliance/networking", "/appliance/networking?action=change", []FixedQueryParameter{{Name: "action", Value: "change"}})
	if err != nil {
		t.Fatal(err)
	}
	if reset == change {
		t.Fatal("fixed-query operations produced the same detail identity")
	}
}

func TestValidateOperationRequestTargetRejectsInconsistentFixedQuery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		target string
		fixed  []FixedQueryParameter
	}{
		{name: "missing structured query", target: "/appliance/networking?action=reset"},
		{name: "different value", target: "/appliance/networking?action=reset", fixed: []FixedQueryParameter{{Name: "action", Value: "change"}}},
		{name: "different path", target: "/appliance/services?action=reset", fixed: []FixedQueryParameter{{Name: "action", Value: "reset"}}},
		{name: "fragment", target: "/appliance/networking?action=reset#fragment", fixed: []FixedQueryParameter{{Name: "action", Value: "reset#fragment"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := ValidateOperationRequestTarget("/appliance/networking", test.target, test.fixed); err == nil {
				t.Fatal("invalid fixed query was accepted")
			}
		})
	}
}
