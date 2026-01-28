import { NextResponse } from 'next/server';
import { db } from '@/src/db';
import { users } from '@/src/schema';
import { sql } from 'drizzle-orm';

interface HealthCheck {
  status: 'healthy' | 'degraded' | 'unhealthy';
  timestamp: string;
  version: string;
  uptime: number;
  checks: {
    database: {
      status: 'connected' | 'error';
      latency: number;
    };
    memory: {
      used: number;
      total: number;
      percentage: number;
    };
  };
}

export async function GET() {
  const health: HealthCheck = {
    status: 'healthy',
    timestamp: new Date().toISOString(),
    version: process.env.npm_package_version || '1.3.0',
    uptime: process.uptime(),
    checks: {
      database: {
        status: 'error',
        latency: 0,
      },
      memory: {
        used: 0,
        total: 0,
        percentage: 0,
      },
    },
  };

  try {
    const dbStart = Date.now();
    await db.select({ count: sql<number>`1` }).from(users).limit(1);
    health.checks.database = {
      status: 'connected',
      latency: Date.now() - dbStart,
    };
  } catch {
    health.status = 'unhealthy';
    health.checks.database.status = 'error';
  }

  const mem = process.memoryUsage();
  health.checks.memory = {
    used: Math.round(mem.heapUsed / 1024 / 1024),
    total: Math.round(mem.heapTotal / 1024 / 1024),
    percentage: Math.round((mem.heapUsed / mem.heapTotal) * 100),
  };

  if (health.checks.memory.percentage > 90) {
    health.status = health.status === 'unhealthy' ? 'unhealthy' : 'degraded';
  }

  if (health.checks.database.latency > 1000) {
    health.status = health.status === 'unhealthy' ? 'unhealthy' : 'degraded';
  }

  const statusCode = health.status === 'healthy' ? 200 : health.status === 'degraded' ? 200 : 503;

  return NextResponse.json(health, {
    status: statusCode,
    headers: {
      'Cache-Control': 'no-store, no-cache, must-revalidate',
    },
  });
}
