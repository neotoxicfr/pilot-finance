import { drizzle } from 'drizzle-orm/better-sqlite3';
import Database from 'better-sqlite3';
import logger from './lib/logger';

const dbPath = process.env.DATABASE_URL?.replace('file:', '') || '/data/pilot.db';
const encryptionKey = process.env.DB_ENCRYPTION_KEY;

function initializeDatabase(): Database.Database {
  const sqlite = new Database(dbPath);

  if (encryptionKey && encryptionKey.length >= 32) {
    try {
      sqlite.pragma(`key = '${encryptionKey}'`);
      logger.info('Database encryption key applied');
    } catch {
      logger.warn('Database encryption not supported - using better-sqlite3 standard');
    }
  }

  sqlite.pragma('journal_mode = WAL');
  sqlite.pragma('synchronous = NORMAL');
  sqlite.pragma('cache_size = -64000');
  sqlite.pragma('busy_timeout = 30000');
  sqlite.pragma('foreign_keys = ON');
  sqlite.pragma('temp_store = MEMORY');
  sqlite.pragma('mmap_size = 268435456');
  sqlite.pragma('auto_vacuum = INCREMENTAL');
  sqlite.pragma('secure_delete = ON');

  try {
    sqlite.prepare('SELECT 1').get();
  } catch (error) {
    logger.error({ err: error }, 'Failed to initialize database');
    throw new Error('Database initialization failed');
  }

  const walMode = sqlite.pragma('journal_mode', { simple: true });
  logger.info({ dbPath, journalMode: walMode }, 'Database initialized');

  return sqlite;
}

// Skip database initialization during Next.js build
const isBuild = process.env.NEXT_PHASE === 'phase-production-build';
const sqlite = isBuild ? null : initializeDatabase();
export const db = sqlite ? drizzle(sqlite) : null as any;

export function optimizeDatabase(): void {
  if (!sqlite) return;
  sqlite.pragma('optimize');
  sqlite.pragma('incremental_vacuum(1000)');
  logger.info('Database optimization completed');
}

export function getDatabaseStats(): {
  size: number;
  pageCount: number;
  freePages: number;
} {
  if (!sqlite) return { size: 0, pageCount: 0, freePages: 0 };
  const pageCount = sqlite.pragma('page_count', { simple: true }) as number;
  const pageSize = sqlite.pragma('page_size', { simple: true }) as number;
  const freePages = sqlite.pragma('freelist_count', { simple: true }) as number;

  return {
    size: pageCount * pageSize,
    pageCount,
    freePages,
  };
}

export function closeDatabase(): void {
  if (!sqlite) return;
  sqlite.pragma('optimize');
  sqlite.close();
  logger.info('Database connection closed');
}

// Manual garbage collection in production to reduce memory usage
if (process.env.NODE_ENV === 'production' && !isBuild && global.gc) {
  setInterval(() => {
    global.gc();
    logger.debug('Manual garbage collection triggered');
  }, 60000); // Every 60 seconds
}

process.on('SIGINT', () => {
  closeDatabase();
  process.exit(0);
});

process.on('SIGTERM', () => {
  closeDatabase();
  process.exit(0);
});
