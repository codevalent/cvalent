// Package widget provides synthetic parity functions for Go.
package widget

import "fmt"

// --- Free functions ---

func Add(a, b int) int { return a + b }

func Subtract(a, b int) int { return a - b }

func Multiply(a, b int) int { return a * b }

func Divide(a, b int) (int, error) {
	if b == 0 {
		return 0, fmt.Errorf("division by zero")
	}
	return a / b, nil
}

func Greet(name string) string { return "Hello, " + name }

func IsPositive(n int) bool { return n > 0 }

func Clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// --- Struct with value + pointer receivers ---

type OrderRequest struct {
	ID     string
	Amount float64
	Items  []string
}

func (o OrderRequest) Validate() error {
	if o.ID == "" {
		return fmt.Errorf("missing ID")
	}
	return nil
}

func (o OrderRequest) Total() float64 {
	return o.Amount
}

func (o *OrderRequest) SetAmount(a float64) {
	o.Amount = a
}

func (o *OrderRequest) AddItem(item string) {
	o.Items = append(o.Items, item)
}

// --- Generic type with methods ---

type Box[T any] struct {
	Value T
}

func (b Box[T]) Get() T { return b.Value }

func (b *Box[T]) Set(v T) { b.Value = v }

// Generic free function
func Map[T any, U any](items []T, fn func(T) U) []U {
	out := make([]U, len(items))
	for i, item := range items {
		out[i] = fn(item)
	}
	return out
}

// --- Cross-file caller ---

func ProcessAll() {
	req := OrderRequest{ID: "1", Amount: 100.0}
	_ = req.Validate()
	sum := Add(1, 2)
	_ = Greet(fmt.Sprintf("user %d", sum))
}
