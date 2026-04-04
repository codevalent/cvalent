from models import OrderRequest

def process_order(order: OrderRequest) -> bool:
    return validate(order)
