import nodemailer from 'nodemailer';
import logger from './logger';

export const IS_MAIL_ENABLED = process.env.ENABLE_MAIL === 'true';

const SMTP_HOST = process.env.SMTP_HOST;
const SMTP_PORT = parseInt(process.env.SMTP_PORT || '587');
const SMTP_USER = process.env.SMTP_USER;
const SMTP_PASS = process.env.SMTP_PASS;
const SMTP_FROM = process.env.SMTP_FROM || '"Pilot Finance" <noreply@pilot.com>';

let transporter: nodemailer.Transporter | null = null;

function getTransporter(): nodemailer.Transporter | null {
  if (!SMTP_HOST || !SMTP_USER || !SMTP_PASS) {
    return null;
  }

  if (!transporter) {
    transporter = nodemailer.createTransport({
      host: SMTP_HOST,
      port: SMTP_PORT,
      secure: SMTP_PORT === 465,
      auth: {
        user: SMTP_USER,
        pass: SMTP_PASS,
      },
      pool: true,
      maxConnections: 5,
    });
  }

  return transporter;
}

interface EmailOptions {
  to: string;
  subject: string;
  html: string;
}

export async function sendEmail({ to, subject, html }: EmailOptions): Promise<boolean> {
  const smtp = getTransporter();

  if (!smtp) {
    logger.warn({ to: '[REDACTED]' }, 'Email non envoyé : Configuration SMTP manquante');
    return false;
  }

  try {
    const info = await smtp.sendMail({
      from: SMTP_FROM,
      to,
      subject,
      html,
    });

    logger.info({ messageId: info.messageId }, 'Email envoyé avec succès');
    return true;
  } catch (error) {
    logger.error({ err: error }, 'Erreur envoi email');
    return false;
  }
}
