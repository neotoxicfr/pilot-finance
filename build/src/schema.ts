import { sqliteTable, text, integer, real, index } from "drizzle-orm/sqlite-core";

export const users = sqliteTable("users", {
  id: integer("id").primaryKey({ autoIncrement: true }),
  emailEncrypted: text("email_encrypted").notNull(),
  emailBlindIndex: text("email_blind_index").notNull().unique(),
  password: text("password").notNull(),
  role: text("role").default("USER"),
  createdAt: integer("created_at", { mode: "timestamp" }).default(new Date()),
  email_verified: integer("email_verified", { mode: "boolean" }).default(false),
  verification_token: text("verification_token"),
  resetToken: text("reset_token"),
  resetTokenExpiry: integer("reset_token_expiry", { mode: "timestamp" }),
  mfaEnabled: integer("mfa_enabled", { mode: "boolean" }).default(false),
  mfaSecret: text("mfa_secret"),
  failedLoginAttempts: integer("failed_login_attempts").default(0),
  lockUntil: integer("lock_until", { mode: "timestamp" }),
  sessionVersion: integer("session_version").default(1).notNull(),
}, (table) => ({
  verifyTokenIdx: index("user_verify_token_idx").on(table.verification_token),
  resetTokenIdx: index("user_reset_token_idx").on(table.resetToken),
  emailBlindIdx: index("user_email_blind_idx").on(table.emailBlindIndex),
}));

export const accounts = sqliteTable("accounts", {
  id: integer("id").primaryKey({ autoIncrement: true }),
  userId: integer("user_id").references(() => users.id, { onDelete: 'cascade' }),
  name: text("name").notNull(),
  balance: real("balance").notNull().default(0),
  color: text("color").default("#3b82f6"),
  position: integer("position").default(0),
  updatedAt: integer("updated_at", { mode: "timestamp" }).default(new Date()),
  isYieldActive: integer("is_yield_active", { mode: "boolean" }).default(false),
  yieldType: text("yield_type").default("FIXED"),
  yieldMin: real("yield_min").default(0),
  yieldMax: real("yield_max").default(0),
  yieldFrequency: text("yield_frequency").default("YEARLY"),
  payoutFrequency: text("payout_frequency").default("MONTHLY"),
  lastYieldDate: integer("last_yield_date", { mode: "timestamp" }),
  reinvestmentRate: integer("reinvestment_rate").default(100),
  targetAccountId: integer("target_account_id"),
}, (table) => ({
  userIdIdx: index("account_user_id_idx").on(table.userId),
  userPositionIdx: index("account_user_position_idx").on(table.userId, table.position),
}));

export const transactions = sqliteTable("transactions", {
  id: integer("id").primaryKey({ autoIncrement: true }),
  userId: integer("user_id").references(() => users.id, { onDelete: 'cascade' }),
  accountId: integer("account_id").references(() => accounts.id, { onDelete: 'cascade' }),
  amount: real("amount").notNull(),
  description: text("description").notNull(),
  category: text("category"),
  date: integer("date", { mode: "timestamp" }).notNull(),
  createdAt: integer("created_at", { mode: "timestamp" }).default(new Date()),
}, (table) => ({
  userIdIdx: index("tx_user_id_idx").on(table.userId),
  accountIdIdx: index("tx_account_id_idx").on(table.accountId),
  dateIdx: index("tx_date_idx").on(table.date),
  userAccountDateIdx: index("tx_user_account_date_idx").on(table.userId, table.accountId, table.date),
  accountDateIdx: index("tx_account_date_idx").on(table.accountId, table.date),
}));

export const recurringOperations = sqliteTable("recurring_operations", {
  id: integer("id").primaryKey({ autoIncrement: true }),
  userId: integer("user_id").references(() => users.id, { onDelete: 'cascade' }),
  accountId: integer("account_id").references(() => accounts.id, { onDelete: 'cascade' }).notNull(),
  toAccountId: integer("to_account_id"),
  amount: real("amount").notNull(),
  description: text("description").notNull(),
  dayOfMonth: integer("day_of_month").notNull(),
  lastRunDate: integer("last_run_date", { mode: "timestamp" }),
  isActive: integer("is_active", { mode: "boolean" }).default(true),
}, (table) => ({
  userIdIdx: index("rec_user_id_idx").on(table.userId),
  accountIdIdx: index("rec_account_id_idx").on(table.accountId),
  userActiveIdx: index("rec_user_active_idx").on(table.userId, table.isActive),
}));

export const authenticators = sqliteTable("authenticators", {
  id: integer("id").primaryKey({ autoIncrement: true }),
  credentialID: text("credential_id").notNull().unique(),
  credentialPublicKey: text("credential_public_key").notNull(),
  counter: integer("counter").notNull().default(0),
  credentialDeviceType: text("credential_device_type").notNull(),
  credentialBackedUp: integer("credential_backed_up", { mode: "boolean" }).notNull(),
  transports: text("transports"),
  userId: integer("user_id").references(() => users.id, { onDelete: 'cascade' }),
  name: text("name"),
}, (table) => ({
  userIdIdx: index("auth_user_id_idx").on(table.userId),
  credentialIdIdx: index("auth_credential_id_idx").on(table.credentialID),
}));
