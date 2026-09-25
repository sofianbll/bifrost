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
