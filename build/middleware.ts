import { NextRequest, NextResponse } from 'next/server';
import { decrypt } from '@/src/lib/auth';

const PUBLIC_ROUTES = [
  '/login',
  '/forgot-password',
  '/reset-password',
  '/verify-email',
];

const STATIC_PATTERNS = [
  '/_next',
  '/favicon.ico',
  '/logo.svg',
  '/apple-icon.png',
  '/manifest.json',
];

function generateNonce(): string {
  const array = new Uint8Array(16);
  crypto.getRandomValues(array);
  return Buffer.from(array).toString('base64');
}

function buildCSP(nonce: string): string {
  const directives = [
    "default-src 'self'",
    // 'unsafe-inline' nécessaire pour Next.js + compatibilité Cloudflare (qui peut injecter/modifier des scripts)
    "script-src 'self' 'unsafe-inline' 'unsafe-eval'",
    `style-src 'self' 'nonce-${nonce}' 'unsafe-inline'`,
    "img-src 'self' blob: data:",
    "font-src 'self'",
    "connect-src 'self'",
    "object-src 'none'",
    "base-uri 'self'",
    "form-action 'self'",
    "frame-ancestors 'none'",
    "upgrade-insecure-requests",
  ];
  return directives.join('; ');
}

export async function middleware(request: NextRequest) {
  const { pathname } = request.nextUrl;

  if (STATIC_PATTERNS.some(pattern => pathname.startsWith(pattern))) {
    return NextResponse.next();
  }

  const nonce = generateNonce();
  const csp = buildCSP(nonce);

  const isPublicRoute = PUBLIC_ROUTES.some(route => pathname.startsWith(route));
  const sessionCookie = request.cookies.get('__Host-session');

  if (!isPublicRoute && !sessionCookie) {
    const loginUrl = new URL('/login', request.url);
    const response = NextResponse.redirect(loginUrl);
    response.headers.set('x-nonce', nonce);
    response.headers.set('Content-Security-Policy', csp);
    response.headers.set('Permissions-Policy', 'camera=(), microphone=(), geolocation=(), payment=()');
    return response;
  }

  if (sessionCookie) {
    try {
      const session = await decrypt(sessionCookie.value);
      if (!session || new Date(session.expires) < new Date()) {
        const loginUrl = new URL('/login', request.url);
        const response = NextResponse.redirect(loginUrl);
        response.cookies.delete('__Host-session');
        response.headers.set('x-nonce', nonce);
        response.headers.set('Content-Security-Policy', csp);
        response.headers.set('Permissions-Policy', 'camera=(), microphone=(), geolocation=(), payment=()');
        return response;
      }
    } catch {
      if (!isPublicRoute) {
        const loginUrl = new URL('/login', request.url);
        const response = NextResponse.redirect(loginUrl);
        response.cookies.delete('__Host-session');
        return response;
      }
    }
  }

  const response = NextResponse.next();
  response.headers.set('x-nonce', nonce);
  response.headers.set('Content-Security-Policy', csp);
  response.headers.set('Permissions-Policy', 'camera=(), microphone=(), geolocation=(), payment=()');

  return response;
}

export const config = {
  matcher: ['/((?!api|_next/static|_next/image|favicon.ico|logo.svg|apple-icon.png).*)'],
};
