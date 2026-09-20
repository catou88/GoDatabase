import { useEffect, useState } from "react";

// A small React Bits-style reveal kept local to avoid shipping an animation bundle.
export function AnimatedContent({ children, active = true, className = "" }) {
  const [visible, setVisible] = useState(false);
  useEffect(() => {
    setVisible(false);
    if (!active) return undefined;
    const frame = requestAnimationFrame(() => setVisible(true));
    return () => cancelAnimationFrame(frame);
  }, [active, children]);
  return <div className={`animated-content ${visible ? "is-visible" : ""} ${className}`}>{children}</div>;
}

export function AnimatedMetric({ label, value, unit, active }) {
  return <AnimatedContent active={active} className="metric-wrap"><article><p>{label}</p><strong>{value}</strong><small>{unit}</small></article></AnimatedContent>;
}
