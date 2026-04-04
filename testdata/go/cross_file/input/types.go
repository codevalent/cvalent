package main

type OrderRequest struct {
	ID     string
	Amount float64
}

type ProcessResult struct {
	Success bool
}
