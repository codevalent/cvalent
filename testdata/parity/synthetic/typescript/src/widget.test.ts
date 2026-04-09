import { add, Widget, processAll } from "./widget";

export function testAdd(): void {
  if (add(1, 2) !== 3) throw new Error("fail");
}

export function testValidate(): void {
  const w = new Widget("x", 1);
  if (!w.validate()) throw new Error("fail");
}

export function testProcessAll(): void {
  processAll();
}
