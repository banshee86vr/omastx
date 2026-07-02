import styles from "./Meter.module.css";

interface MeterProps {
  label: string;
  value: number;
  max: number;
}

export function Meter({ label, value, max }: MeterProps) {
  const pct = max > 0 ? Math.min(100, (value / max) * 100) : 0;
  return (
    <div
      className={styles.meter}
      role="meter"
      aria-valuenow={value}
      aria-valuemin={0}
      aria-valuemax={max}
      aria-label={label}
    >
      <div className={styles.labels}>
        <span>{label}</span>
        <span>
          {value}/{max}
        </span>
      </div>
      <div className={styles.track}>
        <div className={styles.fill} style={{ width: `${pct}%` }} />
      </div>
    </div>
  );
}
