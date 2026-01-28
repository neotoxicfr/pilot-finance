function escapeHtml(unsafe: string): string {
  return unsafe
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#039;");
}

function escapeUrl(url: string): string {
  try {
    const parsed = new URL(url);
    if (!['http:', 'https:'].includes(parsed.protocol)) {
      return '#';
    }
    return parsed.toString();
  } catch {
    return '#';
  }
}

export function getHtmlTemplate(
  title: string,
  message: string,
  buttonText?: string,
  buttonUrl?: string
): string {
  const safeTitle = escapeHtml(title);
  const safeMessage = escapeHtml(message);
  const safeButtonText = buttonText ? escapeHtml(buttonText) : '';
  const safeButtonUrl = buttonUrl ? escapeUrl(buttonUrl) : '';

  return `
<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; line-height: 1.6; color: #333; max-width: 600px; margin: 0 auto; padding: 20px; }
  .header { text-align: center; margin-bottom: 30px; }
  .logo { font-size: 24px; font-weight: bold; color: #2563eb; text-decoration: none; }
  .card { background: #ffffff; border: 1px solid #e5e7eb; border-radius: 12px; padding: 30px; box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1); }
  .title { font-size: 20px; font-weight: 600; margin-top: 0; margin-bottom: 16px; color: #111827; }
  .button { display: inline-block; background-color: #2563eb; color: #ffffff; padding: 12px 24px; border-radius: 6px; text-decoration: none; font-weight: 500; margin-top: 20px; }
  .footer { margin-top: 30px; text-align: center; font-size: 12px; color: #6b7280; }
</style>
</head>
<body>
  <div class="header">
    <span class="logo">Pilot Finance</span>
  </div>
  <div class="card">
    <h1 class="title">${safeTitle}</h1>
    <p>${safeMessage}</p>
    ${buttonText && safeButtonUrl !== '#' ? `<a href="${safeButtonUrl}" class="button">${safeButtonText}</a>` : ''}
  </div>
  <div class="footer">
    <p>Ce message vous est envoyé automatiquement par votre instance Pilot Finance.</p>
  </div>
</body>
</html>
  `.trim();
}
