'use client'

import { forgotPasswordAction } from '@/src/actions';
import { useState } from 'react';
import { Mail, ArrowLeft, CheckCircle } from 'lucide-react';
import Link from 'next/link';

export default function ForgotPasswordPage() {
  const [success, setSuccess] = useState(false);
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (formData: FormData) => {
    setLoading(true);
    await forgotPasswordAction(formData);
    setLoading(false);
    setSuccess(true);
  };

  return (
    <div className="min-h-screen bg-slate-950 flex items-center justify-center p-4">
      <div className="bg-slate-900 p-8 rounded-2xl border border-slate-800 w-full max-w-sm shadow-2xl">
        <div className="flex items-center gap-2 mb-6">
            <Link href="/login" className="p-2 -ml-2 text-slate-400 hover:text-white hover:bg-slate-800 rounded-full transition-colors"><ArrowLeft size={20}/></Link>
            <h1 className="text-xl font-bold text-white">Mot de passe oublié</h1>
        </div>
        
        {success ? (
            <div className="text-center py-8 animate-in fade-in zoom-in">
                <div className="mx-auto w-16 h-16 bg-emerald-500/10 rounded-full flex items-center justify-center text-emerald-500 mb-4">
                    <CheckCircle size={32} />
                </div>
                <h3 className="text-white font-bold mb-2">Email envoyé !</h3>
                <p className="text-slate-400 text-sm mb-6">Si ce compte existe, un lien de réinitialisation vous a été envoyé.</p>
                <Link href="/login" className="block w-full bg-slate-800 hover:bg-slate-700 text-white font-bold py-3 rounded-xl transition-colors">Retour à la connexion</Link>
            </div>
        ) : (
            <>
                <p className="text-slate-500 text-sm mb-6">Entrez votre email pour recevoir un lien de réinitialisation.</p>
                <form action={handleSubmit} className="space-y-5">
                <div>
                    <label className="text-xs uppercase font-bold text-slate-500 mb-2 flex items-center gap-1 tracking-wider"><Mail size={12}/> Email</label>
                    <input name="email" type="email" required className="w-full bg-slate-950 border border-slate-700 rounded-xl p-3 text-white outline-none focus:border-blue-500 transition-colors" placeholder="nom@exemple.com" />
                </div>
                <button disabled={loading} className="w-full bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white font-bold py-3.5 rounded-xl transition-all shadow-lg shadow-blue-900/20">
                    {loading ? 'Envoi...' : "Envoyer le lien"}
                </button>
                </form>
            </>
        )}
      </div>
    </div>
  );
}