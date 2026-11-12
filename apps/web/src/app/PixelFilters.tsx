// SVG filters that show an element as a mosaic of square blocks: each block
// takes the colour of one pixel in its middle. The front page folders step
// through them, from large blocks to sharp, as they appear.
export const PIXEL_SIZES = [24, 16, 10, 6, 3] as const;

export function PixelFilters() {
  return (
    <svg className="pixel-filters" width="0" height="0" aria-hidden="true" focusable="false">
      <defs>
        {PIXEL_SIZES.map((n) => (
          <filter key={n} id={`rv-px-${n}`} x="0" y="0" width="100%" height="100%" colorInterpolationFilters="sRGB">
            {/* One opaque pixel in the middle of an n×n tile, repeated. */}
            <feFlood x={Math.floor(n / 2)} y={Math.floor(n / 2)} width="1" height="1" />
            <feComposite width={n} height={n} />
            <feTile result="grid" />
            {/* Keep the source only at those pixels, then grow each into its block. */}
            <feComposite in="SourceGraphic" in2="grid" operator="in" />
            <feMorphology operator="dilate" radius={Math.floor(n / 2)} />
          </filter>
        ))}
      </defs>
    </svg>
  );
}
