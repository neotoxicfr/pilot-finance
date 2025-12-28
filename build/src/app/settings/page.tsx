'use client'

import { useState, useEffect } from 'react';
import { updatePasswordAction, getAllUsers, deleteUser, getRegistrationStatus, generateMfaSecretAction, enableMfaAction, disableMfaAction, getMfaStatus, registerPasskeyStart, registerPasskeyFinish, isPasskeyConfigured, getUserPasskeys, deletePasskey, renamePasskey } from '@/src/actions';
import { Settings, Lock, CheckCircle, Shield, User, Mail, Trash2, Unlock, Lock as LockIcon, Smartphone, Fingerprint, Pencil } from 'lucide-react';
import PasswordFeedback from '@/src/components/PasswordFeedback';
import { startRegistration } from '@simplewebauthn/browser';

export default function SettingsPage() {
  const [pwdError, setPwdError] = useState('');
  const [pwdSuccess, setPwdSuccess] = useState(false);
  const [pwdLoading, setPwdLoading] = useState(false);
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');

  // 2FA STATES
  const [mfaEnabled, setMfaEnabled] = useState(false);
  const [mfaSetupData, setMfaSetupData] = useState<{secret: string, imageUrl: string} | null>(null);
  const [mfaCode, setMfaCode] = useState('');
  const [mfaError, setMfaError] = useState('');
  const [mfaLoading, setMfaLoading] = useState(false);
  
  // PASSKEY STATES
  const [showPasskeys, setShowPasskeys] = useState(false);
  const [passkeyLoading, setPasskeyLoading] = useState(false);
  const [passkeyMsg, setPasskeyMsg] = useState('');
  const [myPasskeys, setMyPasskeys] = useState<any[]>([]);

  const [isAdmin, setIsAdmin] = useState(false);
  const [users, setUsers] = useState<any[]>([]);
  const [isRegisterOpen, setIsRegisterOpen] = useState(false);
  const [adminLoading, setAdminLoading] = useState(true);

  // Fonction pour rafraîchir la liste des passkeys
  const refreshPasskeys = async () => {
      const keys = await getUserPasskeys();
      setMyPasskeys(keys);
  };

  useEffect(() => {
    Promise.all([getAllUsers(), getRegistrationStatus(), getMfaStatus(), isPasskeyConfigured(), getUserPasskeys()])
      .then(([usersData, status, mfaStatus, passkeyConfig, keys]) => {
         setUsers(usersData);
         setIsRegisterOpen(status);
         setMfaEnabled(mfaStatus);
         setShowPasskeys(passkeyConfig);
         setMyPasskeys(keys);
         setIsAdmin(true);
      })
      .catch(() => {
         Promise.all([getMfaStatus(), isPasskeyConfigured(), getUserPasskeys()])
            .then(([mfaStatus, passkeyConfig, keys]) => {
                setMfaEnabled(mfaStatus);
                setShowPasskeys(passkeyConfig);
                setMyPasskeys(keys);
            });
         setIsAdmin(false);
      })
      .finally(() => setAdminLoading(false));
  }, []);

  const handlePwdSubmit = async (formData: FormData) => {
    setPwdLoading(true); setPwdError(''); setPwdSuccess(false);
    const res = await updatePasswordAction(formData);
    if (res?.error) setPwdError(res.error); else { setPwdSuccess(true); (document.getElementById('pwdForm') as HTMLFormElement).reset(); setNewPassword(''); setConfirmPassword(''); }
    setPwdLoading(false);
  };
  const handleStartMfaSetup = async () => { setMfaLoading(true); const data = await generateMfaSecretAction(); setMfaSetupData(data); setMfaLoading(false); };
  const handleConfirmMfa = async () => { if(!mfaSetupData || !mfaCode) return; setMfaLoading(true); setMfaError(''); const res = await enableMfaAction(mfaSetupData.secret, mfaCode); if(res?.error) setMfaError(res.error); else { setMfaEnabled(true); setMfaSetupData(null); setMfaCode(''); } setMfaLoading(false); };
  const handleDisableMfa = async () => { if(confirm("Désactiver l'A2F ?")) { await disableMfaAction(); setMfaEnabled(false); } };
  const handleDeleteUser = async (id: number) => { if (confirm("Supprimer l'utilisateur ?")) { await deleteUser(id); setUsers(users.filter(u => u.id !== id)); } };

  const handleRegisterPasskey = async () => {
      setPasskeyLoading(true); setPasskeyMsg('');
      try {
          const options = await registerPasskeyStart();
          
          // 1. Vérification d'erreur serveur
          if ((options as any).error) {
              setPasskeyMsg("❌ " + (options as any).error);
              setPasskeyLoading(false);
              return;
          }

          // 2. NOUVELLE SYNTAXE ici aussi
          const attResp = await startRegistration({ optionsJSON: options as any });
          
          const verification = await registerPasskeyFinish(attResp);
          if (verification.success) {
              setPasskeyMsg("✅ Passkey ajouté !");
              refreshPasskeys();
          } else {
              setPasskeyMsg("❌ Erreur : " + verification.error);
          }
      } catch (error) { 
          console.error(error); 
          setPasskeyMsg("❌ Erreur lors de l'enregistrement."); 
      }
      setPasskeyLoading(false);
  };

  const handleDeletePasskey = async (id: number) => {
      if(confirm('Supprimer cette clé ?')) {
          await deletePasskey(id);
          refreshPasskeys();
      }
  };

  const handleRenamePasskey = async (id: number, currentName: string) => {
      const newName = prompt("Nom de la clé :", currentName);
      if (newName && newName !== currentName) {
          await renamePasskey(id, newName);
          refreshPasskeys();
      }
  };

  return (
    <main className="w-full flex-1 p-4 md:p-8 max-w-[1600px] mx-auto space-y-8 text-slate-200">
      <div className="flex items-center gap-3 border-b border-slate-800 pb-4">
          <Settings className="text-blue-500" size={32}/> 
          <h1 className="text-2xl font-bold text-white">Paramètres</h1>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
          <div className="space-y-8">
            
            {/* SECTION 2FA */}
            <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-xl">
                <h2 className="text-lg font-bold text-slate-200 mb-6 flex items-center gap-2"><Smartphone size={20} className="text-blue-400"/> Double Authentification (A2F)</h2>
                {mfaEnabled ? (
                    <div className="bg-emerald-900/20 border border-emerald-900/50 rounded-xl p-4 flex items-center justify-between"><div className="flex items-center gap-3 text-emerald-400 font-bold text-sm"><Shield size={24}/> A2F Active</div><button onClick={handleDisableMfa} className="text-xs bg-slate-800 hover:bg-red-900/30 text-slate-400 hover:text-red-400 px-3 py-2 rounded-lg border border-slate-700 hover:border-red-900">Désactiver</button></div>
                ) : (
                    <>
                        {!mfaSetupData ? (
                            <div className="space-y-4"><p className="text-sm text-slate-400">Protégez votre compte avec un code temporaire.</p><button onClick={handleStartMfaSetup} disabled={mfaLoading} className="w-full bg-blue-600 hover:bg-blue-500 text-white font-bold py-3 rounded-xl transition-all shadow-lg text-sm">{mfaLoading ? 'Chargement...' : 'Configurer A2F'}</button></div>
                        ) : (
                            <div className="space-y-5 animate-in fade-in slide-in-from-top-4">
                                <div className="text-center bg-white p-4 rounded-xl w-fit mx-auto"><img src={mfaSetupData.imageUrl} alt="QR Code A2F" className="w-48 h-48" /></div>
                                <div className="text-center text-sm text-slate-400">Scannez ce QR Code.</div>
                                <div><label className="text-xs uppercase font-bold text-slate-500 mb-2 block tracking-wider">Code (6 chiffres)</label><input type="text" inputMode="numeric" maxLength={6} value={mfaCode} onChange={e => setMfaCode(e.target.value)} className="w-full bg-slate-950 border border-slate-700 rounded-xl p-3 text-white outline-none focus:border-blue-500 text-center text-xl tracking-[0.5em] font-mono" placeholder="000000"/></div>
                                {mfaError && <div className="text-red-400 text-xs text-center bg-red-900/20 p-3 rounded-xl border border-red-900/50">{mfaError}</div>}
                                <div className="flex gap-3"><button onClick={() => setMfaSetupData(null)} className="flex-1 bg-slate-800 hover:bg-slate-700 text-slate-300 font-bold py-3 rounded-xl transition-all text-sm">Annuler</button><button onClick={handleConfirmMfa} disabled={mfaLoading || mfaCode.length !== 6} className="flex-1 bg-emerald-600 hover:bg-emerald-500 text-white font-bold py-3 rounded-xl transition-all text-sm shadow-lg disabled:opacity-50">Activer</button></div>
                            </div>
                        )}
                    </>
                )}
            </div>

            {/* SECTION PASSKEYS */}
            {showPasskeys && (
                <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-xl animate-in fade-in slide-in-from-bottom-2">
                    <h2 className="text-lg font-bold text-slate-200 mb-6 flex items-center gap-2"><Fingerprint size={20} className="text-purple-400"/> Passkeys</h2>
                    <p className="text-sm text-slate-400 mb-4">Clés de sécurité enregistrées (FaceID, TouchID...).</p>
                    
                    {/* LISTE DES PASSKEYS */}
                    {myPasskeys.length > 0 && (
                        <div className="space-y-2 mb-6">
                            {myPasskeys.map((key) => (
                                <div key={key.id} className="flex items-center justify-between p-3 bg-slate-950 rounded-xl border border-slate-800">
                                    <div className="flex items-center gap-3">
                                        <div className="p-2 bg-purple-500/10 text-purple-400 rounded-lg"><Fingerprint size={16}/></div>
                                        <div>
                                            <div className="font-bold text-sm text-slate-200">{key.name || 'Clé sans nom'}</div>
                                            <div className="text-[10px] text-slate-500 uppercase">{key.credentialDeviceType === 'singleDevice' ? 'Physique' : 'Appareil'}</div>
                                        </div>
                                    </div>
                                    <div className="flex gap-1">
                                        <button onClick={() => handleRenamePasskey(key.id, key.name)} className="p-2 text-slate-500 hover:text-blue-400 hover:bg-slate-800 rounded-lg transition-colors"><Pencil size={14}/></button>
                                        <button onClick={() => handleDeletePasskey(key.id)} className="p-2 text-slate-500 hover:text-red-400 hover:bg-slate-800 rounded-lg transition-colors"><Trash2 size={14}/></button>
                                    </div>
                                </div>
                            ))}
                        </div>
                    )}

                    {passkeyMsg && <div className="mb-4 text-sm font-bold text-center animate-in fade-in">{passkeyMsg}</div>}

                    <button onClick={handleRegisterPasskey} disabled={passkeyLoading} className="w-full bg-purple-600 hover:bg-purple-500 text-white font-bold py-3 rounded-xl transition-all shadow-lg text-sm flex items-center justify-center gap-2">
                        <Fingerprint size={18}/> {passkeyLoading ? 'Enregistrement...' : 'Ajouter un Passkey'}
                    </button>
                </div>
            )}

            {/* SECTION MOT DE PASSE (Inchangée) */}
            <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-xl h-fit">
                <h2 className="text-lg font-bold text-slate-200 mb-6 flex items-center gap-2"><Lock size={20} className="text-slate-400"/> Mot de passe</h2>
                {pwdSuccess && (<div className="mb-6 p-3 bg-emerald-900/30 border border-emerald-900 rounded-xl flex items-center gap-3 text-emerald-400 text-sm"><CheckCircle size={20} />Mot de passe modifié !</div>)}
                <form id="pwdForm" action={handlePwdSubmit} className="space-y-5">
                    <div><label className="text-xs uppercase font-bold text-slate-500 mb-2 block tracking-wider">Actuel</label><input name="currentPassword" type="password" required className="w-full bg-slate-950 border border-slate-700 rounded-xl p-3 text-white outline-none focus:border-blue-500" /></div>
                    <div><label className="text-xs uppercase font-bold text-slate-500 mb-2 block tracking-wider">Nouveau</label><input name="newPassword" type="password" required value={newPassword} onChange={(e) => setNewPassword(e.target.value)} className="w-full bg-slate-950 border border-slate-700 rounded-xl p-3 text-white outline-none focus:border-blue-500" /></div>
                    <div><label className="text-xs uppercase font-bold text-slate-500 mb-2 block tracking-wider">Confirmation</label><input name="confirmPassword" type="password" required value={confirmPassword} onChange={(e) => setConfirmPassword(e.target.value)} className="w-full bg-slate-950 border border-slate-700 rounded-xl p-3 text-white outline-none focus:border-blue-500" /></div>
                    <PasswordFeedback password={newPassword} confirm={confirmPassword} showMatch={true} />
                    {pwdError && <div className="text-red-400 text-xs text-center bg-red-900/20 p-3 rounded-xl border border-red-900/50">{pwdError}</div>}
                    <button disabled={pwdLoading} className="w-full bg-slate-800 hover:bg-blue-600 hover:text-white text-slate-300 font-bold py-3 rounded-xl transition-all shadow-lg">{pwdLoading ? 'Modification...' : 'Changer'}</button>
                </form>
            </div>
          </div>

          {/* ADMIN */}
          {isAdmin && (
            <div className="space-y-6">
                 <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-xl relative overflow-hidden">
                    <div className={`absolute left-0 top-0 bottom-0 w-1 ${isRegisterOpen ? 'bg-emerald-500' : 'bg-red-500'}`}></div>
                    <div className="flex items-center gap-4">
                        <div className={`p-3 rounded-full ${isRegisterOpen ? 'bg-emerald-500/10 text-emerald-400' : 'bg-red-500/10 text-red-400'}`}>
                        {isRegisterOpen ? <Unlock size={24}/> : <LockIcon size={24}/>}
                        </div>
                        <div>
                            <h2 className="text-lg font-bold text-white">Inscriptions {isRegisterOpen ? "Ouvertes" : "Fermées"}</h2>
                            <p className="text-sm text-slate-500">Variable système : <code>ALLOW_REGISTER={isRegisterOpen ? 'true' : 'false'}</code></p>
                        </div>
                    </div>
                </div>
                 <div className="bg-slate-900 border border-slate-800 rounded-2xl p-6 shadow-xl">
                    <h2 className="text-lg font-bold text-slate-200 mb-6 flex items-center gap-2"><Shield size={20} className="text-purple-400"/> Administration ({users.length})</h2>
                    <div className="overflow-hidden rounded-xl border border-slate-800">
                        <table className="w-full text-left text-sm">
                            <thead className="bg-slate-950 text-slate-500 uppercase font-bold text-xs"><tr><th className="px-4 py-3">Utilisateur</th><th className="hidden sm:table-cell px-4 py-3">Rôle</th><th className="px-4 py-3 text-right">Action</th></tr></thead>
                            <tbody className="divide-y divide-slate-800">
                                {users.map((user) => (
                                    <tr key={user.id} className="hover:bg-slate-800/30 transition-colors">
                                        <td className="px-4 py-3"><div className="flex items-center gap-3"><div className={`w-8 h-8 rounded-full flex items-center justify-center flex-shrink-0 ${user.role === 'ADMIN' ? 'bg-purple-500/20 text-purple-400' : 'bg-blue-500/20 text-blue-400'}`}><Mail size={14} /></div><div className="truncate max-w-[150px] sm:max-w-none"><div className="font-bold text-white truncate">{user.email}</div><div className="sm:hidden text-[10px] text-slate-500 uppercase font-bold mt-0.5">{user.role}</div></div></div></td>
                                        <td className="hidden sm:table-cell px-4 py-3"><span className={`px-2 py-1 rounded text-[10px] font-bold uppercase tracking-wide ${user.role === 'ADMIN' ? 'bg-purple-500/10 text-purple-400 border border-purple-500/20' : 'bg-slate-800 text-slate-400 border border-slate-700'}`}>{user.role}</span></td>
                                        <td className="px-4 py-3 text-right">{user.role !== 'ADMIN' && (<button onClick={() => handleDeleteUser(user.id)} className="p-2 text-slate-500 hover:text-red-400 hover:bg-red-500/10 rounded-lg transition-all" title="Supprimer ce compte"><Trash2 size={16} /></button>)}</td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>
          )}
      </div>
    </main>
  );
}