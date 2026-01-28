import { SignJWT, jwtVerify } from 'jose';
import { cookies } from 'next/headers';
import { ENV } from './env';

const key = new TextEncoder().encode(ENV.AUTH_SECRET);
const COOKIE_NAME = '__Host-session';
const SESSION_DURATION = 24 * 60 * 60 * 1000;

const COOKIE_OPTIONS = {
  httpOnly: true,
  secure: true,
  sameSite: 'lax' as const,
  path: '/',
};

interface UserData {
  id: number;
  email: string;
  role: string;
  sessionVersion: number;
}

interface SessionPayload {
  user: UserData;
  expires: Date;
}

export async function encrypt(payload: SessionPayload): Promise<string> {
  return await new SignJWT({ ...payload, expires: payload.expires.toISOString() })
    .setProtectedHeader({ alg: 'HS256' })
    .setIssuedAt()
    .setExpirationTime('24h')
    .sign(key);
}

export async function decrypt(input: string): Promise<SessionPayload | null> {
  try {
    const { payload } = await jwtVerify(input, key, {
      algorithms: ['HS256'],
    });
    return {
      user: payload.user as UserData,
      expires: new Date(payload.expires as string),
    };
  } catch {
    return null;
  }
}

export async function login(userData: UserData): Promise<void> {
  const sessionData: SessionPayload = {
    user: {
      id: userData.id,
      email: userData.email,
      role: userData.role,
      sessionVersion: userData.sessionVersion || 1,
    },
    expires: new Date(Date.now() + SESSION_DURATION),
  };

  const encryptedSession = await encrypt(sessionData);
  const cookieStore = await cookies();

  cookieStore.set(COOKIE_NAME, encryptedSession, {
    ...COOKIE_OPTIONS,
    expires: sessionData.expires,
  });
}

export async function logout(): Promise<void> {
  const cookieStore = await cookies();
  cookieStore.delete(COOKIE_NAME);
}

export async function getSession(): Promise<SessionPayload | null> {
  const cookieStore = await cookies();
  const session = cookieStore.get(COOKIE_NAME)?.value;

  if (!session) {
    return null;
  }

  const parsed = await decrypt(session);

  if (!parsed || new Date(parsed.expires) < new Date()) {
    return null;
  }

  return parsed;
}

export async function refreshSession(): Promise<void> {
  const session = await getSession();
  if (!session) return;

  const newExpires = new Date(Date.now() + SESSION_DURATION);
  const newSession: SessionPayload = {
    ...session,
    expires: newExpires,
  };

  const encryptedSession = await encrypt(newSession);
  const cookieStore = await cookies();

  cookieStore.set(COOKIE_NAME, encryptedSession, {
    ...COOKIE_OPTIONS,
    expires: newExpires,
  });
}
