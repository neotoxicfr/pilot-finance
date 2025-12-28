'use client'

import { authenticate, getRegistrationStatus, loginPasskeyStart, loginPasskeyFinish, isPasskeyConfigured, getMailStatus } from '@/src/actions';
import { useState, useEffect, Suspense } from 'react';
import { Wallet, Mail, Lock, Check, Shield, Fingerprint } from 'lucide-react';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import PasswordFeedback from '@/src/components/PasswordFeedback';
import { startAuthentication } from '@simplewebauthn/browser';

function LoginForm() {
  const [isRegister, setIsRegister] = useState(false);
  const [canRegister, setCanRegister] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [loading, setLoading] = useState(false);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [requires2FA, setRequires2FA] = useState(false);
  const [twoFactorCode, setTwoFactorCode] = useState('');
  const [canUsePasskeys, setCanUsePasskeys] = useState(false);
  const [mailEnabled, setMailEnabled] = useState(false);

  const searchParams = useSearchParams();
  const resetSuccess = searchParams.get('reset') === 'success';

  useEffect(() => {
    getRegistrationStatus().then(status => setCanRegister(status));
    isPasskeyConfigured().then(status => setCanUsePasskeys(status));
    getMailStatus().then(status => setMailEnabled(status));
  }, []);

  const handleSubmit = async (formData: FormData) => {
    setLoading(true); setError(''); setSuccess('');
    if (requires2FA) formData.set('twoFactorCode', twoFactorCode);
    const res = await authenticate(formData, isRegister);
    
    if (res?.error) { 
        setError(res.error); 
        setLoading(false); 
    } else if (res?.message) {
        setSuccess(res.message);
        setLoading(false);
        setIsRegister(false);
    } else if (res?.requires2FA) { 
        setRequires2FA(true); 
        setLoading(false); 
        setError(''); 
    }
  };

  const handlePasskeyLogin = async () => {
    try {
      const options = await loginPasskeyStart();
      if ((options as any).error) {
        setError((options as any).error);
        return;
      }
      const asseResp = await startAuthentication({ optionsJSON: options as any });
      const verification = await loginPasskeyFinish(asseResp);
      if (verification.success) {
        window.location.href = "/";
      } else {
        setError(verification.error || "Erreur de vérification");
      }
    } catch (error) {
      console.error(error);
      setError("Échec de l'authentification par clé.");
    }
  };

  const toggleMode = () => { setIsRegister(!isRegister); setError(''); setSuccess(''); setPassword(''); setConfirmPassword(''); setRequires2FA(false); };

  return (
    <div className="bg-slate-900 p-8 rounded-2xl border border-slate-800 w-full max-w-sm shadow-2xl transition-all">
        <div className="flex justify-center mb-6"><div className="bg-blue-600/20 p-4 rounded-2xl"><Wallet className="text-blue-500 w-10 h-10" /></div></div>
        <h1 className="text-2xl font-bold text-white mb-2 text-center">Pilot Finance</h1>
        <p className="text-slate-500 text-sm text-center mb-8">Votre cockpit financier personnel</p>

        {resetSuccess && (<div className="mb-6 p-3 bg-emerald-900/30 border border-emerald-900 rounded-xl text-center text-emerald-400 text-sm">Mot de passe réinitialisé !</div>)}
        {success && (<div className="mb-6 p-3 bg-blue-900/30 border border-blue-900 rounded-xl text-center text-blue-400 text-sm">{success}</div>)}
        
        {!isRegister && !requires2FA && canUsePasskeys && (
            <button onClick={handlePasskeyLogin} className="w-full bg-slate-800 hover:bg-slate-700 text-white font-bold py-3 rounded-xl mb-6 flex items-center justify-center gap-2 border border-slate-700 transition-all">
                <Fingerprint size={20} className="text-purple-400"/> Se connecter avec Passkey
            </button>
        )}
        
        {!isRegister && !requires2FA && canUsePasskeys && (
            <div className="relative flex py-2 items-center mb-6">
                <div className="flex-grow border-t border-slate-700"></div>
                <span className="flex-shrink-0 mx-4 text-slate-500 text-xs uppercase font-bold">Ou via email</span>
                <div className="flex-grow border-t border-slate-700"></div>
            </div>
        )}

        <form action={handleSubmit} className="space-y-4">
          <div className={requires2FA ? 'hidden' : ''}>
            <label className="text-xs uppercase font-bold text-slate-500 mb-2 flex items-center gap-1 tracking-wider"><Mail size={12}/> Email</label>
            <input name="email" type="email" required value={email} onChange={e => setEmail(e.target.value)} className="w-full bg-slate-950 border border-slate-700 rounded-xl p-3 text-white outline-none focus:border-blue-500" placeholder="nom@exemple.com" />
          </div>
          
          <div className={requires2FA ? 'hidden' : ''}>
            <div className="flex justify-between items-center mb-2">
                <label className="text-xs uppercase font-bold text-slate-500 block tracking-wider flex items-center gap-1"><Lock size={12}/> Mot de passe</label>
                {!isRegister && mailEnabled && (
                    <Link href="/forgot-password" title="Réinitialiser" className="text-[10px] text-blue-400 hover:text-blue-300">Oublié ?</Link>
                )}
            </div>
            <input name="password" type="password" required value={password} onChange={e => setPassword(e.target.value)} className="w-full bg-slate-950 border border-slate-700 rounded-xl p-3 text-white outline-none focus:border-blue-500" placeholder="••••••••" />
          </div>

          {isRegister && !requires2FA && (
              <div className="animate-in slide-in-from-top-2 fade-in">
                <label className="text-xs uppercase font-bold text-slate-500 mb-2 block tracking-wider flex items-center gap-1"><Check size={12}/> Confirmation</label>
                <input name="confirmPassword" type="password" required value={confirmPassword} onChange={e => setConfirmPassword(e.target.value)} className={`w-full bg-slate-950 border rounded-xl p-3 text-white outline-none transition-colors ${confirmPassword && confirmPassword === password ? 'border-emerald-500/50 focus:border-emerald-500' : 'border-slate-700 focus:border-blue-500'}`} placeholder="••••••••" />
                <PasswordFeedback password={password} confirm={confirmPassword} showMatch={true} />
              </div>
          )}

          {requires2FA && (
              <div className="animate-in zoom-in fade-in bg-blue-900/10 p-4 rounded-xl border border-blue-500/30 mt-4">
                  <div className="flex items-center gap-2 text-blue-400 font-bold text-sm mb-3"><Shield size={18}/> Code Authenticator</div>
                  <input name="twoFactorCode" type="text" inputMode="numeric" maxLength={6} autoFocus required value={twoFactorCode} onChange={e => setTwoFactorCode(e.target.value)} className="w-full bg-slate-950 border border-blue-500 rounded-xl p-3 text-white outline-none text-center text-xl tracking-[0.5em] font-mono shadow-[0_0_15px_rgba(59,130,246,0.2)]" placeholder="000000" />
                  <p className="text-[10px] text-slate-400 text-center mt-2">Entrez le code à 6 chiffres.</p>
              </div>
          )}
          
          {error && <div className="text-red-400 text-xs text-center bg-red-900/20 p-3 rounded-xl border border-red-900/50 animate-in shake">{error}</div>}

          <button disabled={loading} className="w-full bg-blue-600 hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed text-white font-bold py-3.5 rounded-xl transition-all shadow-lg shadow-blue-900/20 mt-2">
            {loading ? 'Chargement...' : (requires2FA ? "Vérifier" : (isRegister ? "Créer un compte" : "Se connecter"))}
          </button>
        </form>

        {canRegister && !requires2FA && (
            <div className="mt-6 pt-6 border-t border-slate-800 text-center">
              <button onClick={toggleMode} className="text-xs text-slate-500 hover:text-white transition-colors">{isRegister ? "J'ai déjà un compte ? Se connecter" : "Premier lancement ? Créer un compte"}</button>
            </div>
        )}
    </div>
  );
}

export default function LoginPage() {
    return (<div className="min-h-screen bg-slate-950 flex items-center justify-center p-4"><Suspense fallback={<div className="text-slate-500">Chargement...</div>}><LoginForm /></Suspense></div>);
}