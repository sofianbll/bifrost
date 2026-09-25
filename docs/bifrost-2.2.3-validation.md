# Bifrost 2.2.3 — validation de notre intégration MCP Apps

Vérifié le **25 septembre 2026**, sur macOS ARM64, avec Go 1.27.1.

**Résultat : aucune régression détectée dans les tests exécutés du candidat local 2.2.3 + MCP Apps. Le binaire officiel 2.2.3 ne fournit toujours pas notre relais MCP Apps.** La production et la branche de travail n'ont pas été mises à jour.

## Où en était le projet ?

La branche `feature/mcp-apps-native`, au commit `1463c93`, contient déjà trois commits d'intégration au-dessus de Bifrost 2.2.2 (`fdeef8e`) :

- `a422dac` : relais natif des métadonnées UI, ressources et résultats MCP complets.
- `8c44647` : comportement complémentaire et couverture de régression.
- `1463c93` : isolation des callbacks sur les routes d'un seul serveur amont.

Excalidraw doit être connecté avec **Code Mode désactivé**, sur `/mcp/excalidraw`. Avec plusieurs serveurs, `/mcp` conserve les outils ordinaires mais masque les interfaces et callbacks réservés aux apps. Le rendu progressif dans Codex était encore ouvert ; voir la [note précédente](mcp-apps-realtime-research.md).

## Périmètre de preuve

| Variante | Provenance | Usage |
| --- | --- | --- |
| Référence locale 2.2.2 + MCP Apps | `1463c93a75985b4623da63eda5098eecf58cea62` | Tests ciblés et base SQLite avant migration |
| Officiel 2.2.3 | Binaire Darwin ARM64 téléchargé chez Maxim | Contrôle du comportement réellement distribué |
| Candidat 2.2.3 + MCP Apps | Tag `transports/v2.2.3`, commit `411d62b28b03b03bd3b4025b2cfab50af45f05f4`, plus le diff `fdeef8e..1463c93` | Compilation, tests Go, Excalidraw réel, migration |

Le diff local s'applique **sans conflit et sans adaptation du code produit**. Le candidat a été compilé dans une copie temporaire avec le `go.work` local ; son HTML embarqué était un simple substitut pour une compilation API. **Cette compilation ne valide pas le dashboard.** Le [manifeste](qa/bifrost-2.2.3/manifest.json) conserve les identifiants et empreintes exacts. L'empreinte du binaire officiel est calculée localement, sans checksum publiée par l'éditeur pour comparaison.

## Résultats

Les nombres ci-dessous incluent les sous-tests. Les suites se recoupent : **ne pas additionner leurs lignes**.

| Vérification | Résultat |
| --- | --- |
| Tests ciblés MCP de la référence locale 2.2.2 | 846 réussis, 0 échec, 2 ignorés |
| Mêmes tests sur le candidat 2.2.3 | 854 réussis, 0 échec, 2 ignorés |
| Suite complète `core/internal/mcptests`, avec `-race` et serveurs locaux construits | 669 réussis, 0 échec, 29 ignorés |
| Tous les tests des packages concernés, avec `-race` | 4 166 réussis, 0 échec, 12 ignorés initialement |
| Suite `framework/configstore/...` relancée avec PostgreSQL 16, avec `-race` | 927 réussis, 0 échec, 2 ignorés |
| Six scénarios Newman MCP Apps déjà présents dans le dépôt | 6 requêtes, 6 assertions réussies |
| Contrôle du binaire officiel 2.2.3 | 16/16 observations attendues, dont l'absence du support Apps |
| Candidat, connexion HTTP non persistante | 23/23 contrôles |
| Candidat, connexion persistante et deux serveurs amont | 27/27 contrôles |
| Référence 2.2.2 avant migration | 23/23 contrôles |
| Candidat 2.2.3 sur la **même base SQLite** | 23/23 contrôles |
| Même base migrée, activation de la connexion persistante et redémarrage | 23/23 contrôles |

Après déduplication par package et nom de test, et remplacement des résultats initiaux par ceux obtenus avec PostgreSQL : **2 703 tests de premier niveau réussis ; 4 858 résultats réussis en comptant les sous-tests ; 0 échec ; 31 tests ignorés.** Aucune course de données signalée par les suites exécutées avec `-race`.

Les packages entièrement exécutés sont `core/mcp/...`, `core/schemas/...`, `transports/bifrost-http/handlers/...`, `transports/bifrost-http/server/...`, `framework/configstore/...`, `plugins/governance/...` et, séparément, `core/internal/mcptests`.

Les 31 tests restants sont explicitement désactivés par les suites : scénarios avec vrai LLM, exemples/TODO, cinq tests de protocole marqués incompatibles Go/Node upstream, autres scénarios nécessitant une implémentation de fixture, et deux tests de performance semant environ un million de lignes. Ils ne sont **pas** comptés comme réussis. Les motifs exacts et le détail par suite figurent dans [go-summary.json](qa/bifrost-2.2.3/go-summary.json).

## Ce qui a été observé avec Excalidraw réel

| Contrat | Officiel 2.2.3 | Candidat 2.2.3 + MCP Apps |
| --- | --- | --- |
| `initialize`, `tools/list`, `read_me`, `create_view` | Fonctionnels | Fonctionnels |
| Extension `io.modelcontextprotocol/ui` négociée | Absente | Présente |
| `_meta.ui.resourceUri` | Perdue | Conservée et URI réécrite vers Bifrost |
| Résultat `structuredContent.checkpointId` | Perdu, résultat converti en texte | Conservé |
| `resources/list` / `resources/read` | Erreur `-32601` | Pris en charge |
| Ressource UI | Inaccessible | HTML, MIME `text/html;profile=mcp-app` |
| Callbacks `save_checkpoint` / `read_checkpoint` sans préfixe | Non fournis | Fonctionnels, visibilité `app` |
| Clés limitées / refusées / invalides | Restrictions confirmées | Restrictions confirmées, y compris les ressources et alias |

