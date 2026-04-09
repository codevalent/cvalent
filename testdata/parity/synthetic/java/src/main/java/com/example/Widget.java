package com.example;

import java.util.List;

public class Widget {
    private String name;
    private int value;

    public Widget(String name, int value) {
        this.name = name;
        this.value = value;
    }

    public String getName() { return name; }
    public int getValue() { return value; }

    public void setValue(int v) { this.value = v; }

    // Overloaded methods for sigHash disambiguation
    public String format(String template) {
        return String.format(template, name, value);
    }

    public String format(String template, int precision) {
        return String.format(template, name, precision);
    }

    public String format(List<String> parts) {
        return String.join(", ", parts) + ": " + name;
    }

    public static Widget create(String name) {
        return new Widget(name, 0);
    }

    public static int add(int a, int b) { return a + b; }

    public boolean isValid() { return name != null && !name.isEmpty(); }

    public void process() {
        if (isValid()) {
            format("%s=%d");
        }
    }
}
