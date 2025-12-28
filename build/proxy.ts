import { NextResponse } from 'next/server';
import type { NextRequest } from 'next/server';
import { decrypt } from '@/src/lib/auth';

export async function proxy(request: NextRequest) {
  const path = request.nextUrl.pathname;

  // LISTE DES ROUTES PUBLIQUES (Accessibles sans connexion)
  // Ajout de forgot-password et reset-password
  const publicRoutes = ['/login', '/forgot-password', '/reset-password', '/_next', '/favicon.ico'];

  // Si l'URL commence par l'une des routes publiques, on laisse passer
  if (publicRoutes.some(route => path.startsWith(route))) {
    return NextResponse.next();
  }

  // Vérification du cookie de session pour le reste
  const cookie = request.cookies.get('session')?.value;
  if (!cookie) {
    return NextResponse.redirect(new URL('/login', request.url));
  }

  try {
    const session = await decrypt(cookie);
    if (!session || new Date(session.expires) < new Date()) {
        // Session expirée ou invalide
        const response = NextResponse.redirect(new URL('/login', request.url));
        response.cookies.delete('session');
        return response;
    }
    
    // Tout est OK
    return NextResponse.next();
  } catch (err) {
    return NextResponse.redirect(new URL('/login', request.url));
  }
}

export const config = {
  matcher: ['/((?!api|_next/static|_next/image|favicon.ico).*)'],
};