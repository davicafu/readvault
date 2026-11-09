import type { Metadata } from "next";
import { Literata, Schibsted_Grotesk } from "next/font/google";
import Link from "next/link";
import type { ReactNode } from "react";

import { brand } from "@/config/brand";
import "./globals.css";

const literata = Literata({ subsets: ["latin", "latin-ext"], variable: "--font-literata", display: "swap" });
const schibsted = Schibsted_Grotesk({ subsets: ["latin", "latin-ext"], variable: "--font-schibsted", display: "swap" });

export const metadata: Metadata = {
  title: { default: brand.name, template: `%s | ${brand.name}` },
  description: brand.description,
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en" className={`${literata.variable} ${schibsted.variable}`}>
      <body>
        <header className="masthead">
          <div className="masthead-inner">
            <Link href="/" className="wordmark">
              {brand.name}
            </Link>
            <nav aria-label="Main">
              <ul className="nav">
                <li>
                  <Link href="/">Library</Link>
                </li>
                <li>
                  <Link href="/collections">Collections</Link>
                </li>
                <li>
                  <Link href="/add">Add articles</Link>
                </li>
                <li>
                  <a href="/api/export" download>
                    Export JSON
                  </a>
                </li>
              </ul>
            </nav>
          </div>
        </header>
        <main className="page">{children}</main>
      </body>
    </html>
  );
}
