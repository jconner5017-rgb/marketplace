// Small line icons, sized to sit next to 13–14px text.
type P = { size?: number; filled?: boolean };

export function HeartIcon({ size = 16, filled = false }: P) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" aria-hidden="true" fill={filled ? "currentColor" : "none"} stroke="currentColor" strokeWidth="2">
      <path d="M12 20.5s-7.5-4.6-9.3-9.1C1.4 8 3.6 4.5 7.1 4.5c2 0 3.4 1.1 4.9 2.9 1.5-1.8 2.9-2.9 4.9-2.9 3.5 0 5.7 3.5 4.4 6.9-1.8 4.5-9.3 9.1-9.3 9.1z" strokeLinejoin="round" />
    </svg>
  );
}

export function BellIcon({ size = 20 }: P) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M6 9a6 6 0 1 1 12 0c0 5 2 6.5 2 6.5H4S6 14 6 9z" />
      <path d="M10 19a2 2 0 0 0 4 0" />
    </svg>
  );
}

export function CartIcon({ size = 22 }: P) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" aria-hidden="true" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M2.5 3.5h2.6l2.3 11h10.9l2.2-7.6H6.3" />
      <circle cx="9.5" cy="19.5" r="1.5" />
      <circle cx="17" cy="19.5" r="1.5" />
    </svg>
  );
}

export function ChevronIcon() {
  return (
    <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true" className="transition-transform group-open:rotate-180">
      <path d="M1.5 3.5 5 7l3.5-3.5" stroke="currentColor" strokeWidth="1.6" fill="none" />
    </svg>
  );
}
