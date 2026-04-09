"""Core functions for the synthetic parity fixture."""

from typing import Optional, List


def add(a: int, b: int) -> int:
    return a + b


def subtract(a: int, b: int) -> int:
    return a - b


def greet(name: str) -> str:
    return f"Hello, {name}"


def is_positive(n: int) -> bool:
    return n > 0


def clamp(v: int, lo: int, hi: int) -> int:
    if v < lo:
        return lo
    if v > hi:
        return hi
    return v


def find_first(items: List[str], prefix: str) -> Optional[str]:
    for item in items:
        if item.startswith(prefix):
            return item
    return None


class Widget:
    def __init__(self, name: str, value: int):
        self.name = name
        self.value = value

    def validate(self) -> bool:
        return bool(self.name)

    def format(self, template: str) -> str:
        return template.format(name=self.name, value=self.value)

    def process(self):
        if self.validate():
            return self.format("{name}={value}")
        return None

    def _internal_helper(self):
        pass

    def __repr__(self) -> str:
        return f"Widget({self.name!r}, {self.value})"


class Service:
    def __init__(self, widget: Widget):
        self.widget = widget

    def run(self) -> str:
        if self.widget.validate():
            return self.widget.format("{name}: {value}")
        return "invalid"

    def compute(self, x: int) -> int:
        return add(x, self.widget.value)


def process_all():
    w = Widget("test", 42)
    s = Service(w)
    s.run()
    total = add(1, 2)
    greet(f"user {total}")
