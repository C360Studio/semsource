// Ambient and merged-namespace declarations beside module-level namesakes (#329).

declare global { interface Win { x: number } }
declare module "m" { export interface Mod {} }

namespace N { export function g() { return 1; } }
namespace N { export function f() { return g(); } }
namespace Outer { export interface Sh {} }
namespace Outer.Inner { export function use(s: Sh) { return s; } }

export function g() { return 2; }
export interface Mod {}
export interface Sh {}
export function useWin(w: Win): Win { return w; }
