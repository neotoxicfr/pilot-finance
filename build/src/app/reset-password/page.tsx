'use client'

import { resetPasswordAction } from '@/src/actions';
import { useState, Suspense } from 'react';
import { Lock, AlertCircle } from 'lucide-react';
import { useSearchParams } from 'next/navigation';

function ResetForm() {
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const searchParams = useSearchParams();
  const token = searchParams.get('token');

  const handleSubmit = async (formData: FormData) => {
    setLoading(true);
    setError('');
    const res = await resetPasswordAction(formData);
    if (res?.error) {
        setError(res.error);
        setLoading(false);
    }
  };

  if (!token) return <div className="text-red-500 font-bold text-center">Lien invalide ou expiré.</div>;

  return (
    <div className="dashboard-card bg-background border rounded-2xl p-8 w-full max-w-sm transition-all animate-in zoom-in-95 duration-500">
        <h1 className="text-xl font-bold text-foreground mb-2 text-center">Nouveau mot de passe</h1>
        <p className="text-muted-foreground text-sm text-center mb-6">Choisissez un nouveau mot de passe sécurisé.</p>
        
        <form action={handleSubmit} className="space-y-5">
          <input type="hidden" name="token" value={token} />
          
          <div>
            <label className="text-xs uppercase font-bold text-muted-foreground mb-2 flex items-center gap-1 tracking-wider">
                <Lock size={12}/> Nouveau mot de passe
            </label>
            <input 
                name="password" 
                type="password" 
                required 
                minLength={6} 
                className="w-full bg-background border border-border rounded-xl p-3 text-foreground outline-none focus:border-blue-500 transition-colors placeholder:text-muted-foreground/50" 
                placeholder="••••••••" 
            />
          </div>
          {error && (
            <div className="p-3 bg-red-500/10 border border-red-500/20 rounded-xl flex items-center gap-2 text-red-600 dark:text-red-400 text-xs font-bold animate-in shake">
              <AlertCircle size={16} />
              {error}
            </div>
          )}
          <button disabled={loading} className="w-full bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white font-bold py-3.5 rounded-xl transition-all">
            {loading ? 'Modification...' : "Valider"}
          </button>
        </form>
    </div>
  );
}

export default function ResetPasswordPage() {
    return (
        <div className="min-h-screen bg-accent/20 flex items-center justify-center p-4">
            <Suspense fallback={<div className="text-muted-foreground">Chargement...</div>}>
                <ResetForm />
            </Suspense>
        </div>
    );
}