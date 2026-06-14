# US-2 — Résolution de l'ownership du périmètre touché

> **Produit** : Daylight Review — MVP assignation (pilier Équité).
> **Fondation** : service stateless, ownership dérivé de `CODEOWNERS` + groupes,
> stratégie de sélection unique derrière une interface. Cible : GitLab + Teams.

**En tant que** lead d'une équipe,
**je veux** que les reviewers candidats soient déterminés à partir du plan d'ownership
déclaré,
**afin que** la review revienne aux responsables du code touché, sans volontariat.

## Critères d'acceptation

- **Étant donné** les fichiers touchés par une MR, **quand** Daylight résout l'ownership,
  **alors** il identifie l'ensemble des équipes propriétaires d'au moins un fichier touché.
- **Étant donné** qu'une équipe est exprimée comme un groupe, **quand** Daylight résout les
  candidats, **alors** le pool = les membres du groupe récupérés à la source (jamais
  recopiés).
- **Étant donné** qu'un fichier relève de plusieurs équipes, **quand** Daylight résout
  l'ownership, **alors** chaque équipe propriétaire est retenue indépendamment.
- **Étant donné** un fichier touché sans règle d'ownership, **quand** au moins une autre
  équipe est propriétaire sur la MR, **alors** ce fichier est ignoré dans la résolution.
- **Étant donné** une MR dont *aucun* fichier n'a de propriétaire, **quand** Daylight résout
  l'ownership, **alors** aucune assignation n'est posée (no-op, statu quo git).

## Notes techniques (cible)

- GitLab : ownership lu depuis `CODEOWNERS` ; une équipe = une section et/ou un `@groupe` ;
  membres résolus via l'API (groupe → membres). Les sections, analysées séparément, portent
  nativement le cas multi-équipe.
- Fichiers touchés via l'API MR (changes).
- Agnostique : « plan d'ownership déclaré », « équipe propriétaire », « pool de candidats ».

## Dépendances

- Dépend de US-1 (consomme le périmètre détecté). Pré-requis de US-3.
