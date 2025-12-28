import { SignJWT, jwtVerify } from 'jose';
import { cookies } from 'next/headers';

const secretKey = process.env.AUTH_SECRET;
const key = new TextEncoder().encode(secretKey);

export async function encrypt(payload: any) {
  return await new SignJWT(payload)
    .setProtectedHeader({ alg: 'HS256' })
    .setIssuedAt()
    .setExpirationTime('1 week')
    .sign(key);
}

export async function decrypt(input: string): Promise<any> {
  const { payload } = await jwtVerify(input, key, { algorithms: ['HS256'] });
  return payload;
}

export async function login(userData: any) {
  const expires = new Date(Date.now() + 7 * 24 * 60 * 60 * 1000);
  // 1 semaine
  const session = await encrypt({ user: userData, expires });
  // Cookie HttpOnly : inaccessible via JavaScript côté client (sécurité XSS)
  (await cookies()).set('session', session, { expires, httpOnly: true, sameSite: 'lax' });
}

export async function logout() {
  (await cookies()).set('session', '', { expires: new Date(0) });
}

export async function getSession() {
  const session = (await cookies()).get('session')?.value;
  if (!session) return null;
  try {
    return await decrypt(session);
  } catch (e) {
    return null;
  }
}