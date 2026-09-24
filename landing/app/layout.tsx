import type { Metadata, Viewport } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import { MotionConfig } from "framer-motion";
import { SmoothScroll } from "@/components/landing/SmoothScroll";
import { links, siteUrl } from "@/lib/content";
import "./globals.css";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

const TITLE = "Duster: the Windows cleaner built for the terminal";
const DESCRIPTION =
  "Duster is a single-binary Windows CLI that cleans caches, purges dev artifacts, and shows what is eating your disk. Every destructive command previews first.";

// The social image comes from app/opengraph-image.tsx; twitter inherits it (and title/description) from openGraph.

export const metadata: Metadata = {
  metadataBase: siteUrl,
  title: {
    default: TITLE,
    template: "%s | Duster",
  },
  description: DESCRIPTION,
  applicationName: "Duster",
  authors: [{ name: "Nur Adnan", url: "https://github.com/Nur-Adnan" }],
  alternates: {
    canonical: "/",
  },
  icons: {
    icon: "/duster-icon.png",
  },
  openGraph: {
    type: "website",
    url: "/",
    siteName: "Duster",
    locale: "en_US",
    title: TITLE,
    description: DESCRIPTION,
  },
  twitter: { card: "summary_large_image" },
  robots: { googleBot: { "max-image-preview": "large" } },
};

const jsonLd = {
  "@context": "https://schema.org",
  "@type": "SoftwareApplication",
  name: "Duster",
  alternateName: "du.exe",
  description: DESCRIPTION,
  url: siteUrl.href,
  image: new URL("/opengraph-image", siteUrl).href,
  operatingSystem: "Windows 10, Windows 11",
  applicationCategory: "UtilitiesApplication",
  license: links.license,
  isAccessibleForFree: true,
  sameAs: [links.repo],
  author: {
    "@type": "Person",
    name: "Nur Adnan",
    url: "https://github.com/Nur-Adnan",
  },
  offers: {
    "@type": "Offer",
    price: "0",
    priceCurrency: "USD",
  },
};

export const viewport: Viewport = {
  themeColor: "#0b0d12",
  colorScheme: "dark",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="en" className={`dark ${geistSans.variable} ${geistMono.variable} antialiased`}>
      <head>
        <script
          type="application/ld+json"
          // Escape "<" so no value can close the script tag (Next.js JSON-LD guide).
          dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd).replace(/</g, "\\u003c") }}
        />
      </head>
      <body>
        <a
          href="#main"
          className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-[100] focus:rounded-md focus:bg-brand-accent focus:px-4 focus:py-2 focus:text-sm focus:font-medium focus:text-white"
        >
          Skip to content
        </a>
        <MotionConfig reducedMotion="user">
          <SmoothScroll />
          {children}
        </MotionConfig>
      </body>
    </html>
  );
}
