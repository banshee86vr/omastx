/** The Omastx logo mark: single-stroke ammonite spiral in the beacon accent. */
export function AmmoniteMark({ size = 32 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 32 32"
      role="img"
      aria-label="Omastx"
      style={{ color: "var(--beacon)" }}
    >
      <path
        d="M16 15 a1 1 0 0 1 1 1 a2 2 0 0 1 -2 2 a3.5 3.5 0 0 1 -3.5 -3.5 a5.5 5.5 0 0 1 5.5 -5.5 a8 8 0 0 1 8 8 a11 11 0 0 1 -11 11 a13 13 0 0 1 -13 -13"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
      />
    </svg>
  );
}
