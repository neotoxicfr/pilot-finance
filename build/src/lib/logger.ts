import pino from 'pino';

const isDev = process.env.NODE_ENV !== 'production';

const logger = pino({
  level: process.env.LOG_LEVEL || (isDev ? 'debug' : 'info'),

  transport: isDev
    ? {
        target: 'pino-pretty',
        options: {
          colorize: true,
          translateTime: 'SYS:standard',
          ignore: 'pid,hostname',
        },
      }
    : undefined,

  base: {
    env: process.env.NODE_ENV,
    service: 'pilot-finance',
  },

  redact: {
    paths: [
      'password',
      'token',
      'secret',
      'email',
      'mfaSecret',
      'currentPassword',
      'newPassword',
      'confirmPassword',
      'req.headers.authorization',
      'req.headers.cookie',
    ],
    remove: true,
  },

  formatters: {
    level: (label) => ({ level: label }),
  },

  timestamp: pino.stdTimeFunctions.isoTime,
});

export default logger;
