import { drizzle } from 'drizzle-orm/better-sqlite3';
import Database from 'better-sqlite3';

// On retire l'import du migrator car on gère la structure via 'drizzle-kit push' manuellement
// import { migrate } from 'drizzle-orm/better-sqlite3/migrator'; 

const dbPath = process.env.DATABASE_URL?.replace("file:", "") || 'sqlite.db';

const sqlite = new Database(dbPath);
export const db = drizzle(sqlite);

// IMPORTANT : On commente ou supprime cette ligne pour empêcher l'app de tenter de recréer les tables
// migrate(db, { migrationsFolder: "drizzle" });