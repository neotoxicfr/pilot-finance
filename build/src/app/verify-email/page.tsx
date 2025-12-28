'use client'

import { useEffect, useState, Suspense } from 'react';
import { useSearchParams, useRouter } from 'next/navigation';
import { verifyEmailAction } from '@/src/actions';
import { CheckCircle, XCircle, Loader2, Wallet } from 'lucide-react';

function VerifyContent() {
    const searchParams = useSearchParams();
    const router = useRouter();
    const [status, setStatus] = useState<'loading' | 'success' | 'error'>('loading');
    const [message, setMessage] = useState('');

    useEffect(() => {
        const token = searchParams.get('token');
        if (!token) {
            setStatus('error');
            setMessage('Jeton manquant.');
            return;
        }

        verifyEmailAction(token).then(res => {
            if (res.success) {
                setStatus('success');
                setTimeout(() => router.push('/login'), 3000);
            } else {
                setStatus('error');
                setMessage(res.error || 'Erreur inconnue');
            }
        });
    }, [searchParams, router]);

    return (
        <div className="bg-slate-900 p-8 rounded-2xl border border-slate-800 w-full max-w-sm shadow-2xl text-center">
            <div className="flex justify-center mb-6">
                <div className="bg-blue-600/20 p-4 rounded-2xl">
                    <Wallet className="text-blue-500 w-10 h-10" />
                </div>
            </div>
            
            {status === 'loading' && (
                <div className="flex flex-col items-center gap-4">
                    <Loader2 className="w-12 h-12 text-blue-500 animate-spin" />
                    <p className="text-slate-300">Validation de votre compte...</p>
                </div>
            )}

            {status === 'success' && (
                <div className="flex flex-col items-center gap-4 animate-in zoom-in">
                    <CheckCircle className="w-12 h-12 text-emerald-500" />
                    <h2 className="text-xl font-bold text-white">Compte validé !</h2>
                    <p className="text-slate-400 text-sm">Redirection vers la connexion...</p>
                </div>
            )}

            {status === 'error' && (
                <div className="flex flex-col items-center gap-4 animate-in shake">
                    <XCircle className="w-12 h-12 text-red-500" />
                    <h2 className="text-xl font-bold text-white">Échec de validation</h2>
                    <p className="text-red-400 text-sm">{message}</p>
                    <button onClick={() => router.push('/login')} className="mt-4 text-blue-400 hover:underline text-sm">Retour à la connexion</button>
                </div>
            )}
        </div>
    );
}

export default function VerifyEmailPage() {
    return (
        <div className="min-h-screen bg-slate-950 flex items-center justify-center p-4">
            <Suspense fallback={<div>Chargement...</div>}>
                <VerifyContent />
            </Suspense>
        </div>
    );
}