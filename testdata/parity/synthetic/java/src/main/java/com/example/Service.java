package com.example;

public class Service {
    private Widget widget;

    public Service(Widget w) {
        this.widget = w;
    }

    public String run() {
        if (widget.isValid()) {
            return widget.format("%s: %d");
        }
        return "invalid";
    }

    public static void main(String[] args) {
        Widget w = Widget.create("test");
        Service s = new Service(w);
        System.out.println(s.run());
    }

    public int compute(int x) { return Widget.add(x, widget.getValue()); }

    public int compute(int x, int y) { return Widget.add(x, y); }
}
