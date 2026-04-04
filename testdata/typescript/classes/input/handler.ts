export class OrderHandler {
  private service: OrderService;

  constructor(service: OrderService) {
    this.service = service;
  }

  public async processOrder(id: string): Promise<Result> {
    return {} as Result;
  }

  private validate(order: Order): boolean {
    return true;
  }
}
