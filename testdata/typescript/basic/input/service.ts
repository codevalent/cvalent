interface OrderRequest {
  id: string;
  amount: number;
  items: LineItem[];
}

interface ProcessResult {
  success: boolean;
  orderId?: string;
}

export function processOrder(order: OrderRequest, retry: boolean = false): ProcessResult {
  return { success: true };
}

function calculateTotal(items: LineItem[]): number {
  return 0;
}
