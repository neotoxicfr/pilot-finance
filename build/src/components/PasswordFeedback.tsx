'use client'

import { Check, X, ShieldCheck, ShieldAlert } from 'lucide-react';

interface Props {
    password: string;
    confirm?: string; // Optionnel
    showMatch?: boolean; // Doit-on vérifier la correspondance ?
}

export default function PasswordFeedback({ password, confirm, showMatch = false }: Props) {
    if (!password) return null;

    const criteria = [
        { label: "8 caractères min.", valid: password.length >= 8 },
        { label: "1 Majuscule", valid: /[A-Z]/.test(password) },
        { label: "1 minuscule", valid: /[a-z]/.test(password) },
        { label: "1 chiffre", valid: /[0-9]/.test(password) },
        { label: "1 car. spécial", valid: /[^A-Za-z0-9]/.test(password) },
    ];

    const isMatch = showMatch && confirm ? password === confirm : false;

    return (
        <div className="mt-3 p-3 bg-slate-950/50 rounded-xl border border-slate-800 text-xs animate-in fade-in slide-in-from-top-1">
            <div className="grid grid-cols-2 gap-2 mb-2">
                {criteria.map((c, i) => (
                    <div key={i} className={`flex items-center gap-1.5 transition-colors ${c.valid ? 'text-emerald-400' : 'text-slate-500'}`}>
                        {c.valid ? <Check size={12} className="stroke-[3]"/> : <div className="w-3 h-3 rounded-full border border-slate-600"></div>}
                        <span>{c.label}</span>
                    </div>
                ))}
            </div>

            {showMatch && (
                <div className={`pt-2 mt-2 border-t border-slate-800 flex items-center gap-2 font-bold ${isMatch ? 'text-emerald-400' : 'text-slate-500'}`}>
                    {isMatch ? <ShieldCheck size={14} /> : <ShieldAlert size={14} />}
                    {confirm === '' ? 'Confirmez le mot de passe' : (isMatch ? 'Les mots de passe correspondent' : 'Les mots de passe ne correspondent pas')}
                </div>
            )}
        </div>
    );
}