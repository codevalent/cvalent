package main

type OrderRequest struct {
	ID     string
	Amount float64
	Items  []LineItem
}

type ProcessResult struct {
	Success bool
	OrderID string
}

func ProcessOrder(order OrderRequest, retry bool) (ProcessResult, error) {
	return ProcessResult{}, nil
}

func calculateTotal(items []LineItem) float64 {
	return 0
}
