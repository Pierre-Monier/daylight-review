# US-3 — Sélection des reviewers

> **Produit** : Daylight Review — MVP assignation (pilier Équité).
> **Fondation** : service stateless, ownership dérivé de `CODEOWNERS` + groupes,
> stratégie de sélection unique derrière une interface. Cible : GitLab + Teams.

**En tant qu'**auteur d'une MR,
**je veux** qu'un reviewer soit choisi automatiquement dans chaque équipe propriétaire,
**afin d'**obtenir une review sans solliciter personne.

## Critères d'acceptation

- **Étant donné** le pool de candidats d'une équipe propriétaire, **quand** Daylight
  sélectionne, **alors** il choisit un reviewer selon la stratégie active (MVP : aléatoire),
  en **excluant l'auteur**.
- **Étant donné** une MR touchant N équipes propriétaires, **quand** Daylight sélectionne,
  **alors** il choisit **un reviewer par équipe**.
- **Étant donné** une même personne candidate pour plusieurs équipes touchées, **quand** elle
  est sélectionnée plusieurs fois, **alors** elle n'est assignée qu'**une fois** (dédup —
  principe « une charge, un plan »).
- **Étant donné** un pool réduit à l'auteur seul, ou vide, pour une équipe, **quand** Daylight
  sélectionne, **alors** aucun reviewer n'est retenu pour cette équipe, sans bloquer la MR.

## Notes techniques

- Sélection isolée derrière `ReviewerSelectionStrategy.select(candidats, contexte)`, unique
  implémentation `RandomStrategy`, pure et testable. Seam à préserver pour les stratégies
  futures (least-loaded, round-robin, pondérée).
- Agnostique : « stratégie active », « pool de candidats ».

## Dépendances

- Dépend de US-2 (consomme les pools de candidats résolus). Pré-requis de US-4.
