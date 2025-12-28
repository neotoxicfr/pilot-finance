export const IS_MAIL_ENABLED = process.env.ENABLE_MAIL === 'true';
import nodemailer from 'nodemailer';

const SMTP_HOST = process.env.SMTP_HOST;
const SMTP_PORT = parseInt(process.env.SMTP_PORT || '587');
const SMTP_USER = process.env.SMTP_USER;
const SMTP_PASS = process.env.SMTP_PASS;
const SMTP_FROM = process.env.SMTP_FROM || '"Pilot Finance" <noreply@pilot.com>';

export async function sendEmail({ to, subject, html }: { to: string, subject: string, html: string }) {
  // Vérification de sécurité : si pas de config, on ne tente rien
  if (!SMTP_HOST || !SMTP_USER || !SMTP_PASS) {
    console.warn("⚠️  Email non envoyé : Configuration SMTP manquante.");
    return false;
  }

  const transporter = nodemailer.createTransport({
    host: SMTP_HOST,
    port: SMTP_PORT,
    secure: SMTP_PORT === 465, // true pour le port 465, false pour les autres
    auth: {
      user: SMTP_USER,
      pass: SMTP_PASS,
    },
  });

  try {
    const info = await transporter.sendMail({
      from: SMTP_FROM,
      to,
      subject,
      html,
    });
    console.log("✅ Email envoyé : ", info.messageId);
    return true;
  } catch (error) {
    console.error("❌ Erreur envoi email : ", error);
    return false;
  }
}