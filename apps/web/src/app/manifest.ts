import type { MetadataRoute } from "next";

import { brand } from "@/config/brand";

// The web app manifest, served at /manifest.webmanifest: it lets browsers
// install ReadVault as an app on a computer or phone. Installing needs HTTPS
// or localhost.
export default function manifest(): MetadataRoute.Manifest {
  return {
    id: "/",
    name: brand.name,
    short_name: brand.name,
    description: brand.description,
    start_url: "/",
    scope: "/",
    display: "standalone",
    // The light theme's --paper, so the splash screen matches the app.
    background_color: "#eef1f4",
    theme_color: "#eef1f4",
    icons: [
      { src: "/icon.svg", sizes: "any", type: "image/svg+xml", purpose: "any" },
      { src: "/icons/icon-192.png", sizes: "192x192", type: "image/png", purpose: "any" },
      { src: "/icons/icon-512.png", sizes: "512x512", type: "image/png", purpose: "any" },
      { src: "/icons/icon-maskable-512.png", sizes: "512x512", type: "image/png", purpose: "maskable" },
    ],
  };
}
