import type { Metadata } from "next";
import "./globals.css";
import Link from "next/link";
import { LayoutDashboard, Wallet, LogOut, Settings } from "lucide-react";
import { getSession } from "@/src/lib/auth";
import { logoutAction } from "@/src/actions";
import BrandLogo from "@/src/components/BrandLogo";

export const metadata: Metadata = {
  title: "Pilot Finance",
  description: "Cockpit financier personnel",
  // AJOUTER CECI :
  icons: {
    icon: [
      { url: '/favicon.ico', sizes: 'any' }, // Pour les gestionnaires old-school
      { url: '/logo.svg', type: 'image/svg+xml' }, // Pour les navigateurs modernes
    ],
    shortcut: '/logo.svg',
    apple: '/apple-icon.png', // Pour iPhone/iPad
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
    <html lang="fr" className="bg-slate-950">
      <body className="font-sans antialiased min-h-screen flex flex-col text-slate-200">
        
        {/* BANDEAU UNIQUE DE NAVIGATION */}
        {user && (
          <nav className="border-b border-slate-800 bg-slate-900/50 backdrop-blur-md sticky top-0 z-50">
            
            <div className="max-w-[1600px] mx-auto px-4 h-16 flex items-center justify-between">
              
              {/* GAUCHE : Logo + Liens */}
              <div className="flex items-center gap-8">
                <Link href="/" className="hover:opacity-80 transition-opacity">
                   <BrandLogo size={32} />
                </Link>
                
                <div className="flex items-center gap-1">
                  <Link href="/" className="flex items-center gap-2 px-3 py-2 text-sm font-medium text-slate-300 hover:text-white hover:bg-slate-800 rounded-lg transition-all">
                    <LayoutDashboard size={16} />
                    <span className="hidden sm:inline">Dashboard</span>
                  </Link>
                  <Link href="/accounts" className="flex items-center gap-2 px-3 py-2 text-sm font-medium text-slate-300 hover:text-white hover:bg-slate-800 rounded-lg transition-all">
                    <Wallet size={16} />
                    <span className="hidden sm:inline">Comptes</span>
                  </Link>
                </div>
              </div>

              {/* DROITE : Actions */}
              <div className="flex items-center gap-2">
                
                {/* Paramètres (Inclut Admin si éligible) */}
                <Link 
                    href="/settings" 
                    className="p-2 text-slate-400 hover:text-blue-400 hover:bg-blue-500/10 rounded-full transition-all"
                    title="Paramètres"
                  >
                    <Settings size={20} />
                </Link>

                {/* Séparateur discret */}
                <div className="h-4 w-px bg-slate-800 mx-1"></div>

                {/* Déconnexion */}
                <form action={logoutAction}>
                  <button className="p-2 text-slate-400 hover:text-red-400 hover:bg-red-500/10 rounded-full transition-all" title="Se déconnecter">
                    <LogOut size={20} />
                  </button>
                </form>
              </div>

            </div>
          </nav>
        )}

        {children}
      </body>
    </html>
  );
}