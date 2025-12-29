import { sqliteTable, text, integer, real, index } from 'drizzle-orm/sqlite-core';

export const users = sqliteTable('users', {
  id: integer('id').primaryKey({ autoIncrement: true }),
  emailEncrypted: text('email_encrypted').notNull(),
  emailBlindIndex: text('email_blind_index').notNull().unique(),
  password: text('password').notNull(),
  role: text('role').default('USER'),
  createdAt: integer('created_at', { mode: 'timestamp' }).$defaultFn(() => new Date()),
  resetToken: text('reset_token'),
  resetTokenExpiry: integer('reset_token_expiry', { mode: 'timestamp' }),
  mfaEnabled: integer('mfa_enabled', { mode: 'boolean' }).default(false),
  mfaSecret: text('mfa_secret'),
  email_verified: integer('email_verified', { mode: 'boolean' }).default(true),
  verification_token: text('verification_token'),
}, (table) => ({
  // Index pour recherche rapide lors du login (blind index)
  emailIdx: index('user_email_idx').on(table.emailBlindIndex),
}));

export const authenticators = sqliteTable('authenticators', {
  id: integer('id').primaryKey({ autoIncrement: true }),
  credentialID: text('credential_id').notNull().unique(),
  credentialPublicKey: text('credential_public_key').notNull(),
  counter: integer('counter').notNull(),
  credentialDeviceType: text('credential_device_type').notNull(),
  credentialBackedUp: integer('credential_backed_up', { mode: 'boolean' }).notNull(),
  transports: text('transports'),
  userId: integer('user_id').references(() => users.id, { onDelete: 'cascade' }),
  name: text('name').default('Clé de sécurité'),
}, (table) => ({
  userIdIdx: index('auth_user_idx').on(table.userId),
  credentialIdIdx: index('auth_cred_idx').on(table.credentialID),
}));

export const accounts = sqliteTable('accounts', {
  id: integer('id').primaryKey({ autoIncrement: true }),
  userId: integer('user_id').references(() => users.id, { onDelete: 'cascade' }),
  name: text('name').notNull(),
  balance: real('balance').default(0).notNull(),
  color: text('color').default('#3b82f6'),
  isYieldActive: integer('is_yield_active', { mode: 'boolean' }).default(false),
  yieldType: text('yield_type').default('FIXED'),
  yieldMin: real('yield_min').default(0),
  yieldMax: real('yield_max').default(0),
  yieldFrequency: text('yield_frequency').default('YEARLY'),
  payoutFrequency: text('payout_frequency').default('MONTHLY'),
  reinvestmentRate: integer('reinvestment_rate').default(100),
  targetAccountId: integer('target_account_id'),
  lastYieldDate: integer('last_yield_date', { mode: 'timestamp' }),
  position: integer('position').default(0),
  updatedAt: integer('updated_at', { mode: 'timestamp' }).$defaultFn(() => new Date()),
}, (table) => ({
  userIdIdx: index('acc_user_idx').on(table.userId),
}));

export const transactions = sqliteTable('transactions', {
  id: integer('id').primaryKey({ autoIncrement: true }),
  userId: integer('user_id').references(() => users.id, { onDelete: 'cascade' }),
  accountId: integer('account_id').references(() => accounts.id, { onDelete: 'cascade' }).notNull(),
  amount: real('amount').notNull(),
  description: text('description').notNull(),
  date: integer('date', { mode: 'timestamp' }).$defaultFn(() => new Date()).notNull(),
  category: text('category'),
}, (table) => ({
  // CRITIQUE : Index sur userId pour filtrer les données de l'utilisateur courant
  userIdIdx: index('tx_user_idx').on(table.userId),
  // CRITIQUE : Index sur accountId pour l'historique d'un compte spécifique
  accountIdIdx: index('tx_account_idx').on(table.accountId),
  // CRITIQUE : Index sur date pour les tris chronologiques et graphiques
  dateIdx: index('tx_date_idx').on(table.date),
}));

export const recurringOperations = sqliteTable('recurring_operations', {
  id: integer('id').primaryKey({ autoIncrement: true }),
  userId: integer('user_id').references(() => users.id, { onDelete: 'cascade' }),
  accountId: integer('account_id').references(() => accounts.id, { onDelete: 'cascade' }).notNull(),
  toAccountId: integer('to_account_id').references(() => accounts.id, { onDelete: 'set null' }),
  amount: real('amount').notNull(),
  description: text('description').notNull(),
  dayOfMonth: integer('day_of_month').notNull(),
  lastRunDate: integer('last_run_date', { mode: 'timestamp' }),
  isActive: integer('is_active', { mode: 'boolean' }).default(true),
}, (table) => ({
  userIdIdx: index('rec_user_idx').on(table.userId),
  activeIdx: index('rec_active_idx').on(table.isActive),
}));