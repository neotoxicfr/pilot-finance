'use client'
import { authenticate, getRegistrationStatus, loginPasskeyStart, loginPasskeyFinish, isPasskeyConfigured, getMailStatus } from '@/src/actions';
import { useState, useEffect, Suspense } from 'react';
import { Wallet, Mail, Lock, Check, Shield, Fingerprint, AlertCircle, ArrowRight } from 'lucide-react';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import PasswordFeedback from '@/src/components/PasswordFeedback';
import { startAuthentication } from '@simplewebauthn/browser';
import BrandLogo from "@/src/components/BrandLogo";
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
      const verification = await loginPasskeyFinish(asseResp as any);
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
    <div className="dashboard-card bg-background border rounded-2xl p-8 w-full max-w-sm transition-all animate-in zoom-in-95 duration-500">
        <div className="flex flex-col items-center mb-8">
            <div className="bg-background border border-border p-4 rounded-full mb-4">
                <BrandLogo size={42} />
            </div>
            <h1 className="text-2xl font-bold text-foreground mb-1 text-center">Pilot Finance</h1>
            <p className="text-muted-foreground text-sm text-center">Votre cockpit financier personnel</p>
        </div>
        {resetSuccess && (<div className="mb-6 p-3 bg-emerald-500/10 border border-emerald-500/20 rounded-xl text-center text-emerald-600 dark:text-emerald-400 text-sm font-bold">Mot de passe réinitialisé !</div>)}
        {success && (<div className="mb-6 p-3 bg-blue-500/10 border border-blue-500/20 rounded-xl text-center text-blue-600 dark:text-blue-400 text-sm font-bold">{success}</div>)}
        {!isRegister && !requires2FA && canUsePasskeys && (
            <button onClick={handlePasskeyLogin} className="w-full bg-purple-600 hover:bg-purple-500 text-white font-bold py-3 rounded-xl mb-6 flex items-center justify-center gap-2 transition-all">
                <Fingerprint size={20} /> Se connecter avec Passkey
            </button>
        )}
        {!isRegister && !requires2FA && canUsePasskeys && (
            <div className="relative flex py-2 items-center mb-6">
                <div className="flex-grow border-t border-border"></div>
                <span className="flex-shrink-0 mx-4 text-muted-foreground text-xs uppercase font-bold">Ou via email</span>
                <div className="flex-grow border-t border-border"></div>
            </div>
        )}
        <form action={handleSubmit} className="space-y-4">
          <div className={requires2FA ? 'hidden' : ''}>
            <div className="relative group">
               <Mail className="absolute left-3 top-3 text-muted-foreground group-focus-within:text-blue-500 transition-colors" size={18} />
               <input name="email" type="email" required value={email} onChange={e => setEmail(e.target.value)} className="w-full bg-background border border-border rounded-xl py-2.5 pl-10 pr-4 text-sm text-foreground outline-none focus:border-blue-500 transition-colors placeholder:text-muted-foreground/50" placeholder="Email" />
            </div>
          </div>
          <div className={requires2FA ? 'hidden' : ''}>
            <div className="flex justify-between items-center mb-2 px-1">
                <label className="hidden text-xs uppercase font-bold text-muted-foreground tracking-wider items-center gap-1">Mot de passe</label>
                {!isRegister && mailEnabled && (
                    <Link href="/forgot-password" title="Réinitialiser" className="text-[10px] ml-auto text-blue-500 hover:text-blue-400 font-medium">Mot de passe oublié ?</Link>
                )}
            </div>
            <div className="relative group">
                <Lock className="absolute left-3 top-3 text-muted-foreground group-focus-within:text-blue-500 transition-colors" size={18} />
                <input name="password" type="password" required value={password} onChange={e => setPassword(e.target.value)} className="w-full bg-background border border-border rounded-xl py-2.5 pl-10 pr-4 text-sm text-foreground outline-none focus:border-blue-500 transition-colors placeholder:text-muted-foreground/50" placeholder="Mot de passe" />
            </div>
          </div>
          {isRegister && !requires2FA && (
              <div className="animate-in slide-in-from-top-2 fade-in space-y-4">
                <div className="relative group">
                    <Check className="absolute left-3 top-3 text-muted-foreground group-focus-within:text-blue-500 transition-colors" size={18} />
                    <input name="confirmPassword" type="password" required value={confirmPassword} onChange={e => setConfirmPassword(e.target.value)} className={`w-full bg-background border rounded-xl py-2.5 pl-10 pr-4 text-sm text-foreground outline-none transition-colors placeholder:text-muted-foreground/50 ${confirmPassword && confirmPassword === password ? 'border-emerald-500/50 focus:border-emerald-500' : 'border-border focus:border-blue-500'}`} placeholder="Confirmation" />
                </div>
                <PasswordFeedback password={password} confirm={confirmPassword} showMatch={true} />
              </div>
          )}
          {requires2FA && (
              <div className="animate-in zoom-in fade-in bg-blue-500/5 p-4 rounded-xl border border-blue-500/20 mt-4">
                  <div className="flex items-center gap-2 text-blue-500 font-bold text-sm mb-3"><Shield size={18}/> Code Authenticator</div>
                  <input name="twoFactorCode" type="text" inputMode="numeric" maxLength={6} autoFocus required value={twoFactorCode} onChange={e => setTwoFactorCode(e.target.value)} className="w-full bg-background border border-border rounded-xl p-3 text-foreground outline-none text-center text-xl tracking-[0.5em] font-mono focus:border-blue-500 transition-colors" placeholder="000000" />
                  <p className="text-[10px] text-muted-foreground text-center mt-2">Entrez le code à 6 chiffres.</p>
              </div>
          )}
          {error && <div className="text-red-600 dark:text-red-400 text-xs text-center bg-red-500/10 p-3 rounded-xl border border-red-500/20 animate-in shake font-bold flex items-center justify-center gap-2"><AlertCircle size={16}/> {error}</div>}
          <button disabled={loading} className="w-full bg-blue-600 hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed text-white font-bold py-3 rounded-xl transition-all mt-2 flex items-center justify-center gap-2">
            {loading ? 'Chargement...' : (requires2FA ? "Vérifier" : (isRegister ? "Créer un compte" : "Se connecter"))}
            {!loading && <ArrowRight size={18} />}
          </button>
        </form>
        {canRegister && !requires2FA && (
            <div className="mt-6 pt-6 border-t border-border text-center">
              <button onClick={toggleMode} className="text-xs text-muted-foreground hover:text-foreground transition-colors font-medium">
                  {isRegister ? "J'ai déjà un compte ? Se connecter" : "Pas encore de compte ? S'inscrire"}
              </button>
            </div>
        )}
    </div>
  );
}
export default function LoginPage() {
    return (<div className="min-h-screen bg-accent/20 flex flex-col items-center justify-center p-4"><Suspense fallback={<div className="text-muted-foreground">Chargement...</div>}><LoginForm /></Suspense><div className="mt-8 text-center text-[10px] text-muted-foreground uppercase tracking-widest opacity-50">Sécurisé par chiffrement AES-256</div></div>);
}