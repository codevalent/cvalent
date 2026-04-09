"""Tests for core.py — exercises test tagging."""

from example_pkg.core import add, Widget, process_all


def test_add():
    assert add(1, 2) == 3


def test_widget_validate():
    w = Widget("x", 1)
    assert w.validate()


def test_process_all():
    process_all()
