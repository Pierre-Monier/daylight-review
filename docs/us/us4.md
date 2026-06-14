# US-4 — Application de l'assignation

> **Produit** : Daylight Review — MVP assignation (pilier Équité).
> **Fondation** : service stateless, ownership dérivé de `CODEOWNERS` + groupes,
> stratégie de sélection unique derrière une interface. Cible : GitLab + Teams.

**En tant qu'**auteur d'une MR,
**je veux** que les reviewers sélectionnés soient effectivement positionnés sur la MR,
**afin que** la review soit attribuée et visible dans l'outil.

## Critères d'acceptation

- **Étant donné** des reviewers sélectionnés, **quand** Daylight applique l'assignation,
  **alors** ils sont positionnés comme reviewers sur la MR dans la plateforme git.
- **Étant donné** une MR déjà assignée par Daylight, **quand** un événement survient sans
  changement de périmètre, **alors** l'assignation existante est conservée (pas de churn).
- **Étant donné** un changement de périmètre ajoutant une nouvelle équipe propriétaire,
  **quand** Daylight ré-évalue, **alors** il **complète** l'assignation pour la nouvelle
  équipe sans retirer les reviewers déjà valides.
- **Étant donné** un échec de l'appel d'assignation (API indisponible), **quand** Daylight
  applique, **alors** l'opération est ré-essayée et loggée, sans laisser la MR dans un état
  incohérent.

## Notes techniques (cible)

- GitLab : via l'API MR (champ *reviewers* / `reviewer_ids`), distinct des *assignees* et des
  *approval rules*.
- Positionner un reviewer déclenche la notification native GitLab — filet en attendant US-5
  (notification ciblée Teams, en refinement).
- Agnostique : « positionner comme reviewer ».

## Dépendances

- Dépend de US-3 (applique la sélection).
- Produit la notification native git en attendant US-5 + son enabler (app Graph/Teams,
  consentement admin tenant, à dérisquer en spike sprint 0).
