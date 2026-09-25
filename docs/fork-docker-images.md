# Compiler et publier une image du fork

`main` reste le miroir du dépôt officiel. La compilation utilise **le code du
commit choisi**, y compris les modifications de `core`, des plugins, du gateway
et l'interface web. MCP Apps est inclus quand ce code est présent dans la branche.
Le chargement dynamique des plugins Go `.so` est activé avec `DYNAMIC=1`.

## Publier depuis une branche feature

La branche doit contenir le workflow `.github/workflows/fork-image.yml` et les
fichiers de compilation associés. Intégrer un tag officiel met à jour le code,
mais ne rajoute pas automatiquement cette configuration personnelle à une branche
neuve. Pour une autre feature, repartir d'une branche qui la contient, ou reprendre
le commit de configuration Docker du fork avec `git cherry-pick`.

Une fois les changements commités, depuis la branche choisie :

```bash
git switch feature/mcp-apps-native
git push origin HEAD
git tag image/mcp-apps-2.2.3-1
git push origin image/mcp-apps-2.2.3-1
```

Le tag `image/...` désigne ce commit précis. Choisir un nouveau nom à chaque
publication, sans déplacer un tag déjà publié. Le suffixe devient le tag Docker :
`ghcr.io/sofianbll/bifrost:mcp-apps-2.2.3-1`. Utiliser des minuscules, chiffres,
points, tirets et underscores, avec au maximum 80 caractères.

