package widget

import "testing"

func TestAdd(t *testing.T) {
	if Add(1, 2) != 3 {
		t.Fatal("1+2 != 3")
	}
}

func TestValidate(t *testing.T) {
	o := OrderRequest{ID: "1"}
	if err := o.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestProcessAll(t *testing.T) {
	ProcessAll() // just exercises the cross-file caller
}
