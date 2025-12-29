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
          if ((options as any).error) {
              setPasskeyMsg("❌ " + (options as any).error);
              setPasskeyLoading(false);
              return;
          }
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
    <main className="w-full flex-1 p-4 md:p-8 max-w-[1600px] mx-auto space-y-8 text-foreground">
      
      {/* EN-TÊTE */}
      <div className="flex items-center gap-3 border-b border-border pb-4">
          <Settings className="text-blue-500" size={32}/> 
          <h1 className="text-2xl font-bold text-foreground">Paramètres</h1>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
          <div className="space-y-8">
            
            {/* SECTION 2FA */}
            <div className="dashboard-card bg-background border rounded-2xl p-6">
                <h2 className="text-lg font-bold text-foreground mb-6 flex items-center gap-2">
                    <Smartphone size={20} className="text-blue-500"/> Double Authentification (A2F)
                </h2>
                
                {mfaEnabled ? (
                    <div className="bg-emerald-500/10 border border-emerald-500/20 rounded-xl p-4 flex items-center justify-between">
                        <div className="flex items-center gap-3 text-emerald-600 dark:text-emerald-400 font-bold text-sm">
                            <Shield size={24}/> A2F Active
                        </div>
                        <button onClick={handleDisableMfa} className="text-xs bg-background hover:bg-red-500/10 text-muted-foreground hover:text-red-500 px-3 py-2 rounded-lg border border-border hover:border-red-500/50 transition-colors">
                            Désactiver
                        </button>
                    </div>
                ) : (
                    <>
                        {!mfaSetupData ? (
                            <div className="space-y-4">
                                <p className="text-sm text-muted-foreground">Protégez votre compte avec un code temporaire (Google Authenticator, Authy...).</p>
                                {/* SUPPRESSION SHADOW */}
                                <button onClick={handleStartMfaSetup} disabled={mfaLoading} className="w-full bg-blue-600 hover:bg-blue-500 text-white font-bold py-3 rounded-xl transition-all text-sm">
                                    {mfaLoading ? 'Chargement...' : 'Configurer A2F'}
                                </button>
                            </div>
                        ) : (
                            <div className="space-y-5 animate-in fade-in slide-in-from-top-4">
                                <div className="text-center bg-white p-4 rounded-xl w-fit mx-auto border border-border">
                                    <img src={mfaSetupData.imageUrl} alt="QR Code A2F" className="w-48 h-48" />
                                </div>
                                <div className="text-center text-sm text-muted-foreground">Scannez ce QR Code avec votre application.</div>
                                <div>
                                    <label className="text-xs uppercase font-bold text-muted-foreground mb-2 block tracking-wider">Code (6 chiffres)</label>
                                    <input type="text" inputMode="numeric" maxLength={6} value={mfaCode} onChange={e => setMfaCode(e.target.value)} className="w-full bg-accent/50 border border-border rounded-xl p-3 text-foreground outline-none focus:border-blue-500 text-center text-xl tracking-[0.5em] font-mono transition-colors" placeholder="000000"/>
                                </div>
                                {mfaError && <div className="text-red-500 text-xs text-center bg-red-500/10 p-3 rounded-xl border border-red-500/20">{mfaError}</div>}
                                <div className="flex gap-3">
                                    <button onClick={() => setMfaSetupData(null)} className="flex-1 bg-accent hover:bg-accent/80 text-muted-foreground font-bold py-3 rounded-xl transition-all text-sm">Annuler</button>
                                    {/* SUPPRESSION SHADOW */}
                                    <button onClick={handleConfirmMfa} disabled={mfaLoading || mfaCode.length !== 6} className="flex-1 bg-emerald-600 hover:bg-emerald-500 text-white font-bold py-3 rounded-xl transition-all text-sm disabled:opacity-50">Activer</button>
                                </div>
                            </div>
                        )}
                    </>
                )}
            </div>

            {/* SECTION PASSKEYS */}
            {showPasskeys && (
                <div className="dashboard-card bg-background border rounded-2xl p-6">
                    <h2 className="text-lg font-bold text-foreground mb-6 flex items-center gap-2">
                        <Fingerprint size={20} className="text-purple-500"/> Passkeys
                    </h2>
                    <p className="text-sm text-muted-foreground mb-4">Clés de sécurité enregistrées (FaceID, TouchID, YubiKey...).</p>
                    
                    {/* LISTE DES PASSKEYS */}
                    {myPasskeys.length > 0 && (
                        <div className="space-y-2 mb-6">
                            {myPasskeys.map((key) => (
                                <div key={key.id} className="flex items-center justify-between p-3 bg-accent/30 rounded-xl border border-border">
                                    <div className="flex items-center gap-3">
                                        <div className="p-2 bg-purple-500/10 text-purple-500 rounded-lg"><Fingerprint size={16}/></div>
                                        <div>
                                            <div className="font-bold text-sm text-foreground">{key.name || 'Clé sans nom'}</div>
                                            <div className="text-[10px] text-muted-foreground uppercase font-bold">{key.credentialDeviceType === 'singleDevice' ? 'Physique' : 'Appareil'}</div>
                                        </div>
                                    </div>
                                    <div className="flex gap-1">
                                        <button onClick={() => handleRenamePasskey(key.id, key.name)} className="p-2 text-muted-foreground hover:text-blue-500 hover:bg-background rounded-lg transition-colors"><Pencil size={14}/></button>
                                        <button onClick={() => handleDeletePasskey(key.id)} className="p-2 text-muted-foreground hover:text-red-500 hover:bg-background rounded-lg transition-colors"><Trash2 size={14}/></button>
                                    </div>
                                </div>
                            ))}
                        </div>
                    )}

                    {passkeyMsg && <div className="mb-4 text-sm font-bold text-center animate-in fade-in text-foreground">{passkeyMsg}</div>}

                    {/* SUPPRESSION SHADOW */}
                    <button onClick={handleRegisterPasskey} disabled={passkeyLoading} className="w-full bg-purple-600 hover:bg-purple-500 text-white font-bold py-3 rounded-xl transition-all text-sm flex items-center justify-center gap-2">
                        <Fingerprint size={18}/> {passkeyLoading ? 'Enregistrement...' : 'Ajouter un Passkey'}
                    </button>
                </div>
            )}

            {/* SECTION MOT DE PASSE */}
            <div className="dashboard-card bg-background border rounded-2xl p-6 h-fit">
                <h2 className="text-lg font-bold text-foreground mb-6 flex items-center gap-2">
                    <Lock size={20} className="text-muted-foreground"/> Mot de passe
                </h2>
                {pwdSuccess && (<div className="mb-6 p-3 bg-emerald-500/10 border border-emerald-500/20 rounded-xl flex items-center gap-3 text-emerald-600 dark:text-emerald-400 text-sm"><CheckCircle size={20} />Mot de passe modifié !</div>)}
                <form id="pwdForm" action={handlePwdSubmit} className="space-y-5">
                    <div>
                        <label className="text-xs uppercase font-bold text-muted-foreground mb-2 block tracking-wider">Actuel</label>
                        {/* CHANGEMENT bg-accent/50 -> bg-background */}
                        <input name="currentPassword" type="password" required className="w-full bg-background border border-border rounded-xl p-3 text-foreground outline-none focus:border-blue-500 transition-colors" />
                    </div>
                    <div>
                        <label className="text-xs uppercase font-bold text-muted-foreground mb-2 block tracking-wider">Nouveau</label>
                        {/* CHANGEMENT bg-accent/50 -> bg-background */}
                        <input name="newPassword" type="password" required value={newPassword} onChange={(e) => setNewPassword(e.target.value)} className="w-full bg-background border border-border rounded-xl p-3 text-foreground outline-none focus:border-blue-500 transition-colors" />
                    </div>
                    <div>
                        <label className="text-xs uppercase font-bold text-muted-foreground mb-2 block tracking-wider">Confirmation</label>
                        {/* CHANGEMENT bg-accent/50 -> bg-background */}
                        <input name="confirmPassword" type="password" required value={confirmPassword} onChange={(e) => setConfirmPassword(e.target.value)} className="w-full bg-background border border-border rounded-xl p-3 text-foreground outline-none focus:border-blue-500 transition-colors" />
                    </div>
 
                    <PasswordFeedback password={newPassword} confirm={confirmPassword} showMatch={true} />
                    
                    {pwdError && <div className="text-red-500 text-xs text-center bg-red-500/10 p-3 rounded-xl border border-red-500/20">{pwdError}</div>}
                    
                    {/* CHANGEMENT STYLE BOUTON + SUPPRESSION SHADOW */}
                    <button disabled={pwdLoading} className="w-full bg-blue-600 hover:bg-blue-500 text-white font-bold py-3 rounded-xl transition-all">
                        {pwdLoading ? 'Modification...' : 'Changer le mot de passe'}
                    </button>
                </form>
            </div>
          </div>

          {/* ADMIN */}
          {isAdmin && (
            <div className="space-y-6">
                 <div className="dashboard-card bg-background border rounded-2xl p-6 relative overflow-hidden">
                    <div className={`absolute left-0 top-0 bottom-0 w-1 ${isRegisterOpen ? 'bg-emerald-500' : 'bg-red-500'}`}></div>
                    <div className="flex items-center gap-4">
                        <div className={`p-3 rounded-full ${isRegisterOpen ? 'bg-emerald-500/10 text-emerald-500' : 'bg-red-500/10 text-red-500'}`}>
                            {isRegisterOpen ? <Unlock size={24}/> : <LockIcon size={24}/>}
                        </div>
                        <div>
                            <h2 className="text-lg font-bold text-foreground">Inscriptions {isRegisterOpen ? "Ouvertes" : "Fermées"}</h2>
                            <p className="text-sm text-muted-foreground">Variable système : <code>ALLOW_REGISTER={isRegisterOpen ? 'true' : 'false'}</code></p>
                        </div>
                    </div>
                </div>
                 <div className="dashboard-card bg-background border rounded-2xl p-6">
                    <h2 className="text-lg font-bold text-foreground mb-6 flex items-center gap-2">
                        <Shield size={20} className="text-purple-500"/> Administration ({users.length})
                    </h2>
                    <div className="overflow-hidden rounded-xl border border-border">
                        <table className="w-full text-left text-sm">
                            <thead className="bg-accent/50 text-muted-foreground uppercase font-bold text-xs">
                                <tr>
                                    <th className="px-4 py-3">Utilisateur</th>
                                    <th className="hidden sm:table-cell px-4 py-3">Rôle</th>
                                    <th className="px-4 py-3 text-right">Action</th>
                                </tr>
                            </thead>
                            <tbody className="divide-y divide-border">
                                {users.map((user) => (
                                    <tr key={user.id} className="hover:bg-accent/50 transition-colors">
                                        <td className="px-4 py-3">
                                            <div className="flex items-center gap-3">
                                                <div className={`w-8 h-8 rounded-full flex items-center justify-center flex-shrink-0 ${user.role === 'ADMIN' ? 'bg-purple-500/10 text-purple-500' : 'bg-blue-500/10 text-blue-500'}`}>
                                                    <Mail size={14} />
                                                </div>
                                                <div className="truncate max-w-[150px] sm:max-w-none">
                                                    <div className="font-bold text-foreground truncate">{user.email}</div>
                                                    <div className="sm:hidden text-[10px] text-muted-foreground uppercase font-bold mt-0.5">{user.role}</div>
                                                </div>
                                            </div>
                                        </td>
                                        <td className="hidden sm:table-cell px-4 py-3">
                                            <span className={`px-2 py-1 rounded text-[10px] font-bold uppercase tracking-wide ${user.role === 'ADMIN' ? 'bg-purple-500/10 text-purple-500 border border-purple-500/20' : 'bg-accent text-muted-foreground border border-border'}`}>
                                                {user.role}
                                            </span>
                                        </td>
                                        <td className="px-4 py-3 text-right">
                                            {user.role !== 'ADMIN' && (
                                                <button onClick={() => handleDeleteUser(user.id)} className="p-2 text-muted-foreground hover:text-red-500 hover:bg-red-500/10 rounded-lg transition-all" title="Supprimer ce compte">
                                                    <Trash2 size={16} />
                                                </button>
                                            )}
                                        </td>
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