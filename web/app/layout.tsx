import type { Metadata } from "next";
import Script from "next/script";
import { Archivo } from "next/font/google";
import "./globals.css";
import Header from "@/components/Header";
import Footer from "@/components/Footer";
import { SITE_NAME } from "@/lib/format";

/** Google Analytics measurement ID (public; it appears in the page source). */
const GA_ID = "G-CX195187X0";

const archivo = Archivo({
  subsets: ["latin"],
  axes: ["wdth"],
  variable: "--font-archivo",
  display: "swap",
});

export const metadata: Metadata = {
  metadataBase: new URL((process.env.SITE_URL || "https://slabsnsingles.com").replace(/\/$/, "")),
  openGraph: { siteName: SITE_NAME, type: "website", locale: "en_US" },
  title: { default: `${SITE_NAME} — sports cards and TCG singles`, template: `%s | ${SITE_NAME}` },
  description: "Buy and sell sports cards and trading card game singles, raw or graded, up to $1,000. US only, free shipping on every card.",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={archivo.variable}>
      <body className="min-h-screen font-sans text-[15px] leading-normal">
        {/* Google Analytics (gtag.js) on every page. Page changes inside the
            site are counted by GA4's enhanced measurement (history changes). */}
        <Script src={`https://www.googletagmanager.com/gtag/js?id=${GA_ID}`} strategy="afterInteractive" />
        <Script id="google-analytics" strategy="afterInteractive">
          {`window.dataLayer = window.dataLayer || [];
function gtag(){dataLayer.push(arguments);}
gtag('js', new Date());
gtag('config', '${GA_ID}');`}
        </Script>
        <a href="#main" className="sr-only focus:not-sr-only focus:absolute focus:left-2 focus:top-2 focus:bg-paper focus:px-3 focus:py-2">
          Skip to content
        </a>
        <Header />
        <main id="main" className="mx-auto max-w-[1280px] px-4 pb-16">
          {children}
        </main>
        <Footer />
      </body>
    </html>
  );
}
