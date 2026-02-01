// Keyword color palette and utilities

export interface KeywordColor {
  bg: string;
  text: string;
  hex: string;
}

// Expanded color palette - distinct, readable colors
export const COLOR_PALETTE: KeywordColor[] = [
  { bg: 'bg-blue-100', text: 'text-blue-700', hex: '#3b82f6' },
  { bg: 'bg-green-100', text: 'text-green-700', hex: '#22c55e' },
  { bg: 'bg-purple-100', text: 'text-purple-700', hex: '#a855f7' },
  { bg: 'bg-orange-100', text: 'text-orange-700', hex: '#f97316' },
  { bg: 'bg-pink-100', text: 'text-pink-700', hex: '#ec4899' },
  { bg: 'bg-teal-100', text: 'text-teal-700', hex: '#14b8a6' },
  { bg: 'bg-indigo-100', text: 'text-indigo-700', hex: '#6366f1' },
  { bg: 'bg-yellow-100', text: 'text-yellow-700', hex: '#eab308' },
  { bg: 'bg-red-100', text: 'text-red-700', hex: '#ef4444' },
  { bg: 'bg-cyan-100', text: 'text-cyan-700', hex: '#06b6d4' },
  { bg: 'bg-emerald-100', text: 'text-emerald-700', hex: '#10b981' },
  { bg: 'bg-violet-100', text: 'text-violet-700', hex: '#8b5cf6' },
  { bg: 'bg-amber-100', text: 'text-amber-700', hex: '#f59e0b' },
  { bg: 'bg-rose-100', text: 'text-rose-700', hex: '#f43f5e' },
  { bg: 'bg-sky-100', text: 'text-sky-700', hex: '#0ea5e9' },
  { bg: 'bg-lime-100', text: 'text-lime-700', hex: '#84cc16' },
  { bg: 'bg-fuchsia-100', text: 'text-fuchsia-700', hex: '#d946ef' },
  { bg: 'bg-slate-100', text: 'text-slate-700', hex: '#64748b' },
  { bg: 'bg-stone-100', text: 'text-stone-700', hex: '#78716c' },
  { bg: 'bg-zinc-100', text: 'text-zinc-700', hex: '#71717a' },
];

export function getColorByIndex(index: number): KeywordColor {
  return COLOR_PALETTE[index % COLOR_PALETTE.length];
}

// Get next unused color index based on existing assignments
export function getNextColorIndex(usedIndices: number[]): number {
  const used = new Set(usedIndices);
  for (let i = 0; i < COLOR_PALETTE.length; i++) {
    if (!used.has(i)) return i;
  }
  return usedIndices.length % COLOR_PALETTE.length;
}
