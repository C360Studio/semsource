export interface LayoutPoint {
  handle: string;
  x: number;
  y: number;
}

function hash(value: string, seed: number): number {
  let result = seed >>> 0;
  for (let index = 0; index < value.length; index += 1) {
    result ^= value.charCodeAt(index);
    result = Math.imul(result, 16777619);
  }
  return result >>> 0;
}

export function initialPosition(handle: string): { x: number; y: number } {
  const unit = (value: number) => value / 0xffffffff;
  return {
    x: Math.round((unit(hash(handle, 2166136261)) * 2 - 1) * 10000) / 10000,
    y: Math.round((unit(hash(handle, 2246822519)) * 2 - 1) * 10000) / 10000,
  };
}

export function computeLayout(
  handles: string[],
  width: number,
  height: number,
): LayoutPoint[] {
  const ordered = [...new Set(handles)].sort((left, right) =>
    left.localeCompare(right),
  );
  if (ordered.length === 0) return [];
  return ordered.map((handle) => {
    const position = initialPosition(handle);
    return {
      handle,
      x: Math.round(((position.x + 1) / 2) * width * 100) / 100,
      y: Math.round(((position.y + 1) / 2) * height * 100) / 100,
    };
  });
}
