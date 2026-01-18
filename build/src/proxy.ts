import { NextRequest, NextResponse } from 'next/server';
import { updateSession } from '@/src/lib/auth';
const PUBLIC_ROUTES = [
  '/login', 
  '/register', 
  '/forgot-password', 
  '/reset-password',
  '/verify-email'
];
const STATIC_ASSETS = [
  '/_next', 
  '/static', 
  '/favicon.ico', 
  '/manifest.webmanifest',
  '/images'
];
export async function proxy(request: NextRequest) {
  const { pathname } = request.nextUrl;
  if (STATIC_ASSETS.some(path => pathname.startsWith(path))) {
    return NextResponse.next();
  }
  const response = await updateSession(request);
  const finalResponse = response || NextResponse.next();
  const isPublicRoute = PUBLIC_ROUTES.some(route => pathname.startsWith(route));
  const sessionCookie = request.cookies.get('session');
  if (!isPublicRoute && !sessionCookie) {
    const loginUrl = new URL('/login', request.url);
    return NextResponse.redirect(loginUrl);
  }
  return finalResponse;
}
export const config = {
  matcher: [
    '/((?!api|_next/static|_next/image|favicon.ico).*)',
  ],
};