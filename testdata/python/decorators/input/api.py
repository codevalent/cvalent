def route(path: str):
    def decorator(func):
        return func
    return decorator

@route("/orders")
def get_orders() -> list:
    return []

class OrderService:
    def process(self, order_id: str) -> bool:
        return True

    @staticmethod
    def validate(data: dict) -> bool:
        return True
