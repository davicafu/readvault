// SVG filters that show an element as a scatter of tiny dots: a fine noise
// decides which pixels show, and each step lets more of them through. The
// front page folders step through them, from a few sparks to whole, as they
// appear. Every step uses the same noise, so dots that showed stay.
export const DUST_THRESHOLDS = [0.75, 0.725, 0.699, 0.674, 0.648, 0.623, 0.598, 0.572, 0.547, 0.522, 0.496, 0.471, 0.445, 0.42] as const;

export function PixelFilters() {
  return (
    <svg className="pixel-filters" width="0" height="0" aria-hidden="true" focusable="false">
      <defs>
        {DUST_THRESHOLDS.map((t, i) => (
          <filter key={t} id={`rv-dust-${i + 1}`} x="0" y="0" width="100%" height="100%" colorInterpolationFilters="sRGB">
            <feTurbulence type="fractalNoise" baseFrequency="0.85" numOctaves="1" seed="7" result="noise" />
            {/* Alpha = 40 * noise - 40 * threshold: a sharp cut, so a pixel either shows or not. */}
            <feColorMatrix in="noise" type="matrix" values={`0 0 0 0 0  0 0 0 0 0  0 0 0 0 0  40 0 0 0 ${-40 * t}`} result="dots" />
            <feComposite in="SourceGraphic" in2="dots" operator="in" />
          </filter>
        ))}
      </defs>
    </svg>
  );
}
