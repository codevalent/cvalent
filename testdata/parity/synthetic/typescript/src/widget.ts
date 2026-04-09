export function add(a: number, b: number): number {
  return a + b;
}

export function subtract(a: number, b: number): number {
  return a - b;
}

export function greet(name: string): string {
  return `Hello, ${name}`;
}

export function isPositive(n: number): boolean {
  return n > 0;
}

export function clamp(v: number, lo: number, hi: number): number {
  return Math.max(lo, Math.min(hi, v));
}

export const formatValue = (v: number): string => {
  return v.toFixed(2);
};

export class Widget {
  name: string;
  value: number;

  constructor(name: string, value: number) {
    this.name = name;
    this.value = value;
  }

  validate(): boolean {
    return this.name.length > 0;
  }

  // Overloaded methods (TS signature overloads are resolved at declaration)
  format(template: string): string {
    return template.replace("{name}", this.name);
  }

  process(): string | null {
    if (this.validate()) {
      return this.format("{name}");
    }
    return null;
  }
}

export class Service {
  private widget: Widget;

  constructor(widget: Widget) {
    this.widget = widget;
  }

  run(): string {
    if (this.widget.validate()) {
      return this.widget.format("{name}");
    }
    return "invalid";
  }

  compute(x: number): number {
    return add(x, this.widget.value);
  }
}

export function processAll(): void {
  const w = new Widget("test", 42);
  const s = new Service(w);
  s.run();
  add(1, 2);
  greet("user");
}
