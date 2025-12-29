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
    <div className="min-h-screen bg-accent/20 flex items-center justify-center p-4">
      <div className="dashboard-card bg-background border rounded-2xl p-8 w-full max-w-sm">
        <div className="flex items-center gap-2 mb-6">
            <Link href="/login" className="p-2 -ml-2 text-muted-foreground hover:text-foreground hover:bg-accent rounded-full transition-colors">
                <ArrowLeft size={20}/>
            </Link>
            <h1 className="text-xl font-bold text-foreground">Mot de passe oublié</h1>
        </div>
        
        {success ? (
            <div className="text-center py-4 animate-in fade-in zoom-in">
                <div className="mx-auto w-16 h-16 bg-emerald-500/10 rounded-full flex items-center justify-center text-emerald-500 mb-4 border border-emerald-500/20">
                    <CheckCircle size={32} />
                </div>
                <h3 className="text-foreground font-bold mb-2">Email envoyé !</h3>
                <p className="text-muted-foreground text-sm mb-6">Si ce compte existe, un lien de réinitialisation vous a été envoyé.</p>
                <Link href="/login" className="block w-full bg-blue-600 hover:bg-blue-500 text-white font-bold py-3 rounded-xl transition-colors text-sm">
                    Retour à la connexion
                </Link>
            </div>
        ) : (
            <>
                <p className="text-muted-foreground text-sm mb-6">Entrez votre email pour recevoir un lien de réinitialisation.</p>
                <form action={handleSubmit} className="space-y-5">
                <div>
                    <label className="text-xs uppercase font-bold text-muted-foreground mb-2 flex items-center gap-1 tracking-wider">
                        <Mail size={12}/> Email
                    </label>
                    <input 
                        name="email" 
                        type="email" 
                        required 
                        className="w-full bg-background border border-border rounded-xl p-3 text-foreground outline-none focus:border-blue-500 transition-colors placeholder:text-muted-foreground/50" 
                        placeholder="nom@exemple.com" 
                    />
                </div>
                <button disabled={loading} className="w-full bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white font-bold py-3.5 rounded-xl transition-all">
                    {loading ? 'Envoi...' : "Envoyer le lien"}
                </button>
                </form>
            </>
        )}
      </div>
    </div>
  );
}