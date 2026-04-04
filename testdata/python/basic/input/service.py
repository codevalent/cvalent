from typing import List, Optional

class OrderRequest:
    id: str
    amount: float
    items: List[str]

class ProcessResult:
    success: bool
    order_id: str

def process_order(order: OrderRequest, retry: bool = False) -> ProcessResult:
    return ProcessResult()

def _calculate_total(items: List[str]) -> float:
    return 0.0
