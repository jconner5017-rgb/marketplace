import type { Metadata } from "next";
import { Archivo } from "next/font/google";
import "./globals.css";
import Header from "@/components/Header";
import { SITE_NAME } from "@/lib/format";

const archivo = Archivo({
  subsets: ["latin"],
  axes: ["wdth"],
  variable: "--font-archivo",
  display: "swap",
});

export const metadata: Metadata = {
  title: { default: `${SITE_NAME} — sports cards and TCG singles`, template: `%s | ${SITE_NAME}` },
  description: "Buy and sell sports cards and trading card game singles, raw or graded, up to $1,000. US only, free shipping on every card.",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={archivo.variable}>
      <body className="min-h-screen font-sans text-[15px] leading-normal">
        <a href="#main" className="sr-only focus:not-sr-only focus:absolute focus:left-2 focus:top-2 focus:bg-paper focus:px-3 focus:py-2">
          Skip to content
        </a>
        <Header />
        <main id="main" className="mx-auto max-w-[1280px] px-4 pb-16">
          {children}
        </main>
        <footer className="border-t border-line bg-wash">
          <div className="mx-auto flex max-w-[1280px] flex-wrap gap-x-6 gap-y-2 px-4 py-6 text-sm text-muted">
            <span>© {new Date().getFullYear()} {SITE_NAME}</span>
            <span>US buyers and sellers only</span>
            <span>Payments by Stripe</span>
            <span>Free shipping on every card</span>
          </div>
        </footer>
      </body>
    </html>
  );
}
