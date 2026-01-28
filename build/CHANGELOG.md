# Journal des modifications

Toutes les modifications notables de Pilot Finance sont documentées dans ce fichier.

Le format est basé sur [Keep a Changelog](https://keepachangelog.com/fr/1.0.0/),
et ce projet adhère au [Versionnage Sémantique](https://semver.org/lang/fr/).

## [1.4.0] - 2026-01-28

### Ajouté
- **Suivi automatique de version** : Le processus de build génère automatiquement les informations de version depuis package.json
- **Endpoint health enrichi** (`/api/health`) :
  - Numéro de version précis depuis le build
  - Horodatage de la date de build
  - Uptime formaté de manière lisible (ex: "2j 5h 30m 15s")
  - Métriques de taille de base de données avec sortie formatée
  - Statut de disponibilité du garbage collection
- **Garbage collection manuel** : GC automatique toutes les 60 secondes en mode production
- **Analyseur de bundle** : Ajout de `@next/bundle-analyzer` pour l'inspection du bundle en développement
- **États de chargement** : La page paramètres affiche désormais un spinner de chargement pour éviter le flash de l'UI
- **Chargement différé** : Les composants graphiques (ProjectionChart, BalancePieChart) se chargent à la demande
- **Type de module** : Ajout de `"type": "module"` au package.json pour un support ES module approprié

### Modifié
- **Migration Tailwind CSS** : Migration de v3.4.17 vers v4.1.18 (moteur basé sur Rust)
  - Mise à jour pour utiliser le plugin `@tailwindcss/postcss`
  - Conversion du CSS vers la nouvelle syntaxe `@config` et `@import`
  - Temps de build amélioré de **74%** (15s → 4.7-5.2s)
  - Nombre de packages réduit de **62 packages** (429 → 367)
- **Optimisation de la taille du bundle** : Bundle initial réduit de ~30% grâce au code splitting
- **Amélioration de l'utilisation mémoire** : Consommation RAM réduite de 95% à 53%
- **Navigation améliorée** : Ajout du prefetch pour le lien /accounts pour des transitions plus rapides

### Corrigé
- **Style des curseurs** : Restauration des curseurs pointer pour les boutons et liens (breaking change Tailwind 4)
- **Builds Docker** : Déplacement de `@tailwindcss/postcss` vers les dépendances de production
- **Compatibilité bundle analyzer** : Import conditionnel pour éviter les échecs de build Docker
- **Skeleton loaders des graphiques** : Correction des décalages de mise en page avec des contraintes de hauteur appropriées
- **Sécurité TypeScript** : Ajout de gardes de type appropriés pour les appels `global.gc`
- **Avertissements de module** : Élimination de l'avertissement MODULE_TYPELESS_PACKAGE_JSON

### Performance
- **Temps de build** : 15s → 4.7-5.2s (-74%)
- **Bundle initial** : Réduit de ~30% via lazy loading
- **Utilisation RAM** : 95% → 53% avec GC manuel
- **Dépendances** : 429 → 367 packages (-62)

### Détails techniques
- Node.js s'exécute maintenant avec le flag `--expose-gc` pour le garbage collection manuel
- Informations de version générées automatiquement durant le build via `scripts/generate-version.mjs`
- Composants graphiques séparés en modules chargés à la demande
- Optimisations CSS : styles inline extraits vers globals.css

## [1.3.1] - 2026-01-XX

### Modifié
- Optimisations de performance
- Mises à jour des dépendances

### Corrigé
- Problèmes de types TypeScript dans les callbacks de transactions
- Accès à la propriété balance des comptes

## [1.3.0] - 2026-01-XX

### Ajouté
- Version majeure initiale avec les fonctionnalités de gestion financière de base

---

[1.4.0]: https://github.com/neotoxicfr/pilot-finance/compare/v1.3.1...v1.4.0
[1.3.1]: https://github.com/neotoxicfr/pilot-finance/compare/v1.3.0...v1.3.1
[1.3.0]: https://github.com/neotoxicfr/pilot-finance/releases/tag/v1.3.0
