export function getHtmlTemplate(title: string, message: string, buttonText: string, buttonUrl: string) {
    return `
  <!DOCTYPE html>
  <html>
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>${title}</title>
  </head>
  <body style="margin: 0; padding: 0; background-color: #f8fafc; font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; -webkit-font-smoothing: antialiased;">
    <table role="presentation" width="100%" border="0" cellspacing="0" cellpadding="0" style="background-color: #f8fafc;">
      <tr>
        <td align="center" style="padding: 40px 20px;">
          <table role="presentation" border="0" cellspacing="0" cellpadding="0" style="margin-bottom: 24px;">
            <tr>
              <td align="center">
                <h1 style="margin: 0; color: #0f172a; font-size: 24px; font-weight: 700; letter-spacing: -0.5px;">Pilot Finance</h1>
              </td>
            </tr>
          </table>
          <table role="presentation" width="100%" border="0" cellspacing="0" cellpadding="0" style="max-width: 480px; background-color: #ffffff; border-radius: 16px; border: 1px solid #e2e8f0; overflow: hidden;">
            <tr>
              <td style="padding: 40px;">
                <h2 style="margin: 0 0 16px 0; color: #0f172a; font-size: 20px; font-weight: 600;">${title}</h2>
                <p style="margin: 0 0 32px 0; color: #64748b; font-size: 15px; line-height: 1.6;">
                  ${message}
                </p>
                <table role="presentation" border="0" cellspacing="0" cellpadding="0" width="100%">
                  <tr>
                    <td align="center">
                      <a href="${buttonUrl}" target="_blank" style="display: inline-block; background-color: #2563eb; color: #ffffff; font-size: 15px; font-weight: 600; text-decoration: none; padding: 14px 32px; border-radius: 12px; transition: background-color 0.2s;">
                        ${buttonText}
                      </a>
                    </td>
                  </tr>
                </table>
                <p style="margin: 32px 0 0 0; color: #94a3b8; font-size: 12px; line-height: 1.5; text-align: center;">
                  Si le bouton ne fonctionne pas, copiez ce lien :<br>
                  <a href="${buttonUrl}" style="color: #2563eb; text-decoration: none; word-break: break-all;">${buttonUrl}</a>
                </p>
              </td>
            </tr>
          </table>
          <table role="presentation" border="0" cellspacing="0" cellpadding="0" style="margin-top: 24px;">
            <tr>
              <td align="center" style="color: #94a3b8; font-size: 12px;">
                &copy; ${new Date().getFullYear()} Pilot Finance. Cockpit Financier Personnel.
              </td>
            </tr>
          </table>
        </td>
      </tr>
    </table>
  </body>
  </html>
    `;
  }