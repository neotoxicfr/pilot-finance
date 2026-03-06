// Passkey/WebAuthn JavaScript Handler

// Toast notification helper — shows a temporary message using a fixed-position element.
// Does not rely on Alpine.js so it works even before Alpine is initialised.
function showPasskeyToast(message, isError) {
    var container = document.getElementById('passkey-toast');
    if (!container) {
        container = document.createElement('div');
        container.id = 'passkey-toast';
        container.setAttribute('role', 'alert');
        container.setAttribute('aria-live', 'assertive');
        container.style.cssText = 'position:fixed;top:1.5rem;right:1.5rem;z-index:9999;display:flex;flex-direction:column;gap:0.5rem;pointer-events:none;';
        document.body.appendChild(container);
    }
    var toast = document.createElement('div');
    toast.textContent = message;
    var bg = isError
        ? 'background:rgba(239,68,68,0.95);color:#fff;'
        : 'background:var(--background,#fff);color:var(--foreground,#0f172a);border:1px solid var(--border,#e2e8f0);';
    toast.style.cssText = bg + 'pointer-events:auto;padding:0.75rem 1.25rem;border-radius:0.75rem;font-size:0.875rem;font-weight:600;box-shadow:0 8px 24px rgba(0,0,0,0.15);opacity:0;transform:translateY(-8px);transition:opacity 0.2s,transform 0.2s;max-width:22rem;';
    container.appendChild(toast);
    // Trigger entrance animation
    requestAnimationFrame(function() {
        toast.style.opacity = '1';
        toast.style.transform = 'translateY(0)';
    });
    // Auto-dismiss after 4 seconds
    setTimeout(function() {
        toast.style.opacity = '0';
        toast.style.transform = 'translateY(-8px)';
        setTimeout(function() { toast.remove(); }, 200);
    }, 4000);
}

// Utilitaires base64url
function base64urlToBuffer(base64url) {
    const base64 = base64url.replace(/-/g, '+').replace(/_/g, '/');
    const padding = '='.repeat((4 - base64.length % 4) % 4);
    const binary = atob(base64 + padding);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) {
        bytes[i] = binary.charCodeAt(i);
    }
    return bytes.buffer;
}

function bufferToBase64url(buffer) {
    const bytes = new Uint8Array(buffer);
    let binary = '';
    for (let i = 0; i < bytes.length; i++) {
        binary += String.fromCharCode(bytes[i]);
    }
    const base64 = btoa(binary);
    return base64.replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '');
}

// Login avec Passkey
async function loginWithPasskey() {
    try {
        // 1. Obtenir les options du serveur
        const startResponse = await fetch('/api/passkey/login/start', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include'
        });

        if (!startResponse.ok) {
            const error = await startResponse.text();
            throw new Error(error || 'Erreur serveur');
        }

        const options = await startResponse.json();

        // 2. Convertir les options pour WebAuthn
        const publicKeyOptions = {
            challenge: base64urlToBuffer(options.publicKey.challenge),
            timeout: options.publicKey.timeout,
            rpId: options.publicKey.rpId,
            userVerification: options.publicKey.userVerification || 'preferred'
        };

        // Convertir allowCredentials si present
        if (options.publicKey.allowCredentials) {
            publicKeyOptions.allowCredentials = options.publicKey.allowCredentials.map(cred => ({
                type: cred.type,
                id: base64urlToBuffer(cred.id),
                transports: cred.transports
            }));
        }

        // 3. Appeler WebAuthn
        const credential = await navigator.credentials.get({
            publicKey: publicKeyOptions
        });

        if (!credential) {
            throw new Error('Authentification annulee');
        }

        // 4. Preparer la reponse pour le serveur
        const response = {
            id: credential.id,
            rawId: bufferToBase64url(credential.rawId),
            type: credential.type,
            response: {
                clientDataJSON: bufferToBase64url(credential.response.clientDataJSON),
                authenticatorData: bufferToBase64url(credential.response.authenticatorData),
                signature: bufferToBase64url(credential.response.signature),
                userHandle: credential.response.userHandle ?
                    bufferToBase64url(credential.response.userHandle) : null
            }
        };

        // 5. Envoyer au serveur
        const finishResponse = await fetch('/api/passkey/login/finish', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify(response)
        });

        if (!finishResponse.ok) {
            const error = await finishResponse.text();
            throw new Error(error || 'Authentification echouee');
        }

        // 6. Redirection vers le dashboard
        window.location.href = '/';

    } catch (err) {
        console.error('Passkey error:', err);
        if (err.name === 'NotAllowedError') {
            showPasskeyToast('Authentification annulee ou refusee', true);
        } else if (err.name === 'SecurityError') {
            showPasskeyToast('Erreur de securite: verifiez que vous etes sur HTTPS', true);
        } else {
            showPasskeyToast('Erreur: ' + err.message, true);
        }
    }
}

// Enregistrement de Passkey (pour la page settings)
async function registerPasskey() {
    try {
        // 1. Obtenir les options du serveur
        const startResponse = await fetch('/api/passkey/register/start', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include'
        });

        if (!startResponse.ok) {
            const error = await startResponse.text();
            throw new Error(error || 'Erreur serveur');
        }

        const options = await startResponse.json();

        // 2. Convertir les options pour WebAuthn
        const publicKeyOptions = {
            challenge: base64urlToBuffer(options.publicKey.challenge),
            rp: options.publicKey.rp,
            user: {
                id: base64urlToBuffer(options.publicKey.user.id),
                name: options.publicKey.user.name,
                displayName: options.publicKey.user.displayName
            },
            pubKeyCredParams: options.publicKey.pubKeyCredParams,
            timeout: options.publicKey.timeout,
            authenticatorSelection: options.publicKey.authenticatorSelection,
            attestation: options.publicKey.attestation || 'none'
        };

        // Convertir excludeCredentials si present
        if (options.publicKey.excludeCredentials) {
            publicKeyOptions.excludeCredentials = options.publicKey.excludeCredentials.map(cred => ({
                type: cred.type,
                id: base64urlToBuffer(cred.id),
                transports: cred.transports
            }));
        }

        // 3. Appeler WebAuthn
        const credential = await navigator.credentials.create({
            publicKey: publicKeyOptions
        });

        if (!credential) {
            throw new Error('Enregistrement annule');
        }

        // 4. Preparer la reponse pour le serveur
        const response = {
            id: credential.id,
            rawId: bufferToBase64url(credential.rawId),
            type: credential.type,
            response: {
                clientDataJSON: bufferToBase64url(credential.response.clientDataJSON),
                attestationObject: bufferToBase64url(credential.response.attestationObject)
            }
        };

        // 5. Envoyer au serveur
        const finishResponse = await fetch('/api/passkey/register/finish', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify(response)
        });

        if (!finishResponse.ok) {
            const error = await finishResponse.text();
            throw new Error(error || 'Enregistrement echoue');
        }

        // 6. Recharger la page
        window.location.reload();

    } catch (err) {
        console.error('Passkey registration error:', err);
        if (err.name === 'NotAllowedError') {
            showPasskeyToast('Enregistrement annule ou refuse', true);
        } else if (err.name === 'InvalidStateError') {
            showPasskeyToast('Cette Passkey est deja enregistree', true);
        } else {
            showPasskeyToast('Erreur: ' + err.message, true);
        }
    }
}