Le candidat expose une **ressource à template** : `resources/list` retourne une liste vide (`null` dans la sérialisation du SDK), `resources/templates/list` expose `ui://bifrost/{client}/{resource}`, puis `resources/read` lit l'URI annoncée par l'outil. La sonde ne confond donc pas une liste vide avec l'absence de support des ressources.

Avec deux amonts, la route agrégée n'annonce ni UI ni callbacks privés, ne négocie pas l'extension UI et refuse le callback non qualifié ; `/mcp/excalidraw` continue de fonctionner. L'ancien 403 signalé après changement de connexion persistante **n'a pas été reproduit dans ce scénario isolé** ; son origine passée n'est pas établie.

Preuves : [officiel](qa/bifrost-2.2.3/official-live.json), [candidat](qa/bifrost-2.2.3/patched-live.json), [multi-serveurs](qa/bifrost-2.2.3/patched-sticky-multi-live.json), [migration](qa/bifrost-2.2.3/patched-after-upgrade-live.json), [redémarrage avec connexion persistante](qa/bifrost-2.2.3/patched-upgrade-sticky-live.json), [harness existant](qa/bifrost-2.2.3/existing-harness.log).

## Nouveautés et corrections utiles

La [recherche détaillée avec sources primaires](bifrost-2.2.3-release-research.md) couvre les 43 commits et 144 fichiers de la release :

- **Nouveautés :** clé fournisseur imposable pour chaque fallback, outils OpenAI asynchrones, cache de prompt GPT-6 et `reasoning.effort: "none"` sur Sol/Luna.
- **Corrections :** sérialisation Responses/MCP, affinité des routes LLM, raisonnement Bedrock, cache OpenRouter/Anthropic, transcription Gemini, règles de routage et télémétrie.
- **Correctif supplémentaire présent dans le tag :** alias des noms d'outils longs pour Codex/Kimi, absent du changelog de release.
- **Compatibilité Go :** certaines propriétés Responses deviennent des unions et le type `ParsedFallbacks` change. Notre code d'intégration compile et passe ses tests avec ces changements.

La 2.2.3 ne contient pas notre relais MCP Apps. Une mise à jour vers son **binaire officiel seul** supprimerait les fonctionnalités ajoutées par notre compilation locale.

## Rejouer les vérifications

Dans une copie du tag 2.2.3 sur laquelle le diff `fdeef8e..1463c93` a été appliqué, avec le workspace Go local :

```sh
make setup-mcp-tests
env -u OPENAI_API_KEY go test -json -race -count=1 -timeout=20m ./core/internal/mcptests
go test -json -race -count=1 -timeout=10m \
  ./core/mcp/... ./core/schemas/... \
  ./transports/bifrost-http/handlers/... ./transports/bifrost-http/server/... \
  ./framework/configstore/... ./plugins/governance/...
```

Les tests PostgreSQL utilisent le service `postgres` de `tests/docker-compose.yml`, utilisateur/base `bifrost`, port 5432. Cette validation l'a lancé seul dans le projet Compose `bifrost-mcp-223-qa`, avec un override limitant le port à `127.0.0.1`.

La [sonde HTTP réutilisable](../tests/mcpapps/smoke.py) accepte `--url`, `--keys`, `--output`, `--expect-apps` et `--multiple-clients`. Le fichier de clés JSON doit contenir `full`, `limited`, `denied` : clés temporaires `sk-bf-…` pour respectivement tous les outils Excalidraw, uniquement `read_me`, aucun outil. Activer `client.enforce_auth_on_inference`, configurer le client sous le nom et slug `excalidraw`, sans Code Mode. Pour le scénario multiple, ajouter l'amont `second` et l'autoriser pour `full`. La sonde ne sauvegarde ni clés ni HTML.

Les six scénarios existants ont été extraits sans modification de leurs assertions du dossier `112. MCP Apps Excalidraw (mcp-apps)` de `provider-harness.json`, puis exécutés via Newman avec une authentification Bearer héritée de la collection et une clé temporaire.

## Limites et état final

- **Validé :** code de la passerelle, protocole MCP Apps, contrôle d'accès, connexions persistantes, migration SQLite et tests PostgreSQL concernés.
- **Non validé :** rendu visuel/animation progressive dans Codex ou ChatGPT, dashboard compilé, autres fournisseurs LLM réels, autres suites du monorepo et instance de production. Le dessin progressif exige une vérification des événements hôte → iframe ; un succès HTTP ne le prouve pas.
- **Modifications du dépôt :** documentation, preuves et sonde de test uniquement. Branche, commits et code produit conservés ; aucune mise à jour de production.
- Les serveurs QA et le conteneur PostgreSQL dédiés sont arrêtés et supprimés après les mesures ; les preuves sans clés sont conservées dans `docs/qa/bifrost-2.2.3/`.

Les premiers essais de la nouvelle sonde ont révélé deux erreurs de **fixture de test**, corrigées avant les résultats retenus : préfixe de VK incorrect et attente d'une ressource statique alors que le relais utilise un template. Les erreurs initiales de ports/cache Go du sandbox ont été résolues en exécutant les tests avec les permissions nécessaires. Aucun de ces essais n'est présenté comme une régression produit.
