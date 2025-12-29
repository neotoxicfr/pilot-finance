'use client'

import { Check, X } from 'lucide-react';

interface PasswordFeedbackProps {
  password?: string;
  confirm?: string;
  showMatch?: boolean;
}

export default function PasswordFeedback({ password = '', confirm = '', showMatch = false }: PasswordFeedbackProps) {
  const hasMinLength = password.length >= 8;
  const hasUpper = /[A-Z]/.test(password);
  const hasLower = /[a-z]/.test(password);
  const hasNumber = /[0-9]/.test(password);
  const hasSpecial = /[!@#$%^&*(),.?":{}|<>]/.test(password);

  const criteria = [hasMinLength, hasUpper, hasLower, hasNumber, hasSpecial];
  const validCount = criteria.filter(Boolean).length;
  const strength = (validCount / 5) * 100;

  let strengthColor = 'bg-red-500';
  if (validCount >= 3) strengthColor = 'bg-orange-500';
  if (validCount >= 4) strengthColor = 'bg-yellow-500';
  if (validCount === 5) strengthColor = 'bg-emerald-500';

  const passwordsMatch = password && confirm && password === confirm;

  const Requirement = ({ met, text }: { met: boolean, text: string }) => (
    <div className={`flex items-center gap-1.5 text-xs ${met ? 'text-emerald-500' : 'text-muted-foreground'}`}>
      {met ? <Check size={14} className="text-emerald-500" /> : <X size={14} className="text-muted-foreground opacity-50" />}
      <span>{text}</span>
    </div>
  );

  return (
    <div className="space-y-3 animate-in fade-in slide-in-from-top-1">
      <div className="h-1.5 w-full bg-accent rounded-full overflow-hidden">
        <div className={`h-full ${strengthColor} transition-all duration-500 ease-out`} style={{ width: `${strength}%` }}></div>
      </div>

      <div className="grid grid-cols-2 gap-2">
        <Requirement met={hasMinLength} text="8 caractères min." />
        <Requirement met={hasUpper} text="1 majuscule" />
        <Requirement met={hasLower} text="1 minuscule" />
        <Requirement met={hasNumber} text="1 chiffre" />
        <Requirement met={hasSpecial} text="1 caractère spécial" />
      </div>

      {showMatch && confirm.length > 0 && (
        <div className={`flex items-center gap-1.5 text-xs font-medium ${passwordsMatch ? 'text-emerald-500' : 'text-red-500'}`}>
           {passwordsMatch ? <Check size={14} /> : <X size={14} />}
           {passwordsMatch ? 'Les mots de passe correspondent.' : 'Les mots de passe ne correspondent pas.'}
        </div>
      )}
    </div>
  );
}