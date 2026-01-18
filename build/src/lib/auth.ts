import { SignJWT, jwtVerify } from 'jose';
import { cookies } from 'next/headers';
import { NextRequest, NextResponse } from 'next/server';
import { ENV } from './env';
const key = new TextEncoder().encode(ENV.AUTH_SECRET);
const COOKIE_OPTIONS = {
  httpOnly: true,
  secure: process.env.NODE_ENV === 'production',
  sameSite: 'lax' as const,
  path: '/',
  maxAge: 24 * 60 * 60,
};
export async function encrypt(payload: any) {
  return await new SignJWT(payload)
    .setProtectedHeader({ alg: 'HS256' })
    .setIssuedAt()
    .setExpirationTime('24h')
    .sign(key);
}
export async function decrypt(input: string): Promise<any> {
  try {
    const { payload } = await jwtVerify(input, key, {
      algorithms: ['HS256'],
    });
    return payload;
  } catch (error) {
    return null;
  }
}
export async function login(userData: any) {
  const sessionData = {
    user: {
      id: userData.id,
      email: userData.email,
      role: userData.role,
      sessionVersion: userData.sessionVersion || 1,
    },
    expires: new Date(Date.now() + 24 * 60 * 60 * 1000),
  };
  const encryptedSession = await encrypt(sessionData);
  (await cookies()).set('session', encryptedSession, {
    ...COOKIE_OPTIONS,
    expires: sessionData.expires,
  });
}
export async function logout() {
  (await cookies()).set('session', '', { ...COOKIE_OPTIONS, maxAge: 0 });
}
export async function getSession() {
  const session = (await cookies()).get('session')?.value;
  if (!session) return null;
  return await decrypt(session);
}
export async function updateSession(request: NextRequest) {
  const session = request.cookies.get('session')?.value;
  if (!session) return;
  const parsed = await decrypt(session);
  if (!parsed) return;
  parsed.expires = new Date(Date.now() + 24 * 60 * 60 * 1000);
  const res = NextResponse.next();
  res.cookies.set({
    name: 'session',
    value: await encrypt(parsed),
    ...COOKIE_OPTIONS,
    expires: parsed.expires,
  });
  return res;
}