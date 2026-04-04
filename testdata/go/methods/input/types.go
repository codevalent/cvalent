package types

type OrderRequest struct {
	ID     string
	Amount float64
}

func (o OrderRequest) Validate() error {
	return nil
}

func (o *OrderRequest) SetAmount(amount float64) {
	o.Amount = amount
}

func NewOrderRequest(id string) *OrderRequest {
	return &OrderRequest{ID: id}
}
