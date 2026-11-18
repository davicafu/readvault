import type { NextConfig } from "next";

const config: NextConfig = {
  poweredByHeader: false,
  // No Next.js badge in the corner while developing; build errors still show.
  devIndicators: false,
  async headers() {
    return [
      {
        source: "/:path*",
        headers: [
          { key: "X-Content-Type-Options", value: "nosniff" },
          { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
          { key: "X-Frame-Options", value: "DENY" },
        ],
      },
      // The article page shows a saved PDF in a frame: only this app's own
      // pages may frame it. Being last, it overrides DENY above.
      {
        source: "/api/articles/:id/pdf",
        headers: [{ key: "X-Frame-Options", value: "SAMEORIGIN" }],
      },
    ];
  },
};

export default config;
