import type { Metadata, Viewport } from "next";
import { Literata, Schibsted_Grotesk } from "next/font/google";
import Link from "next/link";
import { Suspense, type ReactNode } from "react";

import { brand } from "@/config/brand";
import { NavIcon } from "./NavIcon";
import { Notifications } from "./Notifications";
import { HeaderSearch } from "./HeaderSearch";
import "./globals.css";

const literata = Literata({ subsets: ["latin", "latin-ext"], variable: "--font-literata", display: "swap" });
const schibsted = Schibsted_Grotesk({ subsets: ["latin", "latin-ext"], variable: "--font-schibsted", display: "swap" });

export const metadata: Metadata = {
  title: { default: brand.name, template: `%s | ${brand.name}` },
  description: brand.description,
  // Installed on an iPhone or iPad, the app opens full screen under its name.
  appleWebApp: { capable: true, title: brand.name, statusBarStyle: "default" },
};

// The browser bar and the installed app's title bar take the page's --paper.
export const viewport: Viewport = {
  themeColor: [
    { media: "(prefers-color-scheme: light)", color: "#eef1f4" },
    { media: "(prefers-color-scheme: dark)", color: "#151b23" },
  ],
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en" className={`${literata.variable} ${schibsted.variable}`} suppressHydrationWarning>
      <body>
        {/* Marks the page once its first frame is on screen, so entrances
            start when they can be seen rather than while the page is still
            loading (see .folder in globals.css). */}
        <script
          dangerouslySetInnerHTML={{
            __html: "requestAnimationFrame(function(){requestAnimationFrame(function(){document.documentElement.dataset.painted=''})})",
          }}
        />
        <div className="backdrop" aria-hidden="true">
          <span />
          <span />
          <span />
        </div>
        <header className="masthead">
          <div className="masthead-inner">
            <Link href="/" className="wordmark">
              {brand.name}
            </Link>
            <Suspense fallback={<div className="header-search" />}>
              <HeaderSearch />
            </Suspense>
            <nav aria-label="Main">
              <ul className="nav">
                <li>
                  <Link href="/articles">
                    <NavIcon name="library" />
                    Library
                  </Link>
                </li>
                <li>
                  <Link href="/collections">
                    <NavIcon name="collections" />
                    Collections
                  </Link>
                </li>
                <li>
                  <Link href="/add">
                    <NavIcon name="add" />
                    Add articles
                  </Link>
                </li>
                <li>
                  <a href="/api/export" download>
                    <NavIcon name="export" />
                    Export JSON
                  </a>
                </li>
              </ul>
            </nav>
            <Notifications />
          </div>
        </header>
        <main className="page">{children}</main>
      </body>
    </html>
  );
}
