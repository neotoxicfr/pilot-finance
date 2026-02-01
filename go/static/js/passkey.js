// Passkey/WebAuthn JavaScript Handler

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
            alert('Authentification annulee ou refusee');
        } else if (err.name === 'SecurityError') {
            alert('Erreur de securite: verifiez que vous etes sur HTTPS');
        } else {
            alert('Erreur: ' + err.message);
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
            alert('Enregistrement annule ou refuse');
        } else if (err.name === 'InvalidStateError') {
            alert('Cette Passkey est deja enregistree');
        } else {
            alert('Erreur: ' + err.message);
        }
    }
}
