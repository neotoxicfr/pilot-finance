import type { Metadata, Viewport } from "next";
import { Inter } from "next/font/google";
import "./globals.css";
import Link from "next/link";
import { LayoutDashboard, Wallet, LogOut, Settings } from "lucide-react";
import { getSession } from "@/src/lib/auth";
import { logoutAction } from "@/src/actions";
import BrandLogo from "@/src/components/BrandLogo";
import { ThemeProvider } from "@/src/components/theme-provider";
import { ThemeToggle } from "@/src/components/theme-toggle";
const inter = Inter({ subsets: ["latin"] });
export const viewport: Viewport = {
  themeColor: [
    { media: '(prefers-color-scheme: light)', color: '#ffffff' },
    { media: '(prefers-color-scheme: dark)', color: '#0f172a' },
  ],
  width: 'device-width',
  initialScale: 1,
  maximumScale: 1,
  userScalable: false,
  viewportFit: 'cover',
};
export const metadata: Metadata = {
  title: "Pilot Finance",
  description: "Cockpit financier personnel",
  manifest: "/manifest.json",
  icons: {
    icon: [
      { url: '/favicon.ico', sizes: 'any' },
      { url: '/logo.svg', type: 'image/svg+xml' },
    ],
    shortcut: '/logo.svg',
    apple: '/apple-icon.png',
  },
  appleWebApp: {
    capable: true,
    statusBarStyle: "default", 
    title: "Pilot",
  },
};
export default async function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  const session = await getSession();
  const user = session?.user;
  return (
    <html lang="fr" suppressHydrationWarning>
      <body className={`${inter.className} font-sans antialiased min-h-screen flex flex-col bg-background text-foreground`}>
        <ThemeProvider
          attribute="class"
          defaultTheme="system"
          enableSystem
          disableTransitionOnChange
          value={{ light: "light", dark: "dark" }}
        >
          {user && (
            <nav className="border-b border-border bg-background/50 backdrop-blur-md sticky top-0 z-50">
              <div className="max-w-[1400px] mx-auto px-2 sm:px-4 h-16 flex items-center justify-between">
                <div className="flex items-center gap-3 sm:gap-8">
                  <Link href="/" className="hover:opacity-80 transition-opacity flex-shrink-0 cursor-pointer">
                    <BrandLogo size={28} />
                  </Link>
                  <div className="flex items-center gap-1">
                    <Link href="/" className="flex items-center gap-2 px-2 py-2 text-sm font-medium text-foreground/70 hover:text-foreground hover:bg-accent rounded-lg transition-all cursor-pointer">
                      <LayoutDashboard size={20} />
                      <span className="hidden lg:inline">Dashboard</span>
                    </Link>
                    <Link href="/accounts" className="flex items-center gap-2 px-2 py-2 text-sm font-medium text-foreground/70 hover:text-foreground hover:bg-accent rounded-lg transition-all cursor-pointer">
                      <Wallet size={20} />
                      <span className="hidden lg:inline">Comptes</span>
                    </Link>
                  </div>
                </div>
                <div className="flex items-center gap-1 sm:gap-2">
                  <ThemeToggle />
                  <div className="h-4 w-px bg-border mx-0.5"></div>
                  <Link href="/settings" className="p-2 text-muted-foreground hover:text-foreground hover:bg-accent rounded-full transition-all cursor-pointer">
                    <Settings size={20} />
                  </Link>
                  <div className="h-4 w-px bg-border mx-0.5"></div>
                  <form action={logoutAction}>
                    <button type="submit" className="p-2 text-muted-foreground hover:text-destructive hover:bg-destructive/10 rounded-full transition-all cursor-pointer">
                      <LogOut size={20} />
                    </button>
                  </form>
                </div>
              </div>
            </nav>
          )}
          <main className="flex-1">
            {children}
          </main>
        </ThemeProvider>
      </body>
    </html>
  );
}