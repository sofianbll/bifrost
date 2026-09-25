# Bifrost 2.2.3 — release officielle et impact MCP Apps

Recherche vérifiée le **25 septembre 2026**, sur la release publiée et ses sources figées. Cette note décrit les changements upstream ; elle ne constitue pas un résultat de tests locaux ni une validation de production.

## Version exacte

| Élément | Valeur vérifiée |
| --- | --- |
| Release stable | **Bifrost HTTP v2.2.3**, `draft=false`, `prerelease=false` |
| Tag | `transports/v2.2.3` |
| Publication | **24/09/2026 à 18:31:19 UTC**, soit 20:31:19 à Paris |
| Commit | `411d62b28b03b03bd3b4025b2cfab50af45f05f4` |
| Objet tag annoté | `fe2a68533589ea880f803108739480a4e6f47f63` |
| Référence précédente | `transports/v2.2.2` → `fdeef8e3f31a3b18a61666ba49247d07bae3600a` |
| Comparaison | **43 commits, 144 fichiers modifiés**, base commune égale à 2.2.2 |
| Modules | Core **1.10.2**, framework **1.7.4** ; `mcp-go` reste **0.43.2** |

Sources : [release officielle](https://github.com/maximhq/bifrost/releases/tag/transports/v2.2.3), [métadonnées de publication](https://api.github.com/repos/maximhq/bifrost/releases/tags/transports%2Fv2.2.3), [tag annoté](https://api.github.com/repos/maximhq/bifrost/git/tags/fe2a68533589ea880f803108739480a4e6f47f63), [comparaison complète](https://api.github.com/repos/maximhq/bifrost/compare/transports%2Fv2.2.2...transports%2Fv2.2.3), [dépendances du transport](https://github.com/maximhq/bifrost/blob/411d62b28b03b03bd3b4025b2cfab50af45f05f4/transports/go.mod#L15-L32).

Le champ GitHub `target_commitish` vaut `dev`, mais l'analyse porte sur le **commit du tag publié**, pas sur le contenu actuel de `dev`. Le fichier Enterprise `ent-v2.2.2.mdx` apparaît dans le diff ; ses annonces ne sont pas attribuées à cette release OSS. [API de release](https://api.github.com/repos/maximhq/bifrost/releases/tags/transports%2Fv2.2.3), [diff des tags](https://github.com/maximhq/bifrost/compare/transports/v2.2.2...transports/v2.2.3)

## Nouveautés

| Fonctionnalité | Ce qui change | Source primaire |
| --- | --- | --- |
| Clé imposée par fallback | Chaque secours d'une règle de routage peut cibler une clé fournisseur ; l'éditeur UI permet de la choisir ou de l'effacer. L'ancienne chaîne `provider/model` reste compatible. | [#7470](https://github.com/maximhq/bifrost/commit/06fbbed4edcc), [UI #7380](https://github.com/maximhq/bifrost/commit/63273ba0828b) |
| Outils OpenAI asynchrones | Responses transmet `async`, `output_schema` et `tunnel_id`. Le champ `async` est retiré si le modèle ne le supporte pas ; capacité ajustable par datasheet. | [#7242](https://github.com/maximhq/bifrost/commit/be9616fe7ace) |
| Cache de prompt GPT-6 | Extension des breakpoints à la famille GPT-6 sur OpenAI, Azure, Bedrock et Mantle ; capacité ajustable par datasheet. | [#7240](https://github.com/maximhq/bifrost/commit/078a7e41e6b2) |
| Raisonnement désactivable | `reasoning.effort: "none"` est conservé pour `gpt-6-sol` et `gpt-6-luna`. | [#7492](https://github.com/maximhq/bifrost/commit/3a409061a228) |

## Corrections

| Périmètre | Correction | Source primaire |
| --- | --- | --- |
| Paramètres OpenAI | Retrait de `temperature`, `top_logprobs` et `logprobs` lorsqu'ils ne sont pas supportés, comme `top_p` ; meilleure interprétation d'un effort omis. | [#7239](https://github.com/maximhq/bifrost/commit/b304e56a25a5) |
| Formats Responses/MCP | Erreurs MCP structurées, `conversation` objet, `allowed_tools` tableau, réponses d'approbation et filtres `in`/`nin` sont correctement sérialisés. | [#7241](https://github.com/maximhq/bifrost/commit/a44106ad7a59) |
| Streaming Responses | Copie indépendante du drapeau `async` et des erreurs structurées, y compris leur contenu brut. | [code de copie](https://github.com/maximhq/bifrost/blob/411d62b28b03b03bd3b4025b2cfab50af45f05f4/framework/streaming/responses.go#L289-L317) |
| OpenRouter → Anthropic | Conservation des breakpoints `cache_control` selon la capacité du modèle. | [#7521](https://github.com/maximhq/bifrost/commit/21d6f15f10c9) |
| Affinité des routes LLM | Les clés imposées suivent leur fournisseur lorsque la chaîne est réordonnée ; la route est identifiée par fournisseur **et** modèle ; les associations suivies vers un échec sont supprimées. | [#7468](https://github.com/maximhq/bifrost/commit/9b91bdbedd45), [#7473](https://github.com/maximhq/bifrost/commit/b36b30dc1f1b) |
| Databricks → Gemini | Fusion des messages système et développeur multiples. | [#7461](https://github.com/maximhq/bifrost/commit/7faebcf84f29) |
| Raisonnement chiffré Bedrock | Une erreur de compte/modèle incompatible déclenche suppression et nouvelle tentative ; correction complémentaire de l'encodage/relecture `redactedContent` pour Converse. | [détection et retry](https://github.com/maximhq/bifrost/commit/c958df37ec2b), [#7529](https://github.com/maximhq/bifrost/commit/6d352efc2bd3) |
| Décisions Mantle | `tool_choice: "auto"` pour gpt-oss ; récupération de balises de paramètres entourées d'espaces. | [correctif d'émulation](https://github.com/maximhq/bifrost/commit/ec580ccb2328) |
| Transcription Gemini | Usage conservé même si la transcription est vide. | [source au tag](https://github.com/maximhq/bifrost/blob/411d62b28b03b03bd3b4025b2cfab50af45f05f4/core/providers/gemini/transcription.go) |
| État des règles | Une mise à jour ou synchronisation omettant `enabled` conserve la valeur stockée au lieu d'écrire NULL. | [source au tag](https://github.com/maximhq/bifrost/blob/411d62b28b03b03bd3b4025b2cfab50af45f05f4/framework/configstore/rdb.go#L5793-L5805) |
| Télémétrie | Persistance du réglage `user_labels_enabled`. | [#7490](https://github.com/maximhq/bifrost/commit/9e9c92b7ce6f) |
| Noms d'outils longs, Codex/Kimi | Les alias hachés commencent désormais par `t`. Le correctif vise les flux Kimi K3 Bedrock vides malgré HTTP 200 lorsque le nom commence par un chiffre. **Inclus dans le tag mais absent du changelog de release.** | [#7530, code et tests](https://github.com/maximhq/bifrost/commit/0b7fe609dd59541bd52239640f03d32dbb5fd5b4) |

## Conséquences pour notre intégration

**Le code officiel 2.2.3 ne remplace pas notre relais MCP Apps.** Son serveur annonce les outils sans capacité de ressources, reconstruit les métadonnées des outils et renvoie encore `NewToolResultText`. La conversion conserve les annotations MCP standard mais pas `_meta.ui`. Aucun nouveau routage `resources/list`/`resources/read` ni fichier `mcpapps.go` n'apparaît dans la comparaison. C'est un constat de sources ; l'exécution de l'artefact officiel doit être rapportée séparément. [serveur officiel](https://github.com/maximhq/bifrost/blob/411d62b28b03b03bd3b4025b2cfab50af45f05f4/transports/bifrost-http/handlers/mcpserver.go#L353-L439), [conversion officielle](https://github.com/maximhq/bifrost/blob/411d62b28b03b03bd3b4025b2cfab50af45f05f4/core/mcp/utils.go#L671-L740), [diff complet](https://github.com/maximhq/bifrost/compare/transports/v2.2.2...transports/v2.2.3)

- **OAuth, authentification, sessions MCP et callbacks :** aucun changement dans les chemins correspondants du relais MCP. Les corrections « session affinity » concernent le choix d'une route LLM ; elles ne modifient pas `needs_session_stickiness` du client MCP. Elles ne démontrent donc pas une résolution de nos questions de reconnexion ou de rendu progressif. [fichiers modifiés](https://api.github.com/repos/maximhq/bifrost/compare/transports%2Fv2.2.2...transports%2Fv2.2.3), [affinité LLM](https://github.com/maximhq/bifrost/blob/411d62b28b03b03bd3b4025b2cfab50af45f05f4/core/sessionaffinity.go)
- **Outils Responses et Code Mode :** les nouvelles unions d'erreurs affectent aussi l'extraction Starlark, mise à jour upstream. Les champs OpenAI `async`/`tunnel_id` et les alias de noms sont des changements du chemin fournisseur ; ils n'ajoutent pas de ressource HTML MCP Apps. [Starlark](https://github.com/maximhq/bifrost/blob/411d62b28b03b03bd3b4025b2cfab50af45f05f4/core/mcp/codemode/starlark/utils.go#L137-L149), [#7242](https://github.com/maximhq/bifrost/commit/be9616fe7ace), [#7530](https://github.com/maximhq/bifrost/commit/0b7fe609dd59541bd52239640f03d32dbb5fd5b4)
- **Plugins :** mises à jour de dépendances, corrections fonctionnelles du routage, cache sémantique et télémétrie. Aucun changement de chargeur dynamique ni de contrat d'extension UI. La release ne prouve donc pas une correction de la distribution des plugins personnalisés. [dépendances](https://github.com/maximhq/bifrost/blob/411d62b28b03b03bd3b4025b2cfab50af45f05f4/transports/go.mod), [diff](https://github.com/maximhq/bifrost/compare/transports/v2.2.2...transports/v2.2.3)

La comparaison avec notre diff local `fdeef8e..1463c93` donne quatre fichiers communs :

| Fichier | Modification upstream | Modification locale |
| --- | --- | --- |
| `core/bifrost.go` | Clé imposée dans les fallbacks | Exécution native MCP et lecture des ressources |
| `core/schemas/bifrost.go` | `Fallback.KeyID` | Métadonnées/contexte et résultat MCP brut |
| `framework/configstore/rdb.go` | Préservation de `enabled` des règles | Persistance des métadonnées des outils MCP |
| `tests/e2e/api/collections/provider-harness.json` | Nombreux scénarios fournisseurs/routage | Retouches locales du harness |

Les trois fichiers Go ont des zones de changement distinctes. Cette inspection réduit le risque de conflit textuel ; elle ne remplace pas compilation et tests. Preuves reproductibles : `git diff fdeef8e..1463c93 --name-only` et [comparaison officielle](https://api.github.com/repos/maximhq/bifrost/compare/transports%2Fv2.2.2...transports%2Fv2.2.3).

## Compatibilité à surveiller

1. **Cassures Go annoncées :** `ResponsesMCPApprovalResponse.ApprovalResponseID` devient `ApprovalRequestID` ; `ResponsesToolMessage.Error` et `ResponsesParameters.Conversation` deviennent des unions au lieu de `*string`. Le type JSON d'approbation devient `mcp_approval_response`, avec `approval_request_id`. [schémas et tests #7241](https://github.com/maximhq/bifrost/commit/a44106ad7a59)
2. **Cassure Go supplémentaire observée :** `TableRoutingRule.ParsedFallbacks` passe de `[]string` à `[]RoutingFallback`. Un consommateur compilé contre ce type doit s'adapter. En JSON, les anciennes chaînes restent acceptées ; un fallback avec clé devient un objet. [type et sérialisation](https://github.com/maximhq/bifrost/blob/411d62b28b03b03bd3b4025b2cfab50af45f05f4/framework/configstore/tables/routingrules.go#L23-L164)
3. **Configuration :** utiliser `provider_key_name` dans `config.json`. Le schéma de configuration n'autorise pas `key_id` dans cet objet, contrairement au modèle API/Go. [schéma de référence](https://github.com/maximhq/bifrost/blob/411d62b28b03b03bd3b4025b2cfab50af45f05f4/transports/config.schema.json#L3706-L3740)
4. **Base de données :** aucune nouvelle migration annoncée. Cela ne dispense pas de vérifier une base existante pendant les essais de mise à jour. [changelog officiel](https://github.com/maximhq/bifrost/blob/411d62b28b03b03bd3b4025b2cfab50af45f05f4/transports/changelog.md)

Aucune autre incompatibilité n'est annoncée dans la release consultée. Cela ne constitue pas une garantie d'absence de régression.

## Artefact officiel macOS ARM64

[Binaire officiel 2.2.3](https://downloads.getmaxim.ai/bifrost/v2.2.3/darwin/arm64/bifrost-http) : requête HEAD du 25/09/2026, **HTTP 200**, **122 704 882 octets**, `Last-Modified: 24 Sep 2026 18:28:30 GMT`. L'URL suit le [résolveur NPX officiel](https://github.com/maximhq/bifrost/blob/411d62b28b03b03bd3b4025b2cfab50af45f05f4/npx/bifrost/bin.js#L314-L319).

**Aucun SHA256 éditeur trouvé** : la release GitHub n'a pas d'assets, `bifrost-http.sha256` répond 404, et le [script de compilation gateway](https://github.com/maximhq/bifrost/blob/411d62b28b03b03bd3b4025b2cfab50af45f05f4/.github/workflows/scripts/build-executables.sh) ne produit pas de checksum. Le SHA256 calculé après téléchargement sera une empreinte locale observée, à distinguer d'une vérification contre une checksum publiée. Aucun téléchargement ni lancement du binaire n'a été effectué pour cette recherche.