Suivre **Fork Docker image** dans
[GitHub Actions](https://github.com/sofianbll/bifrost/actions).
Il compile sur des machines natives AMD64 et ARM64, exécute les tests Go MCP,
schémas, handlers et serveur existants avec le détecteur de races, puis vérifie
la santé, un fichier JavaScript de l'interface et le chargement du plugin
`hello-world` dans chaque image. Ce contrôle ne requiert aucune clé de fournisseur.
Il ne couvre pas les appels à de vrais fournisseurs ni tous les tests du dépôt.

Chaque image testée est envoyée à GHCR sans recompilation. Le tag final contenant
les deux architectures n'est publié que si les deux jobs réussissent. Les tags
`build-...-amd64` et `build-...-arm64` sont les images intermédiaires de ces jobs.
Le résumé du job **publish** donne le digest de l'image finale et le commit source.

## Assembler plusieurs features dans dev

Créer `dev` depuis la base souhaitée, y intégrer les features et la configuration
Docker du fork, puis pousser la branche. Chaque push sur `dev` publie, après les
mêmes contrôles, une image `dev-<commit court>-<numéro de workflow>`.
Il est aussi possible d'y poser un tag `image/...` avec un nom explicite.

## Utiliser l'image dans Compose

GHCR est un registre Docker : Docker Hub n'est pas nécessaire. Le workflow utilise
le `GITHUB_TOKEN` fourni par GitHub, avec la permission `packages: write` ; aucun
secret Docker Hub n'est requis. Sur un fork neuf, activer Actions si GitHub affiche
le bouton d'activation sur la page Actions.

Un nouveau package GHCR est privé par défaut. Pour permettre le téléchargement
sans authentification, passer sa visibilité à **Public** dans les paramètres du
package GitHub. Sinon, s'authentifier à GHCR sur la machine de déploiement avec un
jeton autorisé à lire le package.

Dans le service Bifrost du Compose existant, remplacer seulement la référence
`image` par celle donnée dans le résumé du workflow :

```yaml
image: ghcr.io/sofianbll/bifrost@sha256:REMPLACER_PAR_LE_DIGEST_DU_WORKFLOW
```

Le digest fige exactement l'image testée. Pour passer en production, conserver ce
digest et placer `prod` sur le commit correspondant : pas de nouvelle compilation.
Les données persistantes restent gérées par les volumes de chaque environnement.

## Compiler localement la même image

Depuis la racine du dépôt, Docker compile aussi l'interface web :

```bash
docker build -f transports/Dockerfile.local \
  --build-arg DYNAMIC=1 \
  --build-arg VERSION="$(cat transports/version)-$(git rev-parse --short=12 HEAD)" \
  -t bifrost:local .
```

Le résultat est une **image Docker**, contenant le binaire Go et l'interface.
Sans `DYNAMIC=1`, ce Dockerfile conserve le comportement statique d'origine.
Un plugin `.so` doit être compilé avec le même Go, les mêmes dépendances, options
de compilation, architecture et libc que son gateway. Le test du workflow
compile l'exemple dans le même environnement ; il ne garantit pas la compatibilité
d'un plugin précompilé ailleurs.

Sources : [événement push et tags](https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows#push),
[GHCR et visibilité](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry).

## Diagnostic MCP Apps

Utiliser le logger natif de Bifrost, sans ajouter de service de logs. Pour une
session de diagnostic, définir `LOG_LEVEL=debug` dans l'environnement du service
`gateway`, puis recréer ce service. L'option binaire `-log-level debug` existe
aussi ; si présente, elle prend priorité sur la variable d'environnement.

```sh
docker compose up -d --no-deps --force-recreate gateway
docker compose logs --since=10m gateway | grep -E '\[mcp-apps\]|\[mcp-server\]'
```

Les traces `[mcp-apps]` de ce correctif indiquent l'admission des Apps, les
collisions de callbacks, le résultat des appels natifs et la lecture des
ressources. Elles ne contiennent ni arguments, ni HTML, ni credentials, ni URL
upstream. Ce périmètre ne constitue pas un audit de toutes les traces debug de
Bifrost : ne pas publier des logs complets sans les vérifier et les expurger.
Revenir à `LOG_LEVEL=info` après le diagnostic et recréer le service.

La validation doit distinguer : découverte des outils et métadonnées UI,
`resources/list` avec un tableau JSON (y compris vide), `resources/read` avec
le MIME `text/html;profile=mcp-app`, résultat structuré de l'outil, puis rendu et
interactions dans un véritable client MCP Apps. Un message « Diagram displayed »
seul ne valide pas ce dernier point.

### Virtual MCP et Code Mode

Le correctif d'audit expose les Apps sur le même endpoint Virtual MCP que les
autres outils. Son ensemble d'outils admis reste le périmètre d'autorisation
partagé : il n'ajoute pas un périmètre d'isolation distinct pour chaque iframe.
Les ressources UI gardent leurs URI préfixées par source ; leurs lectures et les
appels restent soumis aux permissions Bifrost.

Pour un client upstream configuré en Code Mode, ses outils admis sont aussi
exposés directement par le gateway lorsqu'un outil App admis est présent. Les
outils ordinaires des autres clients restent en Code Mode et le comportement de
l'inférence n'est pas changé. Une App appelée directement dispose ainsi de ses
métadonnées UI et de son résultat structuré. Un appel imbriqué dans
`executeToolCode` reste une exécution de script retournant du texte : son rendu
App n'est pas pris en charge par ce correctif.

Les callbacks au nom upstream ne sont routés que si ce nom est sans ambiguïté
parmi les outils admis. Une collision entre sources, ou avec un nom public, est
rejetée explicitement. Des noms préfixés restent disponibles lorsqu'ils ne sont
pas eux-mêmes en collision. Une App qui impose un nom de callback ambigu doit
utiliser un Virtual MCP plus restreint ou des noms upstream distincts.

Comparaison vérifiée : [agentgateway MCP Apps](https://agentgateway.dev/docs/standalone/latest/documentation/mcp/apps/)
documente fédération et URI réécrites, ainsi que les limites du préfixage des
callbacks. Son [Code Mode](https://docs.solo.io/agentgateway/standalone/latest/documentation/mcp/tool-mode/)
documente le retour final du script ; ces pages ne prouvent pas le rendu d'une
App appelée à l'intérieur de ce script.

### Audit du 25 septembre 2026 — correctif local

Diagnostic reproduit sur Pulsar : `resources/list` répondait HTTP 200 avec
`{"resources":null}`. Le test de régression a échoué avec ce résultat avant le
correctif, puis réussi avec `[]`. Un second test a reproduit l'absence d'alias
pour les callbacks auxiliaires ordinaires avant sa correction.

L'audit de complexité a inventorié l'arbre du dépôt et examiné l'intégration et
son diff depuis `transports/v2.2.3` ; il ne constitue pas une revue exhaustive
de tout le code upstream. Deux simplifications ont été appliquées : modifier
uniquement `params.name` avec le helper JSON existant, et réutiliser les maps de
métadonnées privées au lieu de les recopier. Aucun ajout de dépendance.

Validation locale : tests ciblés MCP Apps et Code Mode réussis ; suites complètes
`go test -race ./core/mcp/... ./core/schemas/...` et
`go test -race ./transports/bifrost-http/handlers/... ./transports/bifrost-http/server/...`
réussies après autorisation des sockets de test hors sandbox. Le premier refus
`EPERM` était une limitation d'exécution, pas un échec de régression.
Compilation Python du smoke test réussie. Image candidate dynamique Linux arm64
`bifrost:mcp-apps-candidate-20260925`, digest
`sha256:f35db922ddef8acf612655f11ed4cfd6ff6dcefa5beef000a03087bc8f4eb8d0`.
Sur conteneur isolé avec Excalidraw réel : 23/23 contrôles mono-source, 27/27
agrégés, 5/5 Code Mode (appel natif de l'App avec Code Mode activé).
Rapports expurgés dans `docs/qa/bifrost-2.2.3/mcp-apps-candidate-*-2026-09-25.json`.
Conteneur de test et clés temporaires supprimés. Le rendu interactif de cette
candidate dans Codex et sa validation AMD64 restent à faire ; aucun déploiement
sur Pulsar n'a eu lieu dans cet audit.
