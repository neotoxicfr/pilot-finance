'use client'

import { resetPasswordAction } from '@/src/actions';
import { useState, Suspense } from 'react';
import { Lock } from 'lucide-react';
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

  if (!token) return <div className="text-red-400 text-center">Lien invalide.</div>;

  return (
    <div className="bg-slate-900 p-8 rounded-2xl border border-slate-800 w-full max-w-sm shadow-2xl">
        <h1 className="text-xl font-bold text-white mb-2 text-center">Nouveau mot de passe</h1>
        <p className="text-slate-500 text-sm text-center mb-6">Choisissez un nouveau mot de passe sécurisé.</p>
        
        <form action={handleSubmit} className="space-y-5">
          <input type="hidden" name="token" value={token} />
          <div>
            <label className="text-xs uppercase font-bold text-slate-500 mb-2 flex items-center gap-1 tracking-wider"><Lock size={12}/> Nouveau mot de passe</label>
            <input name="password" type="password" required minLength={6} className="w-full bg-slate-950 border border-slate-700 rounded-xl p-3 text-white outline-none focus:border-blue-500 transition-colors" placeholder="••••••••" />
          </div>
          
          {error && <div className="text-red-400 text-xs text-center bg-red-900/20 p-3 rounded-xl border border-red-900/50">{error}</div>}

          <button disabled={loading} className="w-full bg-blue-600 hover:bg-blue-500 disabled:opacity-50 text-white font-bold py-3.5 rounded-xl transition-all shadow-lg shadow-blue-900/20">
            {loading ? 'Modification...' : "Valider"}
          </button>
        </form>
    </div>
  );
}

export default function ResetPasswordPage() {
    return (
        <div className="min-h-screen bg-slate-950 flex items-center justify-center p-4">
            <Suspense fallback={<div className="text-slate-500">Chargement...</div>}>
                <ResetForm />
            </Suspense>
        </div>
    );
}