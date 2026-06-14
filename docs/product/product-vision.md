# Vision produit — couche de coordination de la review

*(nom de code à définir)*

## En une phrase

Plus de volontariat : chaque MR est automatiquement confiée aux bonnes personnes selon les règles que chaque équipe maîtrise, et tout le monde voit honnêtement où en sont les reviews et qui porte la charge.

## Le problème

Dans les grosses organisations multi-équipes, la code review repose sur du volontariat. Trois conséquences :

- **Iniquité invisible.** Certains portent les reviews, d'autres esquivent. Les leads le sentent mais ne peuvent ni le voir ni le corriger.
- **Opacité.** Personne n'a de vue systémique sur ce qui est coincé, où, et pourquoi — la vue par MR existe (mal) dans l'outil git, la vue d'ensemble n'existe nulle part.
- **Coutures qui craquent.** Quand une MR touche les modules de plusieurs équipes, personne ne sait qui doit reviewer, ni quelle équipe absorbe la charge des autres.

## La cible

Grosses équipes d'ingénierie multi-équipes, type Renault. Elles ont des modules, des équipes responsables, des leads, et une vraie sensibilité à la donnée — un facteur qui pèsera lourd sur le choix du modèle de déploiement (voir les tensions à trancher).

## Positionnement

Le marché se divise en deux familles, chacune avec son angle mort :

- Les outils d'*engineering effectiveness* (dashboards pour managers) **mesurent et rapportent**, en top-down, trop tard, et sont vécus comme de la surveillance.
- Les outils de review-in-chat sont **dans la boucle** mais centrés conversation, pas équité ni visibilité.

Le trou au milieu, et la place du produit : être **dans la boucle** (agir sur chaque MR en temps réel) avec pour finalité **l'équité et la fluidité**, et un parti pris fort — la transparence par design, pas la surveillance par rapport.

## Les deux piliers

**1. Équité.** Chaque MR reçoit automatiquement les bons reviewers, selon le plan que chaque équipe maîtrise, charge équilibrée. Ferme et déterministe.

**2. Visibilité.** L'état réel de chaque review et de la charge, à l'échelle de plusieurs équipes, rendu lisible et honnête. C'est ce qui transforme une douleur diffuse en fait négociable.

Tout le reste — notifications, sync de ticket, nudges — sont des fonctionnalités *de* ces deux jambes, pas des piliers. La communication est un canal, pas un pilier.

## Principes directeurs

- **Loi à l'intérieur, lumière aux coutures.** Le plan d'une équipe fait loi en interne (appliqué, pas conseillé). Aux frontières entre équipes, le produit ne tranche pas — il mesure, il alerte, il alimente la conversation.
- **Subsidiarité.** Une règle de répartition globale par défaut (équitable), surchargeable par chaque équipe. Les équipes sont maîtresses de leur répartition ; le rôle du produit est de leur faciliter cette configuration.
- **Plus de volontariat = plus de dérive.** Comme le plan est appliqué, la distribution réelle égale la distribution déclarée par construction. La concentration n'existe que si une équipe la *choisit*.
- **Honnêteté imposée, équité non imposée.** Une équipe a le droit de concentrer sa charge ; mais alors tout le monde le voit. Le produit n'impose pas l'équité, il garantit la transparence sur la distribution réelle.
- **Instrument, pas autorité.** Le pousse-back cross-team est consultatif : le produit ne bloque jamais une review. Il fournit le chiffre qui permet à une équipe de dire, en réunion, « on dépasse notre quota, on ne peut pas continuer comme ça ».
- **Une charge, un plan.** Pas de seau interne et de seau externe séparés : une seule charge de review par personne, répartie par le plan. La distinction interne/externe reste un *diagnostic* (voir plus bas).
- **Ne jamais mesurer la compréhension.** Instrumenter l'attention (temps, nombre de commentaires, scroll) produit du théâtre ou de la surveillance. Le produit *facilite* l'attention et ne signale que l'absence flagrante d'engagement — en privé, au reviewer, jamais en classement public.

## Santé d'une MR

Une MR en bonne santé reçoit de l'attention, est vraiment lue et comprise, et ses parties prenantes savent où elle en est. Trois postures distinctes :

- **Attention** — se *permet* en amont : le bon reviewer (expertise + dispo), une MR d'une taille humainement relisible, le bon timing. À moitié déjà réglé par le moteur d'assignation.
- **Compréhension** — terrain miné : réduire la friction (donner le *pourquoi* du changement, pas juste le *quoi*) et ne signaler que le tampon évident, en privé.
- **Conscience des parties prenantes** — la forme honnête de la « communication » : chaque personne concernée *sait* que le changement existe et où il en est.

Parties prenantes d'une MR : **assigné**, **reviewer(s)** (qui absorbe déjà tout le cas cross-team, puisque toucher un module tire son propriétaire dans la review), et **éventuellement le PO** pour qu'il voie le ticket changer de statut.

## Diagnostic d'ownership

Le ratio review interne vs externe par équipe n'est pas qu'une mesure de fatigue : c'est une lentille sur la *structure de découpage*. Une équipe qui passe 60 % de son temps de review sur du code externe ne souffre peut-être pas d'un problème de personnes, mais d'une frontière mal posée ou d'un périmètre trop large. Insight structurel qu'aucun dashboard de vélocité ne donne.

## Périmètre

**Cœur**
- Assignation automatique, fin du volontariat, plan appliqué
- Configuration de la répartition par équipe (défaut global surchargeable)
- Visibilité de la charge, y compris le ratio interne/externe
- Routage et mesure cross-team (alerte de quota, consultative)
- Vue de l'état et de la santé des reviews, à l'échelle multi-équipes

**Nice to have**
- Sync du statut de ticket pour le PO (le ticket reflète automatiquement l'état de la MR)
- Surface de notification (Teams ou autre) comme canal de la visibilité
- Nudge privé en cas de tampon manifeste

**Hors périmètre (pour l'instant)**
- Conversation/chat sur la MR (terrain le plus fort d'Axolo, mal traduit sur Teams)
- Équipes en aval qui dépendent du changement sans posséder les lignes touchées
- Budget cross-team dur / blocage d'assignation
- Tout scoring de la *qualité* d'une review

## Tensions à trancher plus tard

- **Top-down vs bottom-up.** La tour de contrôle cross-équipes est la valeur pour l'acheteur ; la vue perso (« où en est *ma* MR ») crée l'adoption des devs. Laquelle construire d'abord ?
- **Les leviers du « qu'est-ce qu'on fait ? ».** Quand une équipe sature, que lui permet le produit concrètement, au-delà de l'alerte ?
- **La définition opérationnelle du « équitable par défaut ».** Nombre de reviews ? Pondéré par taille/complexité ? Ajusté pour les auteurs déjà chargés et les juniors ?
- **SaaS vs self-hosted.** Le SaaS est plus simple à déployer et accélère l'adoption ; le self-hosted répond à la sensibilité données/souveraineté du segment mais alourdit la mise en place. À arbitrer selon ce que veut vraiment le marché — voire à proposer les deux.
