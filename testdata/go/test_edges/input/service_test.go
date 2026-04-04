package main

import "testing"

func TestProcessOrder(t *testing.T) {
	ProcessOrder(OrderRequest{ID: "test"})
}
