# US-1 — Détection automatique d'une MR à router

> **Produit** : Daylight Review — MVP assignation (pilier Équité).
> **Fondation** : service stateless, ownership dérivé de `CODEOWNERS` + groupes,
> stratégie de sélection unique derrière une interface. Cible : GitLab + Teams.

**En tant qu'**auteur d'une merge request,
**je veux** que Daylight détecte ma MR dès son ouverture,
**afin que** son assignation se déclenche sans action de ma part.

## Critères d'acceptation

- **Étant donné** une MR nouvellement ouverte, **quand** l'événement d'ouverture est reçu,
  **alors** le processus d'assignation se déclenche pour cette MR.
- **Étant donné** une MR ouverte dont le périmètre de fichiers change (nouveau push),
  **quand** l'événement de mise à jour est reçu, **alors** Daylight ré-évalue le périmètre
  touché.
- **Étant donné** une MR en brouillon, **quand** l'événement est reçu, **alors** Daylight ne
  déclenche pas l'assignation (en attente de passage en « prête »).
- **Étant donné** un événement déjà traité (re-livraison), **quand** il est reçu une seconde
  fois, **alors** le traitement est idempotent (pas de double assignation).

## Notes techniques (cible)

- GitLab : webhook *Merge request events*, `action ∈ {open, reopen, update}`. `update` ne
  signale pas toujours un changement de périmètre → ne re-résoudre que si la liste de
  fichiers a bougé.
- Idempotence : clé sur `(project_id, mr_iid, sha)`.
- Agnostique : « événement d'ouverture / mise à jour de MR », « périmètre de fichiers
  touchés ».

## Dépendances

- Aucune (point d'entrée). Pré-requis de US-2.
