import { NextResponse } from 'next/server';
import { sql } from 'drizzle-orm';
import versionInfo from '@/src/version.json';

export const dynamic = 'force-dynamic';
export const runtime = 'nodejs';

interface HealthCheck {
  status: 'healthy' | 'degraded' | 'unhealthy';
  timestamp: string;
  version: string;
  buildDate: string;
  uptime: number;
  uptimeFormatted: string;
  checks: {
    database: {
      status: 'connected' | 'error';
      latency: number;
      size?: number;
      sizeFormatted?: string;
    };
    memory: {
      used: number;
      total: number;
      percentage: number;
    };
    gc?: {
      available: boolean;
    };
  };
}

function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const secs = Math.floor(seconds % 60);

  const parts = [];
  if (days > 0) parts.push(`${days}d`);
  if (hours > 0) parts.push(`${hours}h`);
  if (minutes > 0) parts.push(`${minutes}m`);
  if (secs > 0 || parts.length === 0) parts.push(`${secs}s`);

  return parts.join(' ');
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(2))} ${sizes[i]}`;
}

export async function GET() {
  const uptimeSeconds = process.uptime();

  const health: HealthCheck = {
    status: 'healthy',
    timestamp: new Date().toISOString(),
    version: versionInfo.version,
    buildDate: versionInfo.buildDate,
    uptime: uptimeSeconds,
    uptimeFormatted: formatUptime(uptimeSeconds),
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
      gc: {
        available: typeof global.gc === 'function',
      },
    },
  };

  // Database check with stats
  try {
    const { db, getDatabaseStats } = await import('@/src/db');
    const { users } = await import('@/src/schema');

    const dbStart = Date.now();
    await db.select({ count: sql<number>`1` }).from(users).limit(1);

    const dbStats = getDatabaseStats();

    health.checks.database = {
      status: 'connected',
      latency: Date.now() - dbStart,
      size: dbStats.size,
      sizeFormatted: formatBytes(dbStats.size),
    };
  } catch {
    health.status = 'unhealthy';
    health.checks.database.status = 'error';
  }

  // Memory check
  const mem = process.memoryUsage();
  health.checks.memory = {
    used: Math.round(mem.heapUsed / 1024 / 1024),
    total: Math.round(mem.heapTotal / 1024 / 1024),
    percentage: Math.round((mem.heapUsed / mem.heapTotal) * 100),
  };

  // Status degradation checks
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
