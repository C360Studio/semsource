// Module-level block locals and namespace members that once shared IDs (#329).

if (globalThis.flag) { const cfg = 1; } else { const cfg = 2; }
for (const tmp of [1, 2]) { const step = tmp; }
try { const attempt = 1; } catch (e) { const attempt = 2; }
switch (globalThis.mode) { case 1: { const picked = 1; break; } default: { const picked = 2; } }
{ const scoped = 1; }

export namespace A { export const v = 1; export function g() { return 1; } }
export namespace B { export const v = 1; export function g() { return 2; } }
namespace Outer.Inner { export const v = 1; }
module Legacy { export function g() { return 3; } }

export const cfg = 3;
export const v = 4;
export function g() { return 5; }
